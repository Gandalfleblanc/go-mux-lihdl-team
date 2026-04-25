package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"go-mux-lihdl-team/internal/config"
	"go-mux-lihdl-team/internal/hydracker"
	"go-mux-lihdl-team/internal/mediainfo"
	"go-mux-lihdl-team/internal/mkvtool"
	"go-mux-lihdl-team/internal/naming"
	"go-mux-lihdl-team/internal/tmdb"
)

type App struct {
	ctx context.Context

	// context partagé par l'opération en cours (mux) — permet d'annuler
	// immédiatement via le bouton Stop côté UI.
	mu       sync.Mutex
	opCtx    context.Context
	opCancel context.CancelFunc
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Drop-zone : .mkv → "file:dropped" (1er) ou "files:dropped" (batch N≥2) ;
	// subs externes → "subs:dropped" ; audios externes → "audios:dropped".
	// Les types peuvent être mixés dans un même drop, on dispatche chacun.
	subExts := map[string]bool{
		".srt": true, ".sup": true, ".ass": true,
		".ssa": true, ".sub": true, ".idx": true,
	}
	audExts := map[string]bool{
		".ac3": true, ".eac3": true, ".dts": true, ".aac": true,
		".flac": true, ".mp3": true, ".mka": true, ".wav": true,
		".opus": true, ".truehd": true,
	}
	wr.OnFileDrop(ctx, func(x, y int, paths []string) {
		var mkvs, subs, auds []string
		for _, p := range paths {
			ext := strings.ToLower(filepath.Ext(p))
			switch {
			case ext == ".mkv":
				mkvs = append(mkvs, p)
			case subExts[ext]:
				subs = append(subs, p)
			case audExts[ext]:
				auds = append(auds, p)
			}
		}
		if len(mkvs) == 1 {
			wr.EventsEmit(ctx, "file:dropped", mkvs[0])
		} else if len(mkvs) > 1 {
			wr.EventsEmit(ctx, "files:dropped", mkvs)
		}
		if len(subs) > 0 {
			wr.EventsEmit(ctx, "subs:dropped", subs)
		}
		if len(auds) > 0 {
			wr.EventsEmit(ctx, "audios:dropped", auds)
		}
		if len(mkvs) == 0 && len(subs) == 0 && len(auds) == 0 {
			wr.EventsEmit(ctx, "log", "⚠ Aucun .mkv / sous-titre / audio détecté dans le drop")
		}
	})
}

// --- Version ---

// AppVersion est lue par le frontend (pill dans le header) et utilisée pour
// comparer avec la dernière release GitHub lors du check de mise à jour.
const AppVersion = "v4.0.0"

func (a *App) GetVersion() string { return AppVersion }

// --- Config ---

func (a *App) GetConfig() config.Config         { return config.Load() }
func (a *App) SaveConfig(c config.Config) error { return config.Save(c) }

// --- Dropdowns LiHDL ---

// LihdlOptions regroupe toutes les listes déroulantes figées par les normes
// LiHDL. Exposé au frontend en un seul appel pour simplifier l'init de l'UI.
type LihdlOptions struct {
	AudioLabels    []string `json:"audio_labels"`
	SubtitleLabels []string `json:"subtitle_labels"`
	VideoQualities []string `json:"video_qualities"`
	VideoEncoders  []string `json:"video_encoders"`
	VideoSources   []string `json:"video_sources"`
	VideoTeams     []string `json:"video_teams"`
}

func (a *App) GetLihdlOptions() LihdlOptions {
	return LihdlOptions{
		AudioLabels:    naming.AudioLabels,
		SubtitleLabels: naming.SubtitleLabels,
		VideoQualities: naming.VideoQualities,
		VideoEncoders:  naming.VideoEncoders,
		VideoSources:   naming.VideoSources,
		VideoTeams:     naming.VideoTeams,
	}
}

// --- Helpers fichier ---

// FileSize retourne la taille du fichier en octets, ou -1 si erreur.
func (a *App) FileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

// --- Dialogs système ---

func (a *App) SelectMkvFile() (string, error) {
	path, err := wr.OpenFileDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Choisir un fichier .mkv",
		Filters: []wr.FileFilter{
			{DisplayName: "Matroska (*.mkv)", Pattern: "*.mkv"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// SelectSubFiles ouvre un dialog multi-sélection pour les subs externes.
func (a *App) SelectSubFiles() ([]string, error) {
	return wr.OpenMultipleFilesDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Choisir un ou plusieurs fichiers de sous-titres",
		Filters: []wr.FileFilter{
			{DisplayName: "Sous-titres (*.srt *.sup *.ass *.ssa *.sub *.idx)",
				Pattern: "*.srt;*.sup;*.ass;*.ssa;*.sub;*.idx"},
		},
	})
}

// SelectAudioFiles ouvre un dialog multi-sélection pour les audios externes.
func (a *App) SelectAudioFiles() ([]string, error) {
	return wr.OpenMultipleFilesDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Choisir un ou plusieurs fichiers audio",
		Filters: []wr.FileFilter{
			{DisplayName: "Audio (*.ac3 *.eac3 *.dts *.aac *.flac *.mp3 *.mka *.wav *.opus *.truehd)",
				Pattern: "*.ac3;*.eac3;*.dts;*.aac;*.flac;*.mp3;*.mka;*.wav;*.opus;*.truehd"},
		},
	})
}

func (a *App) SelectOutputDir() (string, error) {
	return wr.OpenDirectoryDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Choisir le dossier de sortie",
	})
}

// OpenURL ouvre une URL dans le navigateur système.
// Wails ne suit pas les <a target="_blank"> par défaut — on passe par
// runtime.BrowserOpenURL pour les liens externes.
func (a *App) OpenURL(url string) {
	if url != "" {
		wr.BrowserOpenURL(a.ctx, url)
	}
}

// MoveToTrash envoie un ou plusieurs fichiers à la corbeille (réversible).
// macOS  : AppleScript via Finder (revient au dossier d'origine si annulé)
// Linux  : gio trash (si dispo) sinon best-effort
// Windows: pas implémenté (renvoie une erreur)
func (a *App) MoveToTrash(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		// AppleScript : passer chaque chemin en POSIX file via Finder
		var sb strings.Builder
		sb.WriteString(`tell application "Finder" to delete every item of {`)
		for i, p := range paths {
			if p == "" {
				continue
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			esc := strings.ReplaceAll(p, `"`, `\"`)
			sb.WriteString(`(POSIX file "` + esc + `")`)
		}
		sb.WriteString(`}`)
		cmd := exec.Command("osascript", "-e", sb.String())
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New("osascript : " + err.Error() + " — " + string(out))
		}
		return nil
	case "linux":
		for _, p := range paths {
			if p == "" {
				continue
			}
			if err := exec.Command("gio", "trash", p).Run(); err != nil {
				// fallback : ignore l'erreur, on log côté front
				wr.EventsEmit(a.ctx, "log", "⚠ corbeille : "+p+" "+err.Error())
			}
		}
		return nil
	default:
		return errors.New("MoveToTrash : non supporté sur " + runtime.GOOS)
	}
}

// OpenFolder ouvre un dossier dans l'explorateur de fichiers natif.
// macOS → open, Linux → xdg-open, Windows → explorer.
func (a *App) OpenFolder(path string) error {
	if path == "" {
		return errors.New("chemin vide")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// --- mkvmerge ---

// LocateMkvmerge retourne le chemin absolu du binaire utilisable ou
// une chaîne vide si non trouvé (laissée au frontend de guider le user).
func (a *App) LocateMkvmerge() string {
	c := config.Load()
	binDir, _ := config.BinDir()
	p, err := mkvtool.Locate(c.MkvmergePath, binDir)
	if err != nil {
		return ""
	}
	return p
}

// AnalyzeMkv exécute "mkvmerge -J" et émet le résultat via l'event
// "analyze:result" (pattern event-based pour contourner les soucis de
// retour de gros payloads via Wails IPC).
func (a *App) AnalyzeMkv(path string) {
	go func() {
		binary := a.LocateMkvmerge()
		if binary == "" {
			wr.EventsEmit(a.ctx, "log", "❌ mkvmerge introuvable — installe MKVToolNix ou configure le chemin")
			wr.EventsEmit(a.ctx, "analyze:result", map[string]any{"ok": false, "error": "mkvmerge introuvable"})
			return
		}
		wr.EventsEmit(a.ctx, "log", "🔎 Analyse "+filepath.Base(path))
		raw, err := mkvtool.IdentifyRaw(a.ctx, binary, path)
		if err != nil {
			wr.EventsEmit(a.ctx, "log", "❌ "+err.Error())
			wr.EventsEmit(a.ctx, "analyze:result", map[string]any{"ok": false, "error": err.Error()})
			return
		}
		var info mkvtool.Info
		_ = json.Unmarshal([]byte(raw), &info)
		wr.EventsEmit(a.ctx, "log", "✓ "+pluralTracks(len(info.Tracks))+" détectée(s)")

		// Enrichissement mediainfo (audio/sub) : mêmes champs qu'AnalyzeMkvSecondary.
		mediainfoByID := map[int]mediainfo.Track{}
		if mibin, mErr := mediainfo.Locate(""); mErr == nil {
			if mi, mErr2 := mediainfo.Identify(a.ctx, mibin, path); mErr2 == nil {
				audIdx, subIdx := 0, 0
				audTracks, subTracks := []mkvtool.Track{}, []mkvtool.Track{}
				for _, t := range info.Tracks {
					if t.Type == "audio" {
						audTracks = append(audTracks, t)
					} else if t.Type == "subtitles" {
						subTracks = append(subTracks, t)
					}
				}
				for _, mt := range mi.Media.Track {
					if mt.Type == "Audio" && audIdx < len(audTracks) {
						mediainfoByID[audTracks[audIdx].ID] = mt
						audIdx++
					} else if mt.Type == "Text" && subIdx < len(subTracks) {
						mediainfoByID[subTracks[subIdx].ID] = mt
						subIdx++
					}
				}
			}
		}

		tracksPayload := make([]map[string]any, 0, len(info.Tracks))
		for _, t := range info.Tracks {
			row := map[string]any{
				"id":               t.ID,
				"type":             t.Type,
				"codec":            t.Codec,
				"language":         t.Properties.Language,
				"track_name":       t.Properties.TrackName,
				"audio_channels":   t.Properties.AudioChannels,
				"codec_id":         t.Properties.CodecID,
				"default_track":    t.Properties.DefaultTrack,
				"forced_track":     t.Properties.ForcedTrack,
				"pixel_dimensions": t.Properties.PixelDimensions,
			}
			if mt, ok := mediainfoByID[t.ID]; ok {
				row["mi_title"] = mt.Title
				row["mi_format"] = mt.Format
				row["mi_format_profile"] = mt.FormatProfile
				row["mi_format_commercial"] = mt.FormatCommercial
				row["mi_format_commercial_if_any"] = mt.FormatCommercialIfAny
				row["mi_format_features"] = mt.FormatAdditionalFeatures
				row["mi_service_kind"] = mt.ServiceKind
				row["mi_service_kind_name"] = mt.ServiceKindNames
				row["mi_stream_size"] = mt.StreamSize
				row["mi_element_count"] = mt.ElementCount
			}
			tracksPayload = append(tracksPayload, row)
		}
		wr.EventsEmit(a.ctx, "log", "🔔 emit analyze:start n="+itoa(len(tracksPayload)))
		wr.EventsEmit(a.ctx, "analyze:start", len(tracksPayload))
		for i, t := range tracksPayload {
			b, err := json.Marshal(t)
			if err != nil {
				continue
			}
			wr.EventsEmit(a.ctx, "log", "🔔 emit analyze:track i="+itoa(i)+" bytes="+itoa(len(b)))
			wr.EventsEmit(a.ctx, "analyze:track", string(b))
		}
		wr.EventsEmit(a.ctx, "log", "🔔 emit analyze:end")
		wr.EventsEmit(a.ctx, "analyze:end", len(tracksPayload))
	}()
}

// AnalyzeMkvSecondary analyse un mkv secondaire (SUPPLY/FW) et émet
// un événement "secondary:tracks" avec la liste des pistes audio + subs,
// enrichies par mediainfo quand disponible (track title détaillé,
// service kind, format profile).
func (a *App) AnalyzeMkvSecondary(path string) {
	binary := a.LocateMkvmerge()
	if binary == "" {
		wr.EventsEmit(a.ctx, "log", "❌ mkvmerge introuvable")
		return
	}
	go func() {
		raw, err := mkvtool.IdentifyRaw(a.ctx, binary, path)
		if err != nil {
			wr.EventsEmit(a.ctx, "log", "❌ secondaire : "+err.Error())
			return
		}
		var info mkvtool.Info
		_ = json.Unmarshal([]byte(raw), &info)

		// Tente d'enrichir avec mediainfo (best-effort, silencieux si absent).
		mediainfoByID := map[int]mediainfo.Track{}
		if mibin, err := mediainfo.Locate(""); err == nil {
			if mi, err := mediainfo.Identify(a.ctx, mibin, path); err == nil {
				// Mapping par index : 1ère audio mediainfo = 1ère audio mkvmerge
				audIdx, subIdx := 0, 0
				audTracks, subTracks := []mkvtool.Track{}, []mkvtool.Track{}
				for _, t := range info.Tracks {
					if t.Type == "audio" {
						audTracks = append(audTracks, t)
					} else if t.Type == "subtitles" {
						subTracks = append(subTracks, t)
					}
				}
				for _, mt := range mi.Media.Track {
					if mt.Type == "Audio" && audIdx < len(audTracks) {
						mediainfoByID[audTracks[audIdx].ID] = mt
						audIdx++
					} else if mt.Type == "Text" && subIdx < len(subTracks) {
						mediainfoByID[subTracks[subIdx].ID] = mt
						subIdx++
					}
				}
				wr.EventsEmit(a.ctx, "log", "✓ mediainfo : enrichi "+itoa(len(mediainfoByID))+" piste(s)")
			}
		}

		tracksPayload := make([]map[string]any, 0, len(info.Tracks))
		for _, t := range info.Tracks {
			if t.Type != "audio" && t.Type != "subtitles" {
				continue
			}
			row := map[string]any{
				"id":             t.ID,
				"type":           t.Type,
				"codec":          t.Codec,
				"language":       t.Properties.Language,
				"track_name":     t.Properties.TrackName,
				"audio_channels": t.Properties.AudioChannels,
				"codec_id":       t.Properties.CodecID,
				"default_track":  t.Properties.DefaultTrack,
				"forced_track":   t.Properties.ForcedTrack,
			}
			if mt, ok := mediainfoByID[t.ID]; ok {
				row["mi_title"] = mt.Title
				row["mi_format"] = mt.Format
				row["mi_format_profile"] = mt.FormatProfile
				row["mi_format_commercial"] = mt.FormatCommercial
				row["mi_format_commercial_if_any"] = mt.FormatCommercialIfAny
				row["mi_format_features"] = mt.FormatAdditionalFeatures
				row["mi_service_kind"] = mt.ServiceKind
				row["mi_service_kind_name"] = mt.ServiceKindNames
				row["mi_stream_size"] = mt.StreamSize
				row["mi_element_count"] = mt.ElementCount
			}
			// Pour les subs texte (SRT/ASS/SSA) en FR, extraire le contenu
			// et compter les marqueurs SDH ([bruit], (musique), ♪, "Speaker:").
			if t.Type == "subtitles" {
				lang := strings.ToLower(t.Properties.Language)
				codecID := strings.ToUpper(t.Properties.CodecID)
				isText := strings.Contains(codecID, "TEXT") || strings.Contains(codecID, "UTF") ||
					strings.Contains(codecID, "ASS") || strings.Contains(codecID, "SSA")
				isFR := lang == "fre" || lang == "fra" || lang == "fr"
				if isText && isFR {
					tmpPath, exErr := mkvtool.ExtractTrackToTemp(a.ctx, binary, path, t.ID, "srt")
					if exErr == nil {
						content, _ := os.ReadFile(tmpPath)
						os.Remove(tmpPath)
						isSDH, score := mkvtool.DetectSubSDHFromContent(string(content))
						row["sdh_detected"] = isSDH
						row["sdh_score"] = score
						wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ sub #%d FR : score SDH = %d → %s", t.ID, score, map[bool]string{true: "SDH", false: "Full"}[isSDH]))
					} else {
						wr.EventsEmit(a.ctx, "log", "⚠ extract sub #"+itoa(t.ID)+" : "+exErr.Error())
					}
				}
			}
			tracksPayload = append(tracksPayload, row)
		}
		b, _ := json.Marshal(tracksPayload)
		wr.EventsEmit(a.ctx, "secondary:tracks", string(b))
		wr.EventsEmit(a.ctx, "log", "✓ Secondaire : "+itoa(len(tracksPayload))+" piste(s) audio/sub")
	}()
}

// --- TMDB ---

func (a *App) SearchTmdb(query string) ([]tmdb.Result, error) {
	c := config.Load()
	// Si la query est un ID TMDB numérique et qu'on a une clé API, fetch direct
	q := strings.TrimSpace(query)
	if q != "" && isAllDigits(q) && c.TmdbKey != "" {
		if r, err := tmdb.FetchByID(q, c.TmdbKey); err == nil && r != nil {
			return []tmdb.Result{*r}, nil
		}
	}
	return tmdb.Search(c.ServeurPersoURL, c.FallbackIndex, query)
}

// TestHydrackerKey teste une clé API Hydracker en hitant /user-profile/me.
type ApiKeyTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func (a *App) TestHydrackerKey(key string) ApiKeyTestResult {
	ok, msg := hydracker.TestKey(strings.TrimSpace(key))
	return ApiKeyTestResult{OK: ok, Message: msg}
}

// TestUnfrKey teste une clé API UNFR en faisant un HEAD/GET sur une URL fiche
// avec la clé en header Authorization. Best-effort : l'API UNFR n'a pas
// d'endpoint /me documenté.
func (a *App) TestUnfrKey(key string) ApiKeyTestResult {
	k := strings.TrimSpace(key)
	if k == "" {
		return ApiKeyTestResult{OK: false, Message: "clé vide"}
	}
	// Test : ping la racine UNFR avec le bearer
	req, err := http.NewRequest("GET", "https://unfr.pw/?d=fiche&movieid=550", nil)
	if err != nil {
		return ApiKeyTestResult{OK: false, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+k)
	req.Header.Set("User-Agent", "GoMuxLiHDLTeam/1.0 (mux-app)")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ApiKeyTestResult{OK: false, Message: err.Error()}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		return ApiKeyTestResult{OK: true, Message: "endpoint accessible (200) ✓"}
	case 401:
		return ApiKeyTestResult{OK: false, Message: "clé invalide (401)"}
	case 403:
		return ApiKeyTestResult{OK: false, Message: "accès refusé (403)"}
	default:
		return ApiKeyTestResult{OK: false, Message: "HTTP " + resp.Status}
	}
}

// LookupHydrackerURL résout l'URL fiche Hydracker pour un ID TMDB donné.
// Retourne chaîne vide si pas de clé API, pas trouvé, ou erreur.
func (a *App) LookupHydrackerURL(tmdbID int) string {
	c := config.Load()
	if c.HydrackerKey == "" {
		return ""
	}
	url, err := hydracker.LookupURL(tmdbID, c.HydrackerKey)
	if err != nil {
		wr.EventsEmit(a.ctx, "log", "⚠ Hydracker lookup : "+err.Error())
		return ""
	}
	return url
}

// CheckVFQ vérifie si un film (par son ID TMDB) a une traduction fr-CA
// dans TMDB. Présence = signal très fort de l'existence d'un VFQ.
// Nécessite la clé API TMDB. Retourne false si pas de clé ou pas de trad.
func (a *App) CheckVFQ(tmdbID string) bool {
	c := config.Load()
	if c.TmdbKey == "" {
		return false
	}
	ok, err := tmdb.HasVFQViaTranslations(tmdbID, c.TmdbKey)
	if err != nil {
		wr.EventsEmit(a.ctx, "log", "⚠ CheckVFQ : "+err.Error())
		return false
	}
	return ok
}

// SearchTmdbMovie cherche un film via l'API TMDB officielle (nom ou ID numérique).
// Nécessite une clé API TMDB. Utilisé en mode LiHDL pour avoir une recherche
// précise et homogène avec PSA SERIES (qui utilise SearchTmdbTV).
func (a *App) SearchTmdbMovie(query string) ([]tmdb.Result, error) {
	c := config.Load()
	if c.TmdbKey == "" {
		return nil, errors.New("clé API TMDB requise pour la recherche film (Réglages)")
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if isAllDigits(q) {
		if r, err := tmdb.FetchByID(q, c.TmdbKey); err == nil && r != nil {
			return []tmdb.Result{*r}, nil
		}
	}
	return tmdb.SearchMovie(q, c.TmdbKey)
}

// SearchTmdbTV cherche une série TV via l'API TMDB (nom ou ID numérique).
// Nécessite une clé API TMDB renseignée dans Réglages.
func (a *App) SearchTmdbTV(query string) ([]tmdb.Result, error) {
	c := config.Load()
	if c.TmdbKey == "" {
		return nil, errors.New("clé API TMDB requise pour chercher des séries (Réglages)")
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if isAllDigits(q) {
		if r, err := tmdb.FetchTVByID(q, c.TmdbKey); err == nil && r != nil {
			return []tmdb.Result{*r}, nil
		}
	}
	return tmdb.SearchTV(q, c.TmdbKey)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// TestTmdbKey teste la clé API TMDB en appelant /3/configuration.
// Retourne un message de succès ou l'erreur.
type TmdbTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func (a *App) TestTmdbKey(key string) TmdbTestResult {
	if strings.TrimSpace(key) == "" {
		return TmdbTestResult{OK: false, Message: "clé vide"}
	}
	url := "https://api.themoviedb.org/3/configuration?api_key=" + key
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return TmdbTestResult{OK: false, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return TmdbTestResult{OK: true, Message: "clé valide ✓"}
	}
	if resp.StatusCode == 401 {
		return TmdbTestResult{OK: false, Message: "clé invalide (401)"}
	}
	return TmdbTestResult{OK: false, Message: "HTTP " + resp.Status}
}

// --- Build filename preview ---

func (a *App) BuildFilename(p naming.FilenameParams) string {
	return naming.BuildFilename(p)
}

func (a *App) VideoTrackName(quality, encoder, source, team string) string {
	return naming.VideoTrackName(quality, encoder, source, team)
}

// --- Mux ---

// MuxRequest est ce que le frontend envoie pour déclencher le mux.
type MuxRequest struct {
	InputPath       string                   `json:"input_path"`
	OutputPath      string                   `json:"output_path"`
	Title           string                   `json:"title"`
	Tracks          []mkvtool.TrackSpec      `json:"tracks"`
	ExternalAudios  []mkvtool.ExternalAudio  `json:"external_audios"`
	ExternalSubs    []mkvtool.ExternalSub    `json:"external_subs"`
	SecondaryPath   string                   `json:"secondary_path"`
	SecondaryAudios []mkvtool.SecondaryTrack `json:"secondary_audios"`
	SecondarySubs   []mkvtool.SecondaryTrack `json:"secondary_subs"`
	NoChapters      bool                     `json:"no_chapters"`
}

func (a *App) Mux(req MuxRequest) error {
	binary := a.LocateMkvmerge()
	if binary == "" {
		return errMkvNotFound
	}
	a.mu.Lock()
	if a.opCancel != nil {
		a.opCancel() // annule un mux précédent encore actif
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.opCtx, a.opCancel = ctx, cancel
	a.mu.Unlock()

	wr.EventsEmit(a.ctx, "log", "🔧 Lancement mkvmerge → "+filepath.Base(req.OutputPath))
	err := mkvtool.Mux(ctx, binary, mkvtool.MuxParams{
		InputPath:       req.InputPath,
		OutputPath:      req.OutputPath,
		Title:           req.Title,
		Tracks:          req.Tracks,
		ExternalAudios:  req.ExternalAudios,
		ExternalSubs:    req.ExternalSubs,
		SecondaryPath:   req.SecondaryPath,
		SecondaryAudios: req.SecondaryAudios,
		SecondarySubs:   req.SecondarySubs,
		NoChapters:      req.NoChapters,
	},
		func(p mkvtool.MuxProgress) {
			wr.EventsEmit(a.ctx, "mux:progress", p)
		},
		func(line string) {
			wr.EventsEmit(a.ctx, "log", line)
		},
	)

	a.mu.Lock()
	a.opCtx, a.opCancel = nil, nil
	a.mu.Unlock()

	if err != nil {
		wr.EventsEmit(a.ctx, "log", "❌ "+err.Error())
		wr.EventsEmit(a.ctx, "mux:done", map[string]any{"ok": false})
		return err
	}
	wr.EventsEmit(a.ctx, "log", "✅ Mux terminé : "+req.OutputPath)
	wr.EventsEmit(a.ctx, "mux:done", map[string]any{"ok": true, "path": req.OutputPath})
	return nil
}

func (a *App) CancelMux() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.opCancel != nil {
		a.opCancel()
		wr.EventsEmit(a.ctx, "log", "🛑 Mux annulé")
	}
}

// --- helpers ---

var errMkvNotFound = &mkvErr{msg: "mkvmerge introuvable"}

type mkvErr struct{ msg string }

func (e *mkvErr) Error() string { return e.msg }

func pluralTracks(n int) string {
	if n > 1 {
		return itoa(n) + " pistes"
	}
	return itoa(n) + " piste"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ============================================================================
// Auto-update : check + download + install via GitHub Releases (repo public).
// ============================================================================

const updateRepo = "Gandalfleblanc/go-mux-lihdl-team"

// UpdateInfo décrit une mise à jour disponible.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
}

type ghAssetPub struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type ghReleasePub struct {
	TagName    string       `json:"tag_name"`
	HTMLURL    string       `json:"html_url"`
	Body       string       `json:"body"`
	Draft      bool         `json:"draft"`
	Prerelease bool         `json:"prerelease"`
	Assets     []ghAssetPub `json:"assets"`
}

func fetchLatestReleasePublic() (*ghReleasePub, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+updateRepo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errMkvErr("HTTP " + resp.Status)
	}
	raw, _ := io.ReadAll(resp.Body)
	var rel ghReleasePub
	if err := json.Unmarshal(raw, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func platformAssetName() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return "GO-Mux-LiHDL-Team-macos-arm64.zip"
	case "darwin/amd64":
		return "GO-Mux-LiHDL-Team-macos-amd64.zip"
	case "windows/amd64":
		return "GO-Mux-LiHDL-Team-windows-amd64.zip"
	case "linux/amd64":
		return "GO-Mux-LiHDL-Team-linux-amd64.tar.gz"
	}
	return ""
}

// CheckUpdate retourne les infos de la dernière release si une version plus
// récente est disponible. Silencieux sur erreur (retourne UpdateInfo{}).
func (a *App) CheckUpdate() UpdateInfo {
	rel, err := fetchLatestReleasePublic()
	if err != nil || rel.Draft || rel.Prerelease || rel.TagName == "" {
		return UpdateInfo{}
	}
	if !isVersionNewer(rel.TagName, AppVersion) {
		return UpdateInfo{}
	}
	return UpdateInfo{
		Available: true,
		Version:   rel.TagName,
		URL:       rel.HTMLURL,
		Notes:     rel.Body,
	}
}

// InstallUpdate télécharge la dernière release, extrait, remplace le binaire
// en place et relance l'app. Supporté sur macOS et Windows. Linux : ouvre la
// page release (fallback manuel).
func (a *App) InstallUpdate() error {
	if runtime.GOOS == "linux" {
		wr.BrowserOpenURL(a.ctx, "https://github.com/"+updateRepo+"/releases")
		return errMkvErr("auto-install non supporté sur Linux — page ouverte")
	}
	rel, err := fetchLatestReleasePublic()
	if err != nil {
		return err
	}
	wantName := platformAssetName()
	var asset *ghAssetPub
	for i := range rel.Assets {
		if rel.Assets[i].Name == wantName {
			asset = &rel.Assets[i]
			break
		}
	}
	if asset == nil {
		return errMkvErr("asset " + wantName + " introuvable dans " + rel.TagName)
	}
	wr.EventsEmit(a.ctx, "log", "⬇️ Téléchargement "+asset.Name+"…")
	tmpDir := filepath.Join(os.TempDir(), "go-mux-lihdl-update")
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return err
	}
	archivePath := filepath.Join(tmpDir, asset.Name)
	if err := downloadTo(asset.BrowserDownloadURL, archivePath); err != nil {
		return err
	}
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return a.installDarwin(tmpDir, archivePath, execPath)
	case "windows":
		return a.installWindows(tmpDir, archivePath, execPath)
	}
	return errMkvErr("plateforme non supportée")
}

func (a *App) installDarwin(tmpDir, zipPath, execPath string) error {
	wr.EventsEmit(a.ctx, "log", "📦 Extraction…")
	if out, err := exec.Command("unzip", "-q", zipPath, "-d", tmpDir).CombinedOutput(); err != nil {
		return errMkvErr("unzip : " + string(out))
	}
	newApp := filepath.Join(tmpDir, "GO Mux LiHDL Team.app")
	if _, err := os.Stat(newApp); err != nil {
		return errMkvErr("'" + newApp + "' introuvable après unzip")
	}
	currentApp := filepath.Dir(filepath.Dir(filepath.Dir(execPath)))
	if !strings.HasSuffix(currentApp, ".app") {
		return errMkvErr("bundle courant introuvable")
	}
	scriptPath := filepath.Join(tmpDir, "install.sh")
	script := "#!/bin/sh\nsleep 1\nrm -rf \"" + currentApp + "\"\nmv \"" + newApp + "\" \"" + currentApp + "\"\nxattr -cr \"" + currentApp + "\" 2>/dev/null || true\nopen \"" + currentApp + "\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}
	wr.EventsEmit(a.ctx, "log", "🔄 Installation, l'app va redémarrer…")
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		wr.Quit(a.ctx)
	}()
	return nil
}

func (a *App) installWindows(tmpDir, zipPath, execPath string) error {
	wr.EventsEmit(a.ctx, "log", "📦 Extraction…")
	if err := unzipTo(zipPath, tmpDir); err != nil {
		return err
	}
	newExe := filepath.Join(tmpDir, "GO Mux LiHDL Team.exe")
	if _, err := os.Stat(newExe); err != nil {
		return errMkvErr("'" + newExe + "' introuvable après unzip")
	}
	batPath := filepath.Join(tmpDir, "install.bat")
	bat := "@echo off\nping -n 3 127.0.0.1 > nul\ndel /f /q \"" + execPath + "\"\nmove /y \"" + newExe + "\" \"" + execPath + "\"\nstart \"\" \"" + execPath + "\"\n"
	if err := os.WriteFile(batPath, []byte(bat), 0755); err != nil {
		return err
	}
	wr.EventsEmit(a.ctx, "log", "🔄 Installation, l'app va redémarrer…")
	cmd := exec.Command("cmd.exe", "/C", "start", "/B", "", batPath)
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		wr.Quit(a.ctx)
	}()
	return nil
}

// --- helpers updater ---

func downloadTo(url, dest string) error {
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errMkvErr("HTTP " + resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func unzipTo(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return errMkvErr("chemin zip non sûr : " + fpath)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, f.Mode())
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// isVersionNewer retourne true si tag a est strictement supérieure à tag b.
func isVersionNewer(a, b string) bool {
	pa := parseVersion(a)
	pb := parseVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return true
		}
		if pa[i] < pb[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		for _, c := range parts[i] {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			} else {
				break
			}
		}
		out[i] = n
	}
	return out
}

func errMkvErr(s string) error { return &mkvErr{msg: s} }

