package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"go-mux-lihdl-team/internal/alass"
	"go-mux-lihdl-team/internal/audiosync"
	"go-mux-lihdl-team/internal/chromaprint"
	"go-mux-lihdl-team/internal/config"
	"go-mux-lihdl-team/internal/discordindex"
	"go-mux-lihdl-team/internal/hydracker"
	"go-mux-lihdl-team/internal/mediainfo"
	"go-mux-lihdl-team/internal/mkvtool"
	"go-mux-lihdl-team/internal/naming"
	"go-mux-lihdl-team/internal/ocrsubs"
	"go-mux-lihdl-team/internal/opensubtitles"
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

	// Ouverture en plein écran adapté à la résolution courante : on définit
	// d'abord la taille de la fenêtre à celle de l'écran principal, puis on
	// la maximise pour qu'elle occupe toute la zone utilisable (hors menu bar
	// macOS et dock). S'adapte automatiquement à n'importe quelle résolution.
	if screens, err := wr.ScreenGetAll(ctx); err == nil && len(screens) > 0 {
		var primary wr.Screen
		for _, s := range screens {
			if s.IsPrimary {
				primary = s
				break
			}
		}
		if primary.Size.Width == 0 {
			primary = screens[0]
		}
		wr.WindowSetSize(ctx, primary.Size.Width, primary.Size.Height)
	}
	wr.WindowMaximise(ctx)

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
const AppVersion = "v5.6.5"

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

// SelectMkvFiles ouvre un dialog multi-sélection pour les MKV (page d'accueil → queue).
func (a *App) SelectMkvFiles() ([]string, error) {
	return wr.OpenMultipleFilesDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Choisir un ou plusieurs fichiers .mkv",
		Filters: []wr.FileFilter{
			{DisplayName: "Matroska (*.mkv)", Pattern: "*.mkv"},
		},
	})
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

// SelectSupFiles ouvre un dialog multi-sélection pour les .sup PGS uniquement
// (utilisé par l'outil OCR autonome dans Outils additionnels).
func (a *App) SelectSupFiles() ([]string, error) {
	return wr.OpenMultipleFilesDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Choisir un ou plusieurs fichiers PGS (.sup)",
		Filters: []wr.FileFilter{
			{DisplayName: "PGS (*.sup)", Pattern: "*.sup"},
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
// MoveDirContentsToTrash envoie à la corbeille tout le contenu d'un dossier
// (fichiers + sous-dossiers, sauf .DS_Store). Utilisé pour vider le dossier
// "LiHDL en cours" après un mux auto réussi.
func (a *App) MoveDirContentsToTrash(dirPath string) (int, error) {
	if dirPath == "" {
		return 0, errors.New("chemin vide")
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, err
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Name() == ".DS_Store" {
			continue
		}
		paths = append(paths, filepath.Join(dirPath, e.Name()))
	}
	if len(paths) == 0 {
		return 0, nil
	}
	if err := a.MoveToTrash(paths); err != nil {
		return 0, err
	}
	return len(paths), nil
}

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
		if mibin, mErr := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); mErr == nil {
			if mi, mErr2 := mediainfo.Identify(a.ctx, mibin, path); mErr2 == nil {
				audIdx, subIdx := 0, 0
				audTracks, subTracks := []mkvtool.Track{}, []mkvtool.Track{}
				videoTracks := []mkvtool.Track{}
				for _, t := range info.Tracks {
					if t.Type == "video" {
						videoTracks = append(videoTracks, t)
					} else if t.Type == "audio" {
						audTracks = append(audTracks, t)
					} else if t.Type == "subtitles" {
						subTracks = append(subTracks, t)
					}
				}
				vidIdx := 0
				for _, mt := range mi.Media.Track {
					if mt.Type == "Video" && vidIdx < len(videoTracks) {
						mediainfoByID[videoTracks[vidIdx].ID] = mt
						vidIdx++
					} else if mt.Type == "Audio" && audIdx < len(audTracks) {
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
				row["mi_channels"] = mt.Channels
				row["mi_service_kind"] = mt.ServiceKind
				row["mi_service_kind_name"] = mt.ServiceKindNames
				row["mi_stream_size"] = mt.StreamSize
				row["mi_element_count"] = mt.ElementCount
				// HDR (utile pour les pistes vidéo 4K).
				row["mi_hdr_format"] = mt.HDRFormat
				row["mi_hdr_compat"] = mt.HDRFormatCompatibility
				row["mi_hdr_profile"] = mt.HDRFormatProfile
				row["mi_hdr_string"] = mt.HDRFormatString
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
		if mibin, err := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); err == nil {
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
				row["mi_channels"] = mt.Channels
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

// TestLanguageToolKey teste une clé API LanguageTool Premium (ou l'endpoint
// public si key vide). Best-effort : envoie une requête minuscule.
func (a *App) TestLanguageToolKey(apiURL, key, user string) ApiKeyTestResult {
	ok, msg := ocrsubs.TestLanguageToolKey(strings.TrimSpace(apiURL), strings.TrimSpace(key), strings.TrimSpace(user))
	return ApiKeyTestResult{OK: ok, Message: msg}
}

// MkvBasicInfo : durée + FPS + scan audio d'un mkv (pour vérif compat subs/sources).
type MkvBasicInfo struct {
	DurationSeconds float64 `json:"duration_seconds"`
	Framerate       float64 `json:"framerate"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	HasVFQAudio     bool    `json:"has_vfq_audio"`  // une piste audio FR Canada détectée
	VFQTrackInfo    string  `json:"vfq_track_info"` // libellé court (ex: "fr-CA · EAC3 5.1")
}

// GetMkvBasicInfo extrait durée + framerate + présence VFQ via mediainfo + mkvmerge.
func (a *App) GetMkvBasicInfo(path string) (*MkvBasicInfo, error) {
	if path == "" {
		return nil, errors.New("chemin vide")
	}
	out := &MkvBasicInfo{}
	// 1) mediainfo : durée + framerate + dimensions
	if mibin, err := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); err == nil {
		if mi, err := mediainfo.Identify(a.ctx, mibin, path); err == nil {
			for _, t := range mi.Media.Track {
				switch t.Type {
				case "General":
					if t.Duration != "" {
						if d, err := strconv.ParseFloat(t.Duration, 64); err == nil {
							out.DurationSeconds = d
						}
					}
				case "Video":
					if t.FrameRate != "" {
						if f, err := strconv.ParseFloat(t.FrameRate, 64); err == nil {
							out.Framerate = f
						}
					}
					if t.Width != "" {
						if w, err := strconv.Atoi(t.Width); err == nil {
							out.Width = w
						}
					}
					if t.Height != "" {
						if h, err := strconv.Atoi(t.Height); err == nil {
							out.Height = h
						}
					}
				}
			}
		}
	}
	// 2) mkvmerge : scan des pistes audio pour détecter une VFQ
	if binary := a.LocateMkvmerge(); binary != "" {
		if info, err := mkvtool.Identify(a.ctx, binary, path); err == nil {
			for _, t := range info.Tracks {
				if t.Type != "audio" {
					continue
				}
				lang := strings.ToLower(t.Properties.Language)
				name := strings.ToLower(t.Properties.TrackName)
				// fr-CA explicite OU mots-clés Canada/Québec dans le nom de piste
				isFR := lang == "fre" || lang == "fra" || lang == "fr" || strings.HasPrefix(lang, "fr-")
				isCanadian := lang == "fr-ca" || strings.Contains(name, "canad") || strings.Contains(name, "québ") || strings.Contains(name, "quebec") || strings.Contains(name, "vfq")
				if isFR && isCanadian {
					out.HasVFQAudio = true
					info := strings.TrimSpace(t.Properties.TrackName)
					if info == "" {
						info = t.Codec
					}
					out.VFQTrackInfo = info
					break
				}
			}
		}
	}
	return out, nil
}

// RefSubResult décrit un sous-titre FR/ENG SRT extrait d'une source de référence,
// prêt à être ajouté à externalSubs côté frontend.
type RefSubResult struct {
	Path        string  `json:"path"`         // chemin du .srt extrait dans un fichier temporaire
	Language    string  `json:"language"`     // "FR" ou "ENG" (préfixe LiHDL)
	Forced      bool    `json:"forced"`       // flag forced du mkv source
	SDH         bool    `json:"sdh"`          // SDH détecté (FR uniquement)
	Label       string  `json:"label"`        // label LiHDL prêt (ex: "FR Full : SRT")
	DelayMs     int     `json:"delay_ms"`     // décalage à appliquer (mkvmerge --sync) si réf désynchro vs source LiHDL
	TempoFactor float64 `json:"tempo_factor"` // ratio atempo si drift linéaire (1.0 = pas de drift)
	Confidence  float64 `json:"confidence"`   // confiance de la détection sync [-1,1]
	Method      string  `json:"method"`       // "constant" | "drift_linear" | "low_confidence" | "no_sync_check"
}

// ExtractRefSubs scanne la source de référence, extrait ses pistes sous-titres
// FR et ENG en format texte (SRT/ASS/SSA — exclut PGS/VobSub), et retourne la
// liste prête à ajouter à externalSubs. Si lihdlSourcePath est fourni, détecte
// auto le décalage entre le 1er audio FR de la référence et celui de la source
// LiHDL (cross-correlation ffmpeg) et l'applique uniformément à tous les SRT.
// Pour chaque piste FR, détecte SDH via le contenu extrait. Construit le label
// LiHDL automatiquement selon les flags + détection SDH (Forced > SDH > Full).
func (a *App) ExtractRefSubs(refPath, lihdlSourcePath string) ([]RefSubResult, error) {
	emitPct := func(pct int) {
		wr.EventsEmit(a.ctx, "srtprogress", map[string]any{"percent": pct})
	}
	emitPct(0)
	if refPath == "" {
		return nil, errors.New("chemin vide")
	}
	binary := a.LocateMkvmerge()
	if binary == "" {
		return nil, errors.New("mkvmerge introuvable")
	}
	info, err := mkvtool.Identify(a.ctx, binary, refPath)
	if err != nil {
		return nil, err
	}
	emitPct(10)

	// Détection sync globale : 1ère piste audio FR de la référence vs 1ère piste
	// audio FR de la source LiHDL. Le décalage trouvé s'applique à tous les SRT
	// extraits (même réf, mêmes timecodes).
	var globalDelayMs int
	var globalConfidence float64
	var globalTempoFactor float64 = 1.0
	globalMethod := "no_sync_check"
	// Pre-sync audio cross-correlation désactivé : peu fiable (conf 0.83 = faux
	// positifs constatés en prod, le SRT était sorti "déjà alignée" alors que
	// le décalage réel était de plusieurs secondes). On laisse alass faire tout
	// le job en aval (sync auto au moment du mux), c'est l'outil dédié à ce cas
	// (offset constant + drift FPS) et il est fiable.

	// Pré-compte des subs FR/ENG texte pour calculer un % linéaire pendant la
	// boucle d'extraction (de 80% à 99%).
	totalToExtract := 0
	for _, t := range info.Tracks {
		if t.Type != "subtitles" {
			continue
		}
		lng := strings.ToLower(t.Properties.Language)
		fr := lng == "fre" || lng == "fra" || lng == "fr" || strings.HasPrefix(lng, "fr-")
		en := lng == "eng" || lng == "en" || strings.HasPrefix(lng, "en-")
		if !fr && !en {
			continue
		}
		cid := strings.ToUpper(t.Properties.CodecID)
		if strings.Contains(cid, "TEXT") || strings.Contains(cid, "UTF") || strings.Contains(cid, "ASS") || strings.Contains(cid, "SSA") {
			totalToExtract++
		}
	}
	emitPct(80)
	extractedCount := 0
	results := make([]RefSubResult, 0)
	for _, t := range info.Tracks {
		if t.Type != "subtitles" {
			continue
		}
		lang := strings.ToLower(t.Properties.Language)
		isFR := lang == "fre" || lang == "fra" || lang == "fr" || strings.HasPrefix(lang, "fr-")
		isENG := lang == "eng" || lang == "en" || strings.HasPrefix(lang, "en-")
		if !isFR && !isENG {
			continue
		}
		codecID := strings.ToUpper(t.Properties.CodecID)
		isASS := strings.Contains(codecID, "ASS") || strings.Contains(codecID, "SSA")
		isPlainSRT := strings.Contains(codecID, "TEXT") || strings.Contains(codecID, "UTF")
		isText := isASS || isPlainSRT
		if !isText {
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("ℹ sub #%d ignoré (%s, format image non extractible en SRT)", t.ID, t.Properties.CodecID))
			continue
		}
		// Étape 1 : extract dans le format natif (ass/srt selon codec).
		extOut := "srt"
		if isASS {
			extOut = "ass"
		}
		tmpPath, exErr := mkvtool.ExtractTrackToTemp(a.ctx, binary, refPath, t.ID, extOut)
		if exErr != nil {
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ extract sub #%d : %s", t.ID, exErr.Error()))
			continue
		}
		// Étape 2 : si ASS/SSA, conversion ffmpeg → SRT (norme LiHDL = SRT).
		if isASS {
			binDir, _ := config.BinDir()
			ffmpegBin, ferr := audiosync.Locate(binDir)
			if ferr != nil {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ ffmpeg introuvable, ASS sub #%d gardé tel quel : %s", t.ID, ferr.Error()))
			} else {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔄 Conversion ASS → SRT (sub #%d)…", t.ID))
				srtPath := strings.TrimSuffix(tmpPath, ".ass") + ".srt"
				cmd := exec.CommandContext(a.ctx, ffmpegBin, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", tmpPath, srtPath)
				if cout, cerr := cmd.CombinedOutput(); cerr != nil {
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ conversion ASS→SRT sub #%d : %s — %s", t.ID, cerr.Error(), string(cout)))
				} else {
					_ = os.Remove(tmpPath) // supprime le .ass intermédiaire
					tmpPath = srtPath
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ ASS converti en SRT (sub #%d)", t.ID))
				}
			}
		}
		isSDH := false
		if isFR {
			// 1. Override explicite via track_name : si l'auteur a marqué la piste
			//    SDH/HI/malentendants, on lui fait confiance immédiatement.
			tnLow := strings.ToLower(t.Properties.TrackName)
			if strings.Contains(tnLow, "sdh") ||
				strings.Contains(tnLow, "hearing impaired") ||
				strings.Contains(tnLow, "malentendant") ||
				strings.Contains(tnLow, "deaf") ||
				strings.Contains(tnLow, "sourd") {
				isSDH = true
			} else {
				// 2. Sinon analyse contenu : crochets/parenthèses descriptives,
				//    notes de musique, locuteurs en MAJUSCULES.
				content, _ := os.ReadFile(tmpPath)
				isSDH, _ = mkvtool.DetectSubSDHFromContent(string(content))
			}
		}
		var langPrefix string
		if isFR {
			// Détection VFF/VFQ via les hints du track_name + langue fr-ca explicite.
			hints := strings.ToLower(t.Properties.TrackName)
			isVFQ := strings.Contains(hints, "vfq") ||
				strings.Contains(hints, "canad") ||
				strings.Contains(hints, "québ") ||
				strings.Contains(hints, "quebec") ||
				lang == "fr-ca"
			isVFF := !isVFQ && (strings.Contains(hints, "vff") || strings.Contains(hints, "france"))
			switch {
			case isVFQ:
				langPrefix = "FR VFQ"
			case isVFF:
				langPrefix = "FR VFF"
			default:
				langPrefix = "FR"
			}
		} else {
			langPrefix = "ENG"
		}
		var variant string
		switch {
		case t.Properties.ForcedTrack:
			variant = "Forced"
		case isSDH:
			variant = "SDH"
		default:
			variant = "Full"
		}
		label := fmt.Sprintf("%s %s : SRT", langPrefix, variant)
		results = append(results, RefSubResult{
			Path:        tmpPath,
			Language:    langPrefix,
			Forced:      t.Properties.ForcedTrack,
			SDH:         isSDH,
			Label:       label,
			DelayMs:     globalDelayMs,
			TempoFactor: globalTempoFactor,
			Confidence:  globalConfidence,
			Method:      globalMethod,
		})
		extractedCount++
		if totalToExtract > 0 {
			// Progresse de 80 à 99 au fil des extractions.
			emitPct(80 + (extractedCount * 19 / totalToExtract))
		}
	}
	emitPct(100)
	return results, nil
}

// FRAudioExtraction décrit une piste audio FR (VFF ou VFQ) extraite d'un fichier
// tiers, prête à être ajoutée à externalAudios côté frontend. Inclut le délai
// de synchronisation auto-détecté vs la source LiHDL.
type FRAudioExtraction struct {
	Path         string  `json:"path"`         // chemin du fichier audio extrait (temp)
	Variant      string  `json:"variant"`      // "VFF" ou "VFQ"
	Codec        string  `json:"codec"`        // "AC3", "EAC3", "DTS", "TRUEHD", etc.
	CodecID      string  `json:"codec_id"`     // codec_id mkvmerge brut (ex: A_EAC3)
	Channels     int     `json:"channels"`     // 2, 6, 8…
	TrackName    string  `json:"track_name"`   // nom de piste source (pour hints atmos)
	Language     string  `json:"language"`     // "fre" ou "fr-ca"
	DelayMs      int     `json:"delay_ms"`     // décalage détecté (mkvmerge --sync)
	TempoFactor  float64 `json:"tempo_factor"` // ratio atempo si drift linéaire (1.0 = pas de drift)
	Confidence   float64 `json:"confidence"`
	Method       string  `json:"method"` // "constant" | "drift_linear" | "low_confidence"…
	Notes        string  `json:"notes"`
	WasConverted bool    `json:"was_converted"` // true si ffmpeg→AC3, false si extraction lossless (déjà AC3)
	BitrateKbps  int     `json:"bitrate_kbps"`  // bitrate AC3 utilisé (96, 192, 256, 448)
	// Champs mediainfo pour atmos detection côté frontend (inferAudioLabel).
	MITitle                 string `json:"mi_title"`
	MIFormat                string `json:"mi_format"`
	MIFormatProfile         string `json:"mi_format_profile"`
	MIFormatCommercial      string `json:"mi_format_commercial"`
	MIFormatCommercialIfAny string `json:"mi_format_commercial_if_any"`
	MIFormatFeatures        string `json:"mi_format_features"`
	MIChannels              string `json:"mi_channels"`
	MIServiceKind           string `json:"mi_service_kind"`
	MIServiceKindName       string `json:"mi_service_kind_name"`
}

// codecIDToExt mappe un codec_id mkvmerge à l'extension de fichier appropriée
// pour mkvextract (pour que mkvmerge re-mux ensuite reconnaisse le format).
func codecIDToExt(codecID string) string {
	switch strings.ToUpper(codecID) {
	case "A_AC3":
		return "ac3"
	case "A_EAC3":
		return "eac3"
	case "A_DTS":
		return "dts"
	case "A_TRUEHD":
		return "thd"
	case "A_AAC":
		return "aac"
	case "A_FLAC":
		return "flac"
	case "A_OPUS":
		return "opus"
	case "A_VORBIS":
		return "ogg"
	case "A_PCM/INT/LIT", "A_PCM/INT/BIG":
		return "wav"
	default:
		return "audio"
	}
}

// codecIDToLabel mappe un codec_id mkvmerge au libellé court LiHDL.
func codecIDToLabel(codecID string) string {
	switch strings.ToUpper(codecID) {
	case "A_AC3":
		return "AC3"
	case "A_EAC3":
		return "EAC3"
	case "A_DTS":
		return "DTS"
	case "A_TRUEHD":
		return "TRUEHD"
	case "A_AAC":
		return "AAC"
	case "A_FLAC":
		return "FLAC"
	case "A_OPUS":
		return "OPUS"
	default:
		return codecID
	}
}

// pickFirstFRAudioID retourne l'ID de la 1ère piste audio FR dans info, ou -1.
// Utilisé pour la détection de sync (référence sur la source LiHDL).
func pickFirstFRAudioID(info *mkvtool.Info) int {
	return pickFirstAudioIDByLang(info, "fr")
}

// pickFirstAudioIDByLang retourne l'ID de la 1ère piste audio dans la langue
// spécifiée ("fr" ou "eng") ; fallback sur la 1ère audio si aucune match.
func pickFirstAudioIDByLang(info *mkvtool.Info, lang string) int {
	lang = strings.ToLower(lang)
	for _, t := range info.Tracks {
		if t.Type != "audio" {
			continue
		}
		l := strings.ToLower(t.Properties.Language)
		name := strings.ToLower(t.Properties.TrackName)
		switch lang {
		case "fr-ca", "vfq":
			// FR VFQ strict : code fr-ca explicite OU mots-clés Canada/Québec/VFQ.
			if l == "fr-ca" || strings.Contains(name, "vfq") || strings.Contains(name, "canad") || strings.Contains(name, "québ") || strings.Contains(name, "quebec") {
				return t.ID
			}
		case "fr", "fre", "fra":
			if l == "fre" || l == "fra" || l == "fr" || strings.HasPrefix(l, "fr-") {
				return t.ID
			}
		case "eng", "en":
			if l == "eng" || l == "en" || strings.HasPrefix(l, "en-") {
				return t.ID
			}
		}
	}
	// Pas de match : fallback sur la 1ère audio.
	for _, t := range info.Tracks {
		if t.Type == "audio" {
			return t.ID
		}
	}
	return -1
}

// pickSyncRefAudioID choisit une piste audio de la source LiHDL pour servir de
// référence à la sync, en excluant les pistes qui vont être remplacées par
// l'extraction. Si l'utilisateur extrait FR VFF ET FR VFQ, on prend la 1ère
// piste non-FR (typiquement ENG VO). Si seul VFF est extrait, FR VFQ peut
// servir de réf. Si seul VFQ est extrait, FR VFF peut servir de réf.
// Fallback : 1ère audio quelconque, puis -1.
func pickSyncRefAudioID(info *mkvtool.Info, wantVFF, wantVFQ bool) int {
	classify := func(t mkvtool.Track) string {
		lang := strings.ToLower(t.Properties.Language)
		isFR := lang == "fre" || lang == "fra" || lang == "fr" || strings.HasPrefix(lang, "fr-")
		if !isFR {
			return "OTHER"
		}
		hints := strings.ToLower(t.Properties.TrackName)
		if lang == "fr-ca" || strings.Contains(hints, "vfq") || strings.Contains(hints, "canad") || strings.Contains(hints, "québ") || strings.Contains(hints, "quebec") {
			return "VFQ"
		}
		return "VFF"
	}
	// Priorité 1 : piste non-FR (ENG VO, JPN VO, etc.) — ne sera jamais remplacée.
	for _, t := range info.Tracks {
		if t.Type != "audio" {
			continue
		}
		if classify(t) == "OTHER" {
			return t.ID
		}
	}
	// Priorité 2 : piste FR qui n'est pas remplacée (l'autre variante).
	for _, t := range info.Tracks {
		if t.Type != "audio" {
			continue
		}
		c := classify(t)
		if c == "VFF" && !wantVFF {
			return t.ID
		}
		if c == "VFQ" && !wantVFQ {
			return t.ID
		}
	}
	// Fallback : 1ère piste audio (peut être celle qu'on remplace, mais mieux que rien).
	for _, t := range info.Tracks {
		if t.Type == "audio" {
			return t.ID
		}
	}
	return -1
}

// classifyFRAudio détermine si une piste audio est FR VFF, FR VFQ ou ENG d'après
// la langue et les hints (track_name, mediainfo). Retourne "VFF", "VFQ", "ENG" ou "".
func classifyFRAudio(t mkvtool.Track) string {
	lang := strings.ToLower(t.Properties.Language)
	isFR := lang == "fre" || lang == "fra" || lang == "fr" || strings.HasPrefix(lang, "fr-")
	isENG := lang == "eng" || lang == "en" || strings.HasPrefix(lang, "en-")
	if isENG {
		return "ENG"
	}
	if !isFR {
		return ""
	}
	hints := strings.ToLower(t.Properties.TrackName)
	isCanada := lang == "fr-ca" ||
		strings.Contains(hints, "canad") ||
		strings.Contains(hints, "québ") ||
		strings.Contains(hints, "quebec") ||
		strings.Contains(hints, "vfq")
	if isCanada {
		return "VFQ"
	}
	return "VFF"
}

// ExtractFRAudios extrait les pistes FR VFF, FR VFQ et/ou ENG VO d'un fichier
// source tiers, détecte le décalage par rapport à la source LiHDL, et retourne
// les fichiers audio prêts à être ajoutés au mux comme externalAudios. Le
// frontend gère ensuite le replace (drop des FR/ENG existants) et l'ajout.
//
// Convention : toutes les pistes non-AC3 (EAC3, DTS, TrueHD, FLAC, etc.) sont
// converties en AC3 via ffmpeg pour respecter la norme LiHDL :
//   - 1.0 → AC3 1.0 (96k)
//   - 2.0 → AC3 2.0 (192k)
//   - 5.1 → AC3 5.1 (384k)
//   - 7.1 → AC3 5.1 (384k, downmix)
func (a *App) ExtractFRAudios(srcPath string, wantVFF, wantVFQ, wantENG bool, lihdlSourcePath, syncRefLang string) ([]FRAudioExtraction, error) {
	if srcPath == "" {
		return nil, errors.New("chemin source FR vide")
	}
	if !wantVFF && !wantVFQ && !wantENG {
		return nil, errors.New("aucune variante demandée (cocher VFF, VFQ ou ENG)")
	}
	binary := a.LocateMkvmerge()
	if binary == "" {
		return nil, errors.New("mkvmerge introuvable")
	}

	srcInfo, err := mkvtool.Identify(a.ctx, binary, srcPath)
	if err != nil {
		return nil, fmt.Errorf("analyse source FR : %w", err)
	}

	// Mediainfo enrichi pour atmos/channels (best-effort) + durée source ref pour
	// la barre de progression de la conversion AC3.
	mediainfoByID := map[int]mediainfo.Track{}
	var srcDuration float64
	if mibin, err := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); err == nil {
		if mi, err := mediainfo.Identify(a.ctx, mibin, srcPath); err == nil {
			audIdx := 0
			audTracks := []mkvtool.Track{}
			for _, t := range srcInfo.Tracks {
				if t.Type == "audio" {
					audTracks = append(audTracks, t)
				}
			}
			for _, mt := range mi.Media.Track {
				if mt.Type == "Audio" && audIdx < len(audTracks) {
					mediainfoByID[audTracks[audIdx].ID] = mt
					audIdx++
				}
				if mt.Type == "General" && mt.Duration != "" && srcDuration == 0 {
					if d, derr := strconv.ParseFloat(mt.Duration, 64); derr == nil {
						srcDuration = d
					}
				}
			}
		}
	}

	// Pour la sync : 1ère piste FR de la source LiHDL en priorité (cross-correlation
	// FR vs FR donne de meilleurs résultats qu'avec ENG/autre langue), fallback
	// sur 1ère audio quelconque sinon.
	var lihdlFRID int = -1
	var lihdlDuration float64
	if lihdlSourcePath != "" {
		if lihdlInfo, err := mkvtool.Identify(a.ctx, binary, lihdlSourcePath); err == nil {
			lihdlFRID = pickFirstAudioIDByLang(lihdlInfo, syncRefLang)
		}
		// Durée pour détection de drift.
		if mibin, err := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); err == nil {
			if mi, err := mediainfo.Identify(a.ctx, mibin, lihdlSourcePath); err == nil {
				for _, t := range mi.Media.Track {
					if t.Type == "General" && t.Duration != "" {
						if d, perr := strconv.ParseFloat(t.Duration, 64); perr == nil {
							lihdlDuration = d
						}
						break
					}
				}
			}
		}
	}

	binDir, _ := config.BinDir()
	ffmpeg, _ := audiosync.Locate(binDir)
	canSync := ffmpeg != "" && lihdlFRID >= 0 && lihdlSourcePath != ""

	results := make([]FRAudioExtraction, 0, 3)
	doneVFF, doneVFQ, doneENG := false, false, false
	for _, t := range srcInfo.Tracks {
		if t.Type != "audio" {
			continue
		}
		variant := classifyFRAudio(t)
		if variant == "" {
			continue
		}
		if variant == "VFF" && (!wantVFF || doneVFF) {
			continue
		}
		if variant == "VFQ" && (!wantVFQ || doneVFQ) {
			continue
		}
		if variant == "ENG" && (!wantENG || doneENG) {
			continue
		}

		// Choix : extraction lossless si déjà AC3, sinon conversion ffmpeg → AC3.
		srcCodec := codecIDToLabel(t.Properties.CodecID)
		isAlreadyAC3 := strings.ToUpper(srcCodec) == "AC3"
		var tmpPath string
		var exErr error
		if isAlreadyAC3 {
			tmpPath, exErr = mkvtool.ExtractTrackToTemp(a.ctx, binary, srcPath, t.ID, "ac3")
			if exErr != nil {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ extract audio #%d (%s) : %s", t.ID, variant, exErr.Error()))
				continue
			}
		} else {
			if ffmpeg == "" {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ piste #%d (%s) en %s : ffmpeg requis pour conversion AC3, skip", t.ID, variant, srcCodec))
				continue
			}
			tmp, terr := os.CreateTemp("", "submux-extract-*.ac3")
			if terr != nil {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ création temp file : %s", terr.Error()))
				continue
			}
			tmpPath = tmp.Name()
			tmp.Close()
			os.Remove(tmpPath)
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔄 Conversion %s → AC3 (piste #%d, %s, %d ch)…", srcCodec, t.ID, variant, t.Properties.AudioChannels))
			progressCb := func(pct int) {
				wr.EventsEmit(a.ctx, "ac3convert:progress", map[string]any{"variant": variant, "percent": pct})
			}
			if cerr := audiosync.ConvertAudioToAC3(a.ctx, ffmpeg, srcPath, t.ID, t.Properties.AudioChannels, tmpPath, srcDuration, progressCb); cerr != nil {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("❌ conversion audio #%d (%s) : %s", t.ID, variant, cerr.Error()))
				continue
			}
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ %s converti en AC3 (piste #%d, %s)", srcCodec, t.ID, variant))
		}

		// Le codec final est toujours AC3 (extrait tel quel ou converti).
		finalCodec := "AC3"
		// Channels finaux : downmix 7.1→5.1 si conversion
		finalChannels := t.Properties.AudioChannels
		if !isAlreadyAC3 && finalChannels > 6 {
			finalChannels = 6
		}
		// Bitrate cible (uniquement si converti — lossless conserve le bitrate source).
		finalBitrate := 0
		if !isAlreadyAC3 {
			switch {
			case finalChannels == 1:
				finalBitrate = 96
			case finalChannels == 2:
				finalBitrate = 192
			case finalChannels >= 5:
				finalBitrate = 448
			default:
				finalBitrate = 256
			}
		}

		extraction := FRAudioExtraction{
			Path:         tmpPath,
			Variant:      variant,
			CodecID:      "A_AC3",
			Codec:        finalCodec,
			Channels:     finalChannels,
			TrackName:    t.Properties.TrackName,
			Language:     t.Properties.Language,
			WasConverted: !isAlreadyAC3,
			BitrateKbps:  finalBitrate,
		}
		if mt, ok := mediainfoByID[t.ID]; ok {
			extraction.MITitle = mt.Title
			extraction.MIFormat = mt.Format
			extraction.MIFormatProfile = mt.FormatProfile
			extraction.MIFormatCommercial = mt.FormatCommercial
			extraction.MIFormatCommercialIfAny = mt.FormatCommercialIfAny
			extraction.MIFormatFeatures = mt.FormatAdditionalFeatures
			extraction.MIChannels = mt.Channels
			extraction.MIServiceKind = mt.ServiceKind
			extraction.MIServiceKindName = mt.ServiceKindNames
		}

		// Détection sync (best-effort — si ffmpeg manquant ou pas de réf FR, skip).
		extraction.TempoFactor = 1.0
		// Pré-calcul du tempo basé sur le FPS : si la source de la piste audio
		// (référence VF2) et la source LiHDL ont des FPS différents, l'audio
		// extraite doit être resampled (sinon drift progressif). On l'applique
		// AUTOMATIQUEMENT, indépendamment de la confiance audio cross-corr (qui
		// est souvent < 0.4 quand il y a justement un drift FPS, donc bloquait).
		if mibinTempo, _ := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); mibinTempo != "" {
			srcFps := getMediaFPS(a.ctx, mibinTempo, srcPath)
			lihdlFps := getMediaFPS(a.ctx, mibinTempo, lihdlSourcePath)
			if srcFps > 0 && lihdlFps > 0 && math.Abs(srcFps-lihdlFps) > 0.05 {
				extraction.TempoFactor = lihdlFps / srcFps
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("📐 %s : FPS différents (réf %.3f vs LiHDL %.3f) → tempo audio ×%.6f", variant, srcFps, lihdlFps, extraction.TempoFactor))
				// APPLIQUE le tempo au fichier audio extrait via ffmpeg atempo.
				// Sans ça, TempoFactor reste juste métadata et l'audio dérive.
				if ffmpeg != "" {
					resampledPath := tmpPath + ".tempo.ac3"
					rb := finalBitrate
					if rb == 0 {
						switch {
						case finalChannels >= 5:
							rb = 448
						case finalChannels == 2:
							rb = 192
						default:
							rb = 256
						}
					}
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔄 %s : application atempo=%.6f via ffmpeg…", variant, extraction.TempoFactor))
					if rerr := audiosync.ResampleAudioFile(a.ctx, ffmpeg, tmpPath, resampledPath, "ac3", finalChannels, rb, extraction.TempoFactor); rerr != nil {
						wr.EventsEmit(a.ctx, "log", fmt.Sprintf("❌ %s resample : %s", variant, rerr.Error()))
					} else {
						os.Remove(tmpPath)
						tmpPath = resampledPath
						extraction.Path = tmpPath
						extraction.WasConverted = true
						extraction.BitrateKbps = rb
						extraction.TempoFactor = 1.0 // baked dans le fichier
						wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ %s : audio resampled (durée corrigée pour FPS source)", variant))
					}
				}
			}
		}
		// Détection offset via CHROMAPRINT : analyse les fingerprints spectraux
		// haut-niveau (musique, ambiance), robuste aux voix différentes (VFQ vs
		// VFF). Beaucoup plus fiable que la cross-correlation RMS classique
		// quand les dialogues diffèrent. Doit être lancé APRÈS le resample
		// éventuel (pour que les durées matchent).
		offsetDetected := false
		fpcalcPath, fpErr := chromaprint.Locate("", binDir)
		if fpErr != nil {
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("ℹ Chromaprint %s : fpcalc introuvable (%s) — fallback cross-corr", variant, fpErr.Error()))
		} else if lihdlSourcePath == "" || lihdlFRID < 0 {
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("ℹ Chromaprint %s : pas de source LiHDL/piste FR de réf — fallback cross-corr", variant))
		} else {
			lihdlAudioPath, eerr := mkvtool.ExtractTrackToTemp(a.ctx, binary, lihdlSourcePath, lihdlFRID, "ac3")
			if eerr != nil {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ Chromaprint %s : extract LiHDL audio échoué : %s", variant, eerr.Error()))
			} else {
				defer os.Remove(lihdlAudioPath)
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🎵 Chromaprint %s : fingerprint en cours… (fpcalc=%s)", variant, fpcalcPath))
				fpA, eA := chromaprint.Fingerprint(a.ctx, fpcalcPath, lihdlAudioPath, 600)
				if eA != nil {
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ Chromaprint %s : fingerprint LiHDL échoué : %s", variant, eA.Error()))
				}
				fpB, eB := chromaprint.Fingerprint(a.ctx, fpcalcPath, tmpPath, 600)
				if eB != nil {
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ Chromaprint %s : fingerprint VFQ échoué : %s", variant, eB.Error()))
				}
				if eA == nil && eB == nil && len(fpA) > 30 && len(fpB) > 30 {
					offsetMs, conf, overlap := chromaprint.FindOffset(fpA, fpB, 480) // ±60s
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🎵 Chromaprint %s : %d hash A / %d hash B → offset %d ms, conf %.2f, overlap %d", variant, len(fpA), len(fpB), int(offsetMs), conf, overlap))
					if conf >= 0.5 {
						extraction.DelayMs = int(offsetMs)
						extraction.Confidence = conf
						extraction.Method = "chromaprint"
						offsetDetected = true
						wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ Chromaprint %s : offset %d ms appliqué (conf %.2f)", variant, extraction.DelayMs, conf))
					} else {
						wr.EventsEmit(a.ctx, "log", fmt.Sprintf("ℹ Chromaprint %s : conf %.2f trop faible (<0.5), fallback cross-corr", variant, conf))
					}
				} else if eA == nil && eB == nil {
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("ℹ Chromaprint %s : fingerprints trop courts (A=%d, B=%d hashes) — fallback", variant, len(fpA), len(fpB)))
				}
			}
		}
		// Fallback cross-correlation RMS si Chromaprint indisponible ou confidence trop basse.
		if !offsetDetected && canSync {
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔎 Détection sync %s…", variant))
			res, syncErr := audiosync.DetectOffsetCross(a.ctx, ffmpeg, lihdlSourcePath, lihdlFRID, srcPath, t.ID, lihdlDuration)
			if syncErr != nil {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ sync %s : %s", variant, syncErr.Error()))
			} else if res != nil {
				extraction.Confidence = res.Confidence
				extraction.Method = res.Method
				extraction.Notes = res.Notes
				if res.Confidence >= 0.4 {
					extraction.DelayMs = res.OffsetMs
					if res.Method == "drift_linear" && res.TempoFactor > 0 {
						extraction.TempoFactor = res.TempoFactor
					}
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ %s : offset %d ms + tempo %.6f (conf %.2f, %s)", variant, res.OffsetMs, extraction.TempoFactor, res.Confidence, res.Method))
				} else {
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ %s : décalage %d ms détecté mais confidence trop faible (%.2f) — pas d'application auto.", variant, res.OffsetMs, res.Confidence))
				}
			}
		}

		results = append(results, extraction)
		switch variant {
		case "VFF":
			doneVFF = true
		case "VFQ":
			doneVFQ = true
		case "ENG":
			doneENG = true
		}
	}

	if len(results) == 0 {
		wr.EventsEmit(a.ctx, "log", "ℹ Aucune piste FR VFF/VFQ correspondant aux choix trouvée.")
	}
	return results, nil
}

// SubSyncRequest : un SRT à synchroniser via alass-cli.
//
// FromReference : true si ce SRT a été extrait de la référence (timeline =
// FPS de la ref, donc faut alass FPS guessing pour drift si FPS différents).
// false si SRT externe ajouté manuellement par le user (typiquement déjà
// adapté à la source LiHDL, donc faut JUSTE l'offset, PAS de FPS guessing).
type SubSyncRequest struct {
	Path          string `json:"path"`
	FromReference bool   `json:"from_reference"`
}

// SubSyncCheck décrit le résultat d'une synchronisation alass entre un SRT et
// la vidéo source LiHDL. Si OK, le SRT a été corrigé dans SyncedPath.
type SubSyncCheck struct {
	Path       string `json:"path"`        // chemin du SRT original
	SyncedPath string `json:"synced_path"` // chemin du SRT corrigé (à utiliser dans le mux)
	OffsetMs   int    `json:"offset_ms"`   // décalage moyen détecté/appliqué (informatif)
	FpsRatio   string `json:"fps_ratio"`   // ex "25/23.976" si drift FPS détecté, sinon ""
	Error      string `json:"error"`       // message d'erreur si l'opération a planté
}

// CheckSubsSync utilise alass-cli pour resynchroniser chaque SRT vers la vidéo
// source LiHDL (VAD + alignment local). Produit un SRT corrigé par entrée. Le
// frontend remplace ensuite le path du SRT par le chemin SyncedPath pour le mux.
// Émet des events `subsync:progress {percent, current}` pendant le traitement.
func (a *App) CheckSubsSync(reqs []SubSyncRequest, sourceMkvPath, referenceMkvPath, syncRefLang string) ([]SubSyncCheck, error) {
	emitProg := func(pct int, current string) {
		wr.EventsEmit(a.ctx, "subsync:progress", map[string]any{"percent": pct, "current": current})
	}
	emitProg(0, "")
	if sourceMkvPath == "" {
		return nil, errors.New("source LiHDL non chargée — impossible de vérifier la sync")
	}
	if len(reqs) == 0 {
		return []SubSyncCheck{}, nil
	}
	binDir, _ := config.BinDir()
	alassPath, aerr := alass.Locate(binDir)
	if aerr != nil {
		return nil, fmt.Errorf("alass-cli : %w", aerr)
	}
	// Extrait aussi ffmpeg + ffprobe vers binDir (alass shellout vers ffprobe).
	if _, ferr := audiosync.Locate(binDir); ferr != nil {
		return nil, fmt.Errorf("ffmpeg : %w", ferr)
	}
	if _, ferr := audiosync.LocateFfprobe(binDir); ferr != nil {
		return nil, fmt.Errorf("ffprobe : %w", ferr)
	}

	// Détection FPS : décide PAR SRT si alass doit corriger un drift FPS.
	// - FPS source/ref identiques → guessing désactivé (sinon alass invente)
	// - FPS différents :
	//     - SRT extrait de la référence (FromReference=true) → guessing actif
	//       (timecodes au FPS de la ref, faut le tempo pour matcher la source)
	//     - SRT externe manuel (FromReference=false) → guessing désactivé
	//       (typiquement déjà adapté à la source LiHDL → juste l'offset)
	fpsSame := true
	if referenceMkvPath != "" {
		mibin, _ := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }())
		if mibin != "" {
			srcFps := getMediaFPS(a.ctx, mibin, sourceMkvPath)
			refFps := getMediaFPS(a.ctx, mibin, referenceMkvPath)
			if srcFps > 0 && refFps > 0 {
				if math.Abs(srcFps-refFps) < 0.05 {
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("📐 FPS identiques (%.3f) → offset uniquement, pas de tempo SRT", srcFps))
				} else {
					fpsSame = false
					wr.EventsEmit(a.ctx, "log", fmt.Sprintf("📐 FPS différents (réf %.3f vs source %.3f) → tempo + offset sur SRT extraits, offset seul sur SRT externes", refFps, srcFps))
				}
			}
		}
	}
	// Helper : doit-on désactiver le FPS guessing alass pour ce SRT ?
	fpsGuessDisabledForReq := func(req SubSyncRequest) bool {
		if fpsSame {
			return true
		}
		return !req.FromReference // SRT externe → pas de tempo, juste offset
	}

	// Identifie le SRT le plus long = "principal" (typiquement le Full). alass
	// a besoin de beaucoup de subs pour s'aligner ; sur les SRT courts comme
	// les Forced (~30 lignes), il sort des offsets délirants (+2min observé).
	// On run alass UNIQUEMENT sur le principal, puis on applique son offset
	// aux courts via shift SRT manuel. Tous les SRT de la même référence ont
	// la même timeline → même offset s'applique.
	const shortSRTThreshold = 300
	type subEntry struct {
		req       SubSyncRequest
		count     int
		origIdx   int
		inputPath string
	}
	entries := make([]subEntry, len(reqs))
	for i, r := range reqs {
		entries[i] = subEntry{req: r, count: countSRTEntries(r.Path), origIdx: i, inputPath: r.Path}
	}
	// Indice du SRT le plus long (référence d'offset).
	mainIdx := 0
	for i := range entries {
		if entries[i].count > entries[mainIdx].count {
			mainIdx = i
		}
	}

	results := make([]SubSyncCheck, len(reqs))
	total := len(reqs)
	mainOffsetMs := 0
	mainFpsRatio := ""
	mainSyncOK := false

	// Step 1 : alass sur le SRT principal.
	mainReq := entries[mainIdx].req
	emitProg(0, filepath.Base(mainReq.Path))
	lower := strings.ToLower(mainReq.Path)
	if !strings.HasSuffix(lower, ".srt") && !strings.HasSuffix(lower, ".ass") && !strings.HasSuffix(lower, ".ssa") {
		results[mainIdx] = SubSyncCheck{Path: mainReq.Path, Error: "format non supporté par alass (SRT/ASS/SSA uniquement)"}
	} else {
		wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔎 alass multi-split principal (%d lignes) : %s", entries[mainIdx].count, filepath.Base(mainReq.Path)))
		outputPath := strings.TrimSuffix(mainReq.Path, filepath.Ext(mainReq.Path)) + ".alass" + filepath.Ext(mainReq.Path)
		// noSplit=false : multi-split pour gérer un drift non-linéaire (decalage
		// qui s'accumule en cours d'épisode, intro coupée, scènes différentes).
		// Un offset constant ne suffit pas dans ces cas.
		res, err := alass.Sync(a.ctx, alassPath, sourceMkvPath, entries[mainIdx].inputPath, outputPath, false, fpsGuessDisabledForReq(mainReq), binDir)
		if err != nil {
			results[mainIdx] = SubSyncCheck{Path: mainReq.Path, Error: err.Error()}
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ alass %s : %s", filepath.Base(mainReq.Path), err.Error()))
		} else {
			results[mainIdx] = SubSyncCheck{Path: mainReq.Path, SyncedPath: res.OutputPath, OffsetMs: res.OffsetMs, FpsRatio: res.FpsRatio}
			mainOffsetMs = res.OffsetMs
			mainFpsRatio = res.FpsRatio
			mainSyncOK = true
			extra := ""
			if res.FpsRatio != "" {
				extra = ", FPS ratio " + res.FpsRatio
			}
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("  → %s : décalage %d ms appliqué%s", filepath.Base(mainReq.Path), res.OffsetMs, extra))
		}
	}

	// Step 2 : pour les autres SRT, alass si suffisamment long, sinon shift manuel
	// avec l'offset du principal (évite les faux positifs alass sur SRT courts).
	for i, e := range entries {
		if i == mainIdx {
			continue
		}
		emitProg((i*100)/total, filepath.Base(e.req.Path))
		lower := strings.ToLower(e.req.Path)
		if !strings.HasSuffix(lower, ".srt") && !strings.HasSuffix(lower, ".ass") && !strings.HasSuffix(lower, ".ssa") {
			results[i] = SubSyncCheck{Path: e.req.Path, Error: "format non supporté par alass (SRT/ASS/SSA uniquement)"}
			continue
		}
		outputPath := strings.TrimSuffix(e.req.Path, filepath.Ext(e.req.Path)) + ".alass" + filepath.Ext(e.req.Path)
		// SRT court (ex: Forced, ~30 lignes) → shift manuel basé sur le principal
		// si l'alass principal a réussi. Sinon fallback alass.
		if e.count < shortSRTThreshold && mainSyncOK {
			if err := shiftSRTFile(e.inputPath, outputPath, mainOffsetMs); err != nil {
				results[i] = SubSyncCheck{Path: e.req.Path, Error: "shift SRT court : " + err.Error()}
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ shift court %s : %s", filepath.Base(e.req.Path), err.Error()))
				continue
			}
			results[i] = SubSyncCheck{Path: e.req.Path, SyncedPath: outputPath, OffsetMs: mainOffsetMs, FpsRatio: mainFpsRatio}
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("  → %s : %d lignes, shift %d ms appliqué (basé sur le SRT principal)", filepath.Base(e.req.Path), e.count, mainOffsetMs))
			continue
		}
		// Sinon : alass multi-split standard (gère drift non-linéaire).
		wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔎 alass multi-split (%d lignes) : %s", e.count, filepath.Base(e.req.Path)))
		res, err := alass.Sync(a.ctx, alassPath, sourceMkvPath, e.inputPath, outputPath, false, fpsGuessDisabledForReq(e.req), binDir)
		if err != nil {
			results[i] = SubSyncCheck{Path: e.req.Path, Error: err.Error()}
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ alass %s : %s", filepath.Base(e.req.Path), err.Error()))
			continue
		}
		results[i] = SubSyncCheck{Path: e.req.Path, SyncedPath: res.OutputPath, OffsetMs: res.OffsetMs, FpsRatio: res.FpsRatio}
		extra := ""
		if res.FpsRatio != "" {
			extra = ", FPS ratio " + res.FpsRatio
		}
		wr.EventsEmit(a.ctx, "log", fmt.Sprintf("  → %s : décalage %d ms appliqué%s", filepath.Base(e.req.Path), res.OffsetMs, extra))
	}
	emitProg(100, "")
	return results, nil
}

// SyncedSubResult décrit un sous-titre extrait du SUPPLY et synchronisé avec
// la PSA via alass. Utilisé par le mode PSA pour récupérer les SRT FR du
// SUPPLY tout en les alignant sur l'audio PSA.
type SyncedSubResult struct {
	Path     string `json:"path"`     // chemin du .srt corrigé par alass (ou brut si alass échoue)
	Label    string `json:"label"`    // ex: "FR Full : SRT", "FR Forced : SRT"
	Language string `json:"language"` // code ISO ex: "fre"
	Default  bool   `json:"default"`
	Forced   bool   `json:"forced"`
	OffsetMs int    `json:"offset_ms"` // décalage appliqué par alass
	Synced   bool   `json:"synced"`    // true si alass a réussi
}

// SyncSupplySubsToPSA extrait les subs FR/ENG du SUPPLY et les synchronise
// sur l'audio PSA via alass + chromaprint. Retourne les paths .srt corrigés
// à ajouter comme externalSubs côté frontend.
// SyncSupplySubsToPSA extrait les subs FR/ENG du SUPPLY et les synchronise sur
// PSA. Stratégie (façon Subtitle Edit) :
//  1. Toujours tenter alass MULTI-SPLIT sub→sub avec une ref PSA texte si dispo
//     (sub→audio en fallback si la PSA n'a aucun sub texte). Multi-split gère
//     les décalages variables (intro coupée, scènes différentes).
//  2. Si alass échoue ET qu'on a offset/tempo audio → tempoShift comme filet.
//  3. Sinon → sub brut.
func (a *App) SyncSupplySubsToPSA(psaMkvPath, supplyMkvPath string, offsetMs int, tempoRatio float64) ([]SyncedSubResult, error) {
	if psaMkvPath == "" || supplyMkvPath == "" {
		return nil, errors.New("chemins PSA ou SUPPLY manquants")
	}
	binary := a.LocateMkvmerge()
	if binary == "" {
		return nil, errors.New("mkvmerge introuvable")
	}
	binDir, _ := config.BinDir()
	if tempoRatio == 0 {
		tempoRatio = 1.0
	}
	hasAudioTransform := offsetMs != 0 || tempoRatio != 1.0
	alassPath, aerr := alass.Locate(binDir)
	if aerr != nil {
		return nil, fmt.Errorf("alass-cli : %w", aerr)
	}
	if _, ferr := audiosync.Locate(binDir); ferr != nil {
		return nil, fmt.Errorf("ffmpeg : %w", ferr)
	}
	if _, ferr := audiosync.LocateFfprobe(binDir); ferr != nil {
		return nil, fmt.Errorf("ffprobe : %w", ferr)
	}
	supplyInfo, err := mkvtool.Identify(a.ctx, binary, supplyMkvPath)
	if err != nil {
		return nil, fmt.Errorf("analyse SUPPLY : %w", err)
	}
	// Stratégie : alass sub→audio_PSA. Le VAD (Voice Activity Detection)
	// sur l'audio est la vérité terrain — bien plus fiable que sub→sub
	// (qui aligne juste des timestamps de blocs sans regarder le contenu).
	// alass se charge d'extraire l'audio via ffmpeg en interne.
	psaRefForAlass := psaMkvPath
	results := []SyncedSubResult{}
	for _, t := range supplyInfo.Tracks {
		if t.Type != "subtitles" {
			continue
		}
		lang := strings.ToLower(t.Properties.Language)
		isFR := lang == "fre" || lang == "fra" || lang == "fr" || strings.HasPrefix(lang, "fr-")
		isENG := lang == "eng" || lang == "en" || strings.HasPrefix(lang, "en-")
		if !isFR && !isENG {
			continue
		}
		codecID := strings.ToUpper(t.Properties.CodecID)
		isText := strings.Contains(codecID, "TEXT") || strings.Contains(codecID, "UTF") || strings.Contains(codecID, "ASS") || strings.Contains(codecID, "SSA")
		if !isText {
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("ℹ sub #%d ignoré (%s, format image non extractible)", t.ID, t.Properties.CodecID))
			continue
		}
		srtPath, eerr := mkvtool.ExtractTrackToTemp(a.ctx, binary, supplyMkvPath, t.ID, "srt")
		if eerr != nil {
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ extract sub #%d : %s", t.ID, eerr.Error()))
			continue
		}
		outputPath := strings.TrimSuffix(srtPath, filepath.Ext(srtPath)) + ".synced.srt"
		variant := "Full"
		if t.Properties.ForcedTrack {
			variant = "Forced"
		}
		var label, langCode string
		if isFR {
			label = "FR " + variant + " : SRT"
			langCode = "fre"
		} else {
			label = "ENG " + variant + " : SRT"
			langCode = "eng"
		}
		sr := SyncedSubResult{Label: label, Language: langCode, Forced: t.Properties.ForcedTrack}
		if isFR && variant == "Forced" {
			sr.Default = true
		}
		// alass MULTI-SPLIT sub→audio_PSA. VAD audio = vérité terrain.
		// noSplit=false → gère les décalages variables (intro coupée, etc.).
		// disableFPSGuessing=false → laisse alass deviner si drift FPS.
		wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔎 alass multi-split sub #%d (%s) vs audio PSA…", t.ID, lang))
		res, syncErr := alass.Sync(a.ctx, alassPath, psaRefForAlass, srtPath, outputPath, false, false, binDir)
		if syncErr == nil && res != nil {
			sr.Path = res.OutputPath
			sr.Synced = true
			sr.OffsetMs = res.OffsetMs
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ sub #%d : alass multi-split OK, shift max %d ms (%s)", t.ID, res.OffsetMs, label))
			results = append(results, sr)
			continue
		}
		// 2. alass a échoué → fallback tempoShift si on a offset/tempo audio.
		alassErr := "résultat vide"
		if syncErr != nil {
			alassErr = syncErr.Error()
		}
		if hasAudioTransform {
			if terr := tempoShiftSRTFile(srtPath, outputPath, tempoRatio, offsetMs); terr == nil {
				sr.Path = outputPath
				sr.Synced = true
				sr.OffsetMs = offsetMs
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("↻ sub #%d : alass KO (%s) → fallback offset %d ms + tempo %.6f appliqués", t.ID, alassErr, offsetMs, tempoRatio))
				results = append(results, sr)
				continue
			}
		}
		// 3. Sub brut.
		sr.Path = srtPath
		sr.Synced = false
		wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ sub #%d : alass KO (%s) — sub brut utilisé", t.ID, alassErr))
		results = append(results, sr)
	}
	return results, nil
}

// ExtractFirstAudioFromMkv extrait la 1ère piste audio FR (ou ENG en fallback)
// d'un MKV vers un fichier temporaire .ac3/.eac3. Utilisé pour le mode PSA
// quand le user fournit un MKV "synchro" comme source audio.
type ExtractedAudio struct {
	Path     string `json:"path"`
	Codec    string `json:"codec"` // ex: "AC3", "EAC3", "AAC"
	Label    string `json:"label"` // ex: "FR VFi : EAC3 5.1"
	Channels int    `json:"channels"`
	Error    string `json:"error"`
}

func (a *App) ExtractFirstAudioFromMkv(mkvPath string) ExtractedAudio {
	res := ExtractedAudio{}
	if mkvPath == "" {
		res.Error = "chemin vide"
		return res
	}
	binary := a.LocateMkvmerge()
	if binary == "" {
		res.Error = "mkvmerge introuvable"
		return res
	}
	info, err := mkvtool.Identify(a.ctx, binary, mkvPath)
	if err != nil {
		res.Error = "analyse mkv : " + err.Error()
		return res
	}
	// 1ère FR, sinon 1ère audio quelconque.
	trackID := pickFirstAudioIDByLang(info, "fr")
	if trackID < 0 {
		res.Error = "aucune piste audio trouvée"
		return res
	}
	// Trouve les détails de la piste choisie.
	var selectedTrack mkvtool.Track
	for _, t := range info.Tracks {
		if t.ID == trackID {
			selectedTrack = t
			break
		}
	}
	codec := codecIDToLabel(selectedTrack.Properties.CodecID)
	ext := strings.ToLower(codec)
	if ext != "ac3" && ext != "eac3" {
		ext = "ac3" // fallback (mkvextract préfère .ac3)
	}
	tmpPath, eerr := mkvtool.ExtractTrackToTemp(a.ctx, binary, mkvPath, trackID, ext)
	if eerr != nil {
		res.Error = "extract : " + eerr.Error()
		return res
	}
	res.Path = tmpPath
	res.Codec = codec
	res.Channels = selectedTrack.Properties.AudioChannels
	// Label par défaut FR VFi ou ENG VO selon langue + ATMOS si JOC.
	lang := strings.ToLower(selectedTrack.Properties.Language)
	prefix := "ENG VO"
	if lang == "fre" || lang == "fra" || lang == "fr" || strings.HasPrefix(lang, "fr-") {
		prefix = "FR VFi"
	}
	chStr := "5.1"
	switch selectedTrack.Properties.AudioChannels {
	case 1:
		chStr = "1.0"
	case 2:
		chStr = "2.0"
	case 6:
		chStr = "5.1"
	case 8:
		chStr = "7.1"
	}
	// ATMOS : pour EAC3 5.1, on consulte mediainfo pour détecter JOC sur la
	// piste sélectionnée. JOC = Joint Object Coding = signature EAC3 ATMOS.
	atmosSuffix := ""
	if codec == "EAC3" && chStr == "5.1" {
		if mibin, mErr := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); mErr == nil {
			if mi, mErr2 := mediainfo.Identify(a.ctx, mibin, mkvPath); mErr2 == nil {
				audIdx := 0
				audTracks := []mkvtool.Track{}
				for _, t := range info.Tracks {
					if t.Type == "audio" {
						audTracks = append(audTracks, t)
					}
				}
				for _, mt := range mi.Media.Track {
					if mt.Type != "Audio" {
						continue
					}
					if audIdx >= len(audTracks) {
						break
					}
					if audTracks[audIdx].ID == trackID {
						// JOC = signature ATMOS par-piste fiable (Format_AdditionalFeatures).
						// FormatCommercial peut propager "Atmos" sur toutes les pistes du
						// fichier dès qu'une seule l'est → on l'évite.
						feat := strings.ToUpper(mt.FormatAdditionalFeatures)
						title := strings.ToUpper(mt.Title)
						if strings.Contains(feat, "JOC") || strings.Contains(title, "ATMOS") {
							atmosSuffix = " ATMOS"
						}
						break
					}
					audIdx++
				}
			}
		}
	}
	res.Label = fmt.Sprintf("%s : %s %s%s", prefix, codec, chStr, atmosSuffix)
	return res
}

// PSASyncResult décrit le résultat de la vérif sync entre PSA et SUPPLY.
type PSASyncResult struct {
	OffsetMs           int     `json:"offset_ms"`
	Confidence         float64 `json:"confidence"`
	Method             string  `json:"method"` // "chromaprint" ou "no-data"
	FpsRefMkv          float64 `json:"fps_ref_mkv"`
	FpsCandMkv         float64 `json:"fps_cand_mkv"`
	TempoRatio         float64 `json:"tempo_ratio"`          // duration_PSA / duration_SUPPLY
	ResampledAudioPath string  `json:"resampled_audio_path"` // SUPPLY atempo'd (vide si pas de drift)
	ResampledChannels  int     `json:"resampled_channels"`
	ResampledTrackID   int     `json:"resampled_track_id"` // ID de la piste SUPPLY originale (à drop)
	ResampledLanguage  string  `json:"resampled_language"` // code ISO de la piste resamplée (fre/eng/...)
	ResampledCodec     string  `json:"resampled_codec"`    // codec de sortie ("AC3" ou "EAC3")
	ResampledIsAtmos   bool    `json:"resampled_is_atmos"` // true si JOC détecté sur la piste source
	Error              string  `json:"error"`
}

// CheckPSASync compare l'audio de la PSA (refMkvPath) et du SUPPLY (candMkvPath)
// via Chromaprint pour détecter un offset constant. Retourne un PSASyncResult
// avec offset+confiance, à appliquer côté frontend (DelayMs sur secondarySelected).
func (a *App) CheckPSASync(refMkvPath, candMkvPath, lang string) PSASyncResult {
	res := PSASyncResult{}
	if refMkvPath == "" || candMkvPath == "" {
		res.Error = "chemins manquants"
		return res
	}
	binary := a.LocateMkvmerge()
	if binary == "" {
		res.Error = "mkvmerge introuvable"
		return res
	}
	binDir, _ := config.BinDir()
	fpcalcPath, fpErr := chromaprint.Locate("", binDir)
	if fpErr != nil {
		res.Error = "chromaprint indisponible : " + fpErr.Error()
		return res
	}
	// FPS + durée info via mediainfo (best-effort).
	res.TempoRatio = 1.0
	if mibin, _ := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); mibin != "" {
		res.FpsRefMkv = getMediaFPS(a.ctx, mibin, refMkvPath)
		res.FpsCandMkv = getMediaFPS(a.ctx, mibin, candMkvPath)
		// Calcul du tempo ratio : si SUPPLY est plus longue que PSA, faut la
		// raccourcir via mkvmerge --sync ratio. Tolérance 100ms (sinon ratio=1).
		durRef := getMediaDuration(a.ctx, mibin, refMkvPath)
		durCand := getMediaDuration(a.ctx, mibin, candMkvPath)
		if durRef > 0 && durCand > 0 && math.Abs(durRef-durCand) > 0.1 {
			res.TempoRatio = durRef / durCand
		}
	}
	// Identifie la 1ère piste audio FR (ou ENG) de chaque MKV.
	if lang == "" {
		lang = "fr"
	}
	refInfo, err := mkvtool.Identify(a.ctx, binary, refMkvPath)
	if err != nil {
		res.Error = "analyse PSA : " + err.Error()
		return res
	}
	candInfo, err := mkvtool.Identify(a.ctx, binary, candMkvPath)
	if err != nil {
		res.Error = "analyse SUPPLY : " + err.Error()
		return res
	}
	refTrackID := pickFirstAudioIDByLang(refInfo, lang)
	candTrackID := pickFirstAudioIDByLang(candInfo, lang)
	if refTrackID < 0 || candTrackID < 0 {
		res.Error = "pas de piste audio trouvée dans une des sources"
		return res
	}
	// Extract audio temp.
	refAudio, eerr := mkvtool.ExtractTrackToTemp(a.ctx, binary, refMkvPath, refTrackID, "ac3")
	if eerr != nil {
		res.Error = "extract PSA audio : " + eerr.Error()
		return res
	}
	defer os.Remove(refAudio)
	candAudio, eerr2 := mkvtool.ExtractTrackToTemp(a.ctx, binary, candMkvPath, candTrackID, "ac3")
	if eerr2 != nil {
		res.Error = "extract SUPPLY audio : " + eerr2.Error()
		return res
	}
	defer os.Remove(candAudio)
	// Fingerprints + offset.
	fpA, eA := chromaprint.Fingerprint(a.ctx, fpcalcPath, refAudio, 600)
	if eA != nil {
		res.Error = "fingerprint PSA : " + eA.Error()
		return res
	}
	fpB, eB := chromaprint.Fingerprint(a.ctx, fpcalcPath, candAudio, 600)
	if eB != nil {
		res.Error = "fingerprint SUPPLY : " + eB.Error()
		return res
	}
	if len(fpA) < 30 || len(fpB) < 30 {
		res.Error = "fingerprints trop courts"
		return res
	}
	offsetMs, conf, _ := chromaprint.FindOffset(fpA, fpB, 480) // ±60s
	res.OffsetMs = int(offsetMs)
	res.Confidence = conf
	res.Method = "chromaprint"
	res.ResampledTrackID = candTrackID
	// Renseigne la langue de la piste resamplée (pour le label frontend correct).
	for _, t := range candInfo.Tracks {
		if t.ID == candTrackID {
			res.ResampledLanguage = t.Properties.Language
			break
		}
	}

	// Dual-point chromaprint : mesure l'offset à la fin et compare avec le
	// début pour calculer le tempo réel (compense un éventuel drift). C'est
	// PLUS précis que durRef/durCand (qui se base sur la durée container,
	// faussée par chapitres/scrap). Si pas de drift mesurable → fallback sur
	// le ratio de durée déjà calculé.
	if ffmpegEndPath, _ := audiosync.Locate(binDir); ffmpegEndPath != "" {
		durRef := getMediaDuration(a.ctx, func() string { d, _ := mediainfo.Locate(func() string { dd, _ := config.BinDir(); return dd }()); return d }(), refMkvPath)
		durCand := getMediaDuration(a.ctx, func() string { d, _ := mediainfo.Locate(func() string { dd, _ := config.BinDir(); return dd }()); return d }(), candMkvPath)
		windowSec := 60.0
		if durRef > windowSec*3 && durCand > windowSec*3 {
			tmpRefEnd := refAudio + ".end.wav"
			tmpCandEnd := candAudio + ".end.wav"
			defer os.Remove(tmpRefEnd)
			defer os.Remove(tmpCandEnd)
			extractEnd := func(src, dst string, totalDur float64) error {
				startSec := totalDur - windowSec - 5
				if startSec < 0 {
					startSec = 0
				}
				cmd := exec.CommandContext(a.ctx, ffmpegEndPath,
					"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
					"-ss", fmt.Sprintf("%.3f", startSec),
					"-i", src,
					"-t", fmt.Sprintf("%.3f", windowSec),
					"-map", "0:a:0",
					"-ac", "1", "-ar", "16000",
					"-f", "wav", dst,
				)
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("%v — %s", err, string(out))
				}
				return nil
			}
			if extractEnd(refMkvPath, tmpRefEnd, durRef) == nil && extractEnd(candMkvPath, tmpCandEnd, durCand) == nil {
				fpEndA, eEA := chromaprint.Fingerprint(a.ctx, fpcalcPath, tmpRefEnd, int(windowSec))
				fpEndB, eEB := chromaprint.Fingerprint(a.ctx, fpcalcPath, tmpCandEnd, int(windowSec))
				if eEA == nil && eEB == nil && len(fpEndA) >= 30 && len(fpEndB) >= 30 {
					endOffsetMs, endConf, _ := chromaprint.FindOffset(fpEndA, fpEndB, 480)
					if endConf >= 0.5 {
						// Drift = différence entre offset à la fin et au début.
						// FindOffset(PSA, SUPPLY) renvoie offset positif si SUPPLY est
						// décalée plus tard que PSA. Si drift croît positivement →
						// SUPPLY accumule du retard = SUPPLY plus LENTE → durée
						// SUPPLY > durée PSA. La convention TempoRatio est PSA/SUPPLY,
						// donc < 1 dans ce cas. Approximation :
						//   durSupply ≈ durRef + driftMs/1000
						//   TempoRatio = durRef / durSupply ≈ 1 - (driftMs/1000)/spanSec
						endOffsetInt := int(endOffsetMs)
						driftMs := float64(endOffsetInt - res.OffsetMs)
						midRefEnd := durRef - windowSec/2 - 5
						midRefStart := windowSec / 2
						spanSec := midRefEnd - midRefStart
						if spanSec > 60 {
							realTempo := 1.0 - (driftMs/1000.0)/spanSec
							if realTempo > 0.95 && realTempo < 1.05 {
								wr.EventsEmit(a.ctx, "log", fmt.Sprintf("📐 Dual-point sync : offset début=%dms / fin=%dms / drift=%.1fms sur %.0fs → tempo réel=%.6f (vs durée=%.6f)", res.OffsetMs, endOffsetInt, driftMs, spanSec, realTempo, res.TempoRatio))
								res.TempoRatio = realTempo
							}
						}
					}
				}
			}
		}
	}

	// Si drift FPS détecté (ratio durée != 1) → resample SUPPLY audio via ffmpeg
	// atempo. Le tempo est BAKED dans le fichier audio → mux fiable, pas dépendant
	// de l'interprétation mkvmerge --sync ratio. Préserve le codec source
	// (AC3/EAC3) au lieu de tout convertir en AC3.
	if math.Abs(res.TempoRatio-1.0) > 0.001 && conf >= 0.5 {
		ffmpegPath, _ := audiosync.Locate(binDir)
		if ffmpegPath != "" {
			// Détecte codec/channels/bitrate/JOC de la piste cible via mediainfo.
			// Matching par index audio (audIdx) entre mkv tracks et mediainfo tracks.
			channels := 6
			bitrate := 448
			outCodec := "AC3"
			outExt := "ac3"
			isAtmos := false
			candAudIdx := 0
			for _, t := range candInfo.Tracks {
				if t.Type == "audio" {
					if t.ID == candTrackID {
						break
					}
					candAudIdx++
				}
			}
			if mibin, _ := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); mibin != "" {
				if mi, err := mediainfo.Identify(a.ctx, mibin, candMkvPath); err == nil {
					audIdx := 0
					for _, t := range mi.Media.Track {
						if t.Type != "Audio" {
							continue
						}
						if audIdx == candAudIdx {
							if c, err := strconv.Atoi(t.Channels); err == nil && c > 0 {
								channels = c
							}
							if br, err := strconv.Atoi(t.BitRate); err == nil && br > 0 {
								bitrate = br / 1000
							}
							fmtUp := strings.ToUpper(t.Format)
							if strings.Contains(fmtUp, "E-AC-3") || strings.Contains(fmtUp, "EAC3") {
								outCodec = "EAC3"
								outExt = "eac3"
							}
							feat := strings.ToUpper(t.FormatAdditionalFeatures)
							title := strings.ToUpper(t.Title)
							if strings.Contains(feat, "JOC") || strings.Contains(title, "ATMOS") {
								isAtmos = true
							}
							break
						}
						audIdx++
					}
				}
			}
			res.ResampledCodec = outCodec
			res.ResampledIsAtmos = isAtmos
			tempoForResample := 1.0 / res.TempoRatio
			outputPath := candAudio + ".resampled." + outExt
			rerr := audiosync.ResampleAudioFile(a.ctx, ffmpegPath, candAudio, outputPath, outExt, channels, bitrate, tempoForResample)
			if rerr != nil {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠ Resample SUPPLY audio : %s", rerr.Error()))
			} else {
				res.ResampledAudioPath = outputPath
				res.ResampledChannels = channels
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔄 Audio SUPPLY (%s%s) resamplé via atempo=%.6f → %s", outCodec, map[bool]string{true: " ATMOS"}[isAtmos], tempoForResample, filepath.Base(outputPath)))
			}
		}
	}
	return res
}

// countSRTEntries compte le nombre d'entrées (subs) dans un fichier SRT/ASS.
// Approximation suffisante : compte les lignes "-->", marqueur de timecode.
func countSRTEntries(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "-->")
}

// shiftSRTFile = tempoShiftSRTFile avec tempoFactor=1.0 (offset uniquement).
func shiftSRTFile(srcPath, dstPath string, offsetMs int) error {
	return tempoShiftSRTFile(srcPath, dstPath, 1.0, offsetMs)
}

// tempoShiftSRTFile applique d'abord un facteur tempo (multiplication des
// timecodes — pour corriger un drift FPS), puis un offset constant. Écrit le
// résultat dans dstPath. Timecodes négatifs forcés à 0.
func tempoShiftSRTFile(srcPath, dstPath string, tempoFactor float64, offsetMs int) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2}),(\d{3})`)
	shifted := re.ReplaceAllStringFunc(string(data), func(s string) string {
		m := re.FindStringSubmatch(s)
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		sec, _ := strconv.Atoi(m[3])
		ms, _ := strconv.Atoi(m[4])
		total := int(float64(h*3600000+mi*60000+sec*1000+ms)*tempoFactor) + offsetMs
		if total < 0 {
			total = 0
		}
		return fmt.Sprintf("%02d:%02d:%02d,%03d", total/3600000, (total/60000)%60, (total/1000)%60, total%1000)
	})
	return os.WriteFile(dstPath, []byte(shifted), 0644)
}

// getMediaDuration retourne la durée du conteneur en secondes via mediainfo.
// 0 si introuvable.
func getMediaDuration(ctx context.Context, mibin, path string) float64 {
	if mibin == "" || path == "" {
		return 0
	}
	mi, err := mediainfo.Identify(ctx, mibin, path)
	if err != nil {
		return 0
	}
	for _, t := range mi.Media.Track {
		if t.Type == "General" && t.Duration != "" {
			if d, perr := strconv.ParseFloat(t.Duration, 64); perr == nil {
				return d
			}
		}
	}
	return 0
}

// getMediaFPS retourne le framerate (fps) de la première piste vidéo du fichier
// via mediainfo. 0 si introuvable.
func getMediaFPS(ctx context.Context, mibin, path string) float64 {
	if mibin == "" || path == "" {
		return 0
	}
	mi, err := mediainfo.Identify(ctx, mibin, path)
	if err != nil {
		return 0
	}
	for _, t := range mi.Media.Track {
		if t.Type == "Video" && t.FrameRate != "" {
			if f, err := strconv.ParseFloat(t.FrameRate, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

// OCRPGSTrack convertit une piste sub PGS interne au MKV en SRT texte propre :
//   - mkvextract → .sup
//   - pgsrip (Tesseract) → .srt brut
//   - cleanup regex FR (apostrophes, espaces insécables, guillemets, etc.)
//
// Le SRT final est sauvé dans le même dossier que le .mkv source (ou ailleurs
// si on étend l'API plus tard) et le path retourné. Émet "ocr:progress" pour
// la progression (status ∈ extract / ocr / clean / done, percent ∈ 0..100).
//
// Préreq : tesseract + pgsrip installés sur la machine. L'erreur retournée
// inclut l'instruction d'install si l'un des deux est absent.
func (a *App) OCRPGSTrack(mkvPath string, trackID int, lang string) (string, error) {
	if mkvPath == "" {
		return "", errors.New("chemin .mkv vide")
	}
	if lang == "" {
		lang = "fra"
	}
	wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "init", "percent": 0, "message": ""})

	// 1. Localise les binaires (mkvextract via mkvtool, tesseract+pgsrip via ocrsubs).
	c := config.Load()
	binDir, _ := config.BinDir()
	mkvmergePath, mErr := mkvtool.Locate(c.MkvmergePath, binDir)
	if mErr != nil {
		return "", fmt.Errorf("mkvmerge introuvable : %w", mErr)
	}
	// mkvextract est à côté de mkvmerge — on le déduit.
	mkvextractPath := strings.TrimSuffix(mkvmergePath, filepath.Ext(mkvmergePath))
	if strings.HasSuffix(strings.ToLower(mkvmergePath), ".exe") {
		mkvextractPath = strings.TrimSuffix(mkvmergePath, filepath.Base(mkvmergePath)) + "mkvextract.exe"
	} else {
		mkvextractPath = filepath.Join(filepath.Dir(mkvmergePath), "mkvextract")
	}
	if _, err := os.Stat(mkvextractPath); err != nil {
		// Fallback PATH système.
		if p, lpErr := exec.LookPath("mkvextract"); lpErr == nil {
			mkvextractPath = p
		} else {
			return "", errors.New("mkvextract introuvable (à côté de mkvmerge ni sur PATH)")
		}
	}
	if _, err := ocrsubs.LocateTesseract(); err != nil {
		return "", err // message déjà actionnable (brew install …)
	}
	pgsripPath, pErr := ocrsubs.LocatePgsrip()
	if pErr != nil {
		return "", pErr // message déjà actionnable (pip3 install pgsrip)
	}

	progress := func(status string, percent int, message string) {
		wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{
			"status":  status,
			"percent": percent,
			"message": message,
		})
		if message != "" {
			wr.EventsEmit(a.ctx, "log", "🔠 OCR : "+message)
		}
	}

	finalDir := filepath.Dir(mkvPath)
	ltOpts := ocrsubs.LangToolOpts{
		Enabled: true,
		APIURL:  c.LanguageToolURL,
		APIKey:  c.LanguageToolKey,
		APIUser: c.LanguageToolUser,
	}
	srtPath, stats, ltStats, err := ocrsubs.ConvertPGSTrackToSRT(a.ctx, mkvextractPath, pgsripPath, mkvPath, trackID, lang, finalDir, ltOpts, progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "error", "percent": 0, "message": err.Error()})
		return "", err
	}
	// Limite review_list à 5 pour l'event final (UI showcase). La liste
	// complète reste dans LangToolStats côté Go si besoin futur.
	reviewTop := ltStats.NeedsReviewList
	if len(reviewTop) > 5 {
		reviewTop = reviewTop[:5]
	}
	wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{
		"status":           "done",
		"percent":          100,
		"message":          srtPath,
		"total_lines":      stats.TotalLines,
		"corrected_lines":  stats.CorrectedLines,
		"suspicious_lines": stats.SuspiciousLines,
		"quality_score":    stats.QualityScore,
		"subtitles":        stats.Subtitles,
		"lt_total_issues":  ltStats.TotalIssues,
		"lt_auto_fixed":    ltStats.AutoFixed,
		"lt_needs_review":  ltStats.NeedsReview,
		"lt_review_list":   reviewTop,
	})
	return srtPath, nil
}

// OCRSupFile : pipeline OCR pour un fichier .sup PGS externe (déjà extrait,
// pas besoin de mkvextract). Lance pgsrip + CleanSRT, retourne le path .srt
// final écrit à côté du .sup source.
func (a *App) OCRSupFile(supPath, lang string) (string, error) {
	if supPath == "" {
		return "", errors.New("chemin .sup vide")
	}
	if lang == "" {
		lang = "fra"
	}
	wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "init", "percent": 0, "message": ""})

	// Cache OCR par sha256(.sup) — gain de minutes si déjà OCRisé.
	if cachedPath, ok := ocrsubs.LookupOCRCache(supPath); ok {
		finalDir := filepath.Dir(supPath)
		base := strings.TrimSuffix(filepath.Base(supPath), filepath.Ext(supPath))
		finalSRT := filepath.Join(finalDir, base+".ocr.srt")
		if cerr := func() error {
			in, err := os.Open(cachedPath)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.Create(finalSRT)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, in)
			return err
		}(); cerr == nil {
			subCount := 0
			if data, rerr := os.ReadFile(finalSRT); rerr == nil {
				subCount = strings.Count(string(data), "-->")
			}
			wr.EventsEmit(a.ctx, "log", "🚀 OCR : utilisé cache (skip pgsrip+clean)")
			wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{
				"status":           "done",
				"percent":          100,
				"message":          finalSRT,
				"total_lines":      0,
				"corrected_lines":  0,
				"suspicious_lines": 0,
				"quality_score":    100.0,
				"subtitles":        subCount,
				"lt_total_issues":  0,
				"lt_auto_fixed":    0,
				"lt_needs_review":  0,
				"lt_review_list":   []ocrsubs.ReviewMatch{},
			})
			return finalSRT, nil
		}
	}

	if _, err := ocrsubs.LocateTesseract(); err != nil {
		return "", err
	}
	pgsripPath, pErr := ocrsubs.LocatePgsrip()
	if pErr != nil {
		return "", pErr
	}

	wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "ocr", "percent": 30, "message": "OCR Tesseract en cours…"})
	wr.EventsEmit(a.ctx, "log", "🔠 OCR : pgsrip "+filepath.Base(supPath))
	srtRaw, err := ocrsubs.RunPgsrip(a.ctx, pgsripPath, supPath, lang)
	if err != nil {
		wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "error", "percent": 0, "message": err.Error()})
		return "", err
	}

	wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "clean", "percent": 80, "message": "Nettoyage regex FR…"})
	stats, err := ocrsubs.CleanSRT(srtRaw)
	if err != nil {
		wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "error", "percent": 0, "message": err.Error()})
		return "", err
	}

	// Renomme en .ocr.srt à côté de la source pour cohérence avec OCRPGSTrack.
	finalDir := filepath.Dir(supPath)
	base := strings.TrimSuffix(filepath.Base(supPath), filepath.Ext(supPath))
	finalSRT := filepath.Join(finalDir, base+".ocr.srt")
	if srtRaw != finalSRT {
		_ = os.Rename(srtRaw, finalSRT)
	}

	// LanguageTool best-effort sur le SRT final (à côté de la source).
	c := config.Load()
	progressLT := func(status string, percent int, message string) {
		wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{
			"status":  status,
			"percent": percent,
			"message": message,
		})
		if message != "" {
			wr.EventsEmit(a.ctx, "log", "🔠 OCR : "+message)
		}
	}
	wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{"status": "languagetool", "percent": 96, "message": "Vérification LanguageTool…"})
	ltStats, ltErr := ocrsubs.LanguageToolFix(a.ctx, finalSRT, lang, c.LanguageToolURL, c.LanguageToolKey, c.LanguageToolUser, progressLT)
	if ltErr != nil {
		wr.EventsEmit(a.ctx, "log", "⚠ LanguageTool : "+ltErr.Error())
	}
	// Le quality_score reste basé sur les patterns suspects regex uniquement.
	// LanguageTool fournit beaucoup de faux positifs (style, grammaire idiomatique)
	// qui ne sont pas de vraies erreurs OCR — affichés à part dans l'UI.

	reviewTop := ltStats.NeedsReviewList
	if len(reviewTop) > 5 {
		reviewTop = reviewTop[:5]
	}
	wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{
		"status":           "done",
		"percent":          100,
		"message":          finalSRT,
		"total_lines":      stats.TotalLines,
		"corrected_lines":  stats.CorrectedLines,
		"suspicious_lines": stats.SuspiciousLines,
		"quality_score":    stats.QualityScore,
		"subtitles":        stats.Subtitles,
		"lt_total_issues":  ltStats.TotalIssues,
		"lt_auto_fixed":    ltStats.AutoFixed,
		"lt_needs_review":  ltStats.NeedsReview,
		"lt_review_list":   reviewTop,
	})
	// Best-effort : sauve le SRT final dans le cache (sha256 du .sup).
	_ = ocrsubs.StoreOCRCache(supPath, finalSRT)
	return finalSRT, nil
}

// ApplyOCRFix patch un SRT avec une correction validée par l'utilisateur dans
// le modal "lignes à vérifier".
//
// Flow :
//  1. Lit le SRT
//  2. Cherche `originalSnippet` autour de la ligne `lineNumber` (±2 lignes)
//  3. Remplace par `correction`
//  4. Sauve le SRT
//
// Retourne nil si succès. Erreur si snippet introuvable (le patch est rollback).
// Le snippet peut contenir des "…" en début/fin (ajoutés par snippetAround) — on
// les strippe avant la recherche.
func (a *App) ApplyOCRFix(srtPath string, lineNumber int, originalSnippet, correction string) error {
	if srtPath == "" {
		return errors.New("srtPath vide")
	}
	if originalSnippet == "" {
		return errors.New("snippet original vide")
	}
	// Strippe les "…" éventuels en début/fin (ils viennent de snippetAround).
	cleanSnippet := strings.TrimSpace(originalSnippet)
	cleanSnippet = strings.TrimPrefix(cleanSnippet, "…")
	cleanSnippet = strings.TrimSuffix(cleanSnippet, "…")
	cleanSnippet = strings.TrimSpace(cleanSnippet)
	if cleanSnippet == "" {
		return errors.New("snippet vide après normalisation")
	}

	raw, err := os.ReadFile(srtPath)
	if err != nil {
		return fmt.Errorf("lecture SRT : %w", err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if lineNumber < 1 {
		lineNumber = 1
	}
	target0 := lineNumber - 1 // index 0-based
	if target0 >= len(lines) {
		target0 = len(lines) - 1
	}
	// Cherche d'abord exactement à lineNumber, puis ±1, ±2.
	candidates := []int{target0}
	for delta := 1; delta <= 2; delta++ {
		if target0-delta >= 0 {
			candidates = append(candidates, target0-delta)
		}
		if target0+delta < len(lines) {
			candidates = append(candidates, target0+delta)
		}
	}
	for _, idx := range candidates {
		line := lines[idx]
		if strings.Contains(line, cleanSnippet) {
			lines[idx] = strings.Replace(line, cleanSnippet, correction, 1)
			out := strings.Join(lines, "\n")
			if !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			if werr := os.WriteFile(srtPath, []byte(out), 0644); werr != nil {
				return fmt.Errorf("écriture SRT : %w", werr)
			}
			return nil
		}
	}
	return fmt.Errorf("snippet introuvable autour de la ligne %d (±2)", lineNumber)
}

// SearchOpenSubtitles interroge l'API OpenSubtitles pour trouver un SRT
// existant. Retourne max ~50 résultats. Nécessite OpenSubtitlesAPIKey en config.
func (a *App) SearchOpenSubtitles(query string, year int, lang string) ([]opensubtitles.OSSearchResult, error) {
	c := config.Load()
	if strings.TrimSpace(c.OpenSubtitlesAPIKey) == "" {
		return nil, errors.New("OpenSubtitles : clé API manquante (Settings → OpenSubtitles)")
	}
	results, err := opensubtitles.Search(a.ctx, c.OpenSubtitlesAPIKey, opensubtitles.DefaultUserAgent, query, year, lang)
	if err != nil {
		wr.EventsEmit(a.ctx, "log", "⚠ OpenSubtitles : "+err.Error())
		return nil, err
	}
	wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔍 OpenSubtitles : %d résultats pour « %s » (%d, %s)", len(results), query, year, lang))
	return results, nil
}

// DownloadOpenSubtitle télécharge un SRT depuis OpenSubtitles ET applique
// CleanSRT + LanguageToolFix dessus avant de retourner le path final.
//
// `dstPath` peut être un chemin précis (.srt) OU un dossier — auquel cas le
// nom est généré depuis fileID + .srt.
func (a *App) DownloadOpenSubtitle(fileID, dstPath string) (string, error) {
	c := config.Load()
	if strings.TrimSpace(c.OpenSubtitlesAPIKey) == "" {
		return "", errors.New("OpenSubtitles : clé API manquante (Settings → OpenSubtitles)")
	}
	if fileID == "" {
		return "", errors.New("fileID vide")
	}
	if dstPath == "" {
		return "", errors.New("dstPath vide")
	}
	// Si dstPath est un dossier, génère un nom propre.
	if st, err := os.Stat(dstPath); err == nil && st.IsDir() {
		dstPath = filepath.Join(dstPath, "opensubtitles-"+fileID+".srt")
	} else if !strings.HasSuffix(strings.ToLower(dstPath), ".srt") {
		dstPath += ".srt"
	}
	wr.EventsEmit(a.ctx, "log", "🔍 OpenSubtitles : téléchargement fileID="+fileID)
	if err := opensubtitles.Download(a.ctx, c.OpenSubtitlesAPIKey, opensubtitles.DefaultUserAgent, fileID, dstPath); err != nil {
		wr.EventsEmit(a.ctx, "log", "❌ OpenSubtitles : "+err.Error())
		return "", err
	}
	// Cleanup regex + LanguageTool best-effort (comme pour OCR).
	if _, err := ocrsubs.CleanSRT(dstPath); err != nil {
		wr.EventsEmit(a.ctx, "log", "⚠ Cleanup SRT (OS) : "+err.Error())
	}
	// LanguageTool en best-effort — on log mais on ne plante pas.
	progressLT := func(status string, percent int, message string) {
		wr.EventsEmit(a.ctx, "ocr:progress", map[string]any{
			"status": status, "percent": percent, "message": message,
		})
	}
	if _, err := ocrsubs.LanguageToolFix(a.ctx, dstPath, "fra", c.LanguageToolURL, c.LanguageToolKey, c.LanguageToolUser, progressLT); err != nil {
		wr.EventsEmit(a.ctx, "log", "⚠ LanguageTool (OS) : "+err.Error())
	}
	wr.EventsEmit(a.ctx, "log", "✓ OpenSubtitles : SRT téléchargé + nettoyé : "+dstPath)
	return dstPath, nil
}

// OCRCustomDictList retourne toutes les entrées du dictionnaire custom.
func (a *App) OCRCustomDictList() ([]ocrsubs.CustomDictEntry, error) {
	return ocrsubs.ListCustomDictEntries()
}

// OCRCustomDictAdd ajoute (ou met à jour) une entrée du dictionnaire custom.
// `auto` = true si l'entrée vient d'une validation utilisateur (modal review).
func (a *App) OCRCustomDictAdd(wrong, right string, auto bool) error {
	return ocrsubs.AddCustomDictEntry(wrong, right, auto)
}

// OCRCustomDictRemove retire une entrée par sa clé `wrong`.
func (a *App) OCRCustomDictRemove(wrong string) error {
	return ocrsubs.RemoveCustomDictEntry(wrong)
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

// SyncAudioTrack résume une piste audio pour l'onglet Synchro.
type SyncAudioTrack struct {
	ID       int    `json:"id"`
	Codec    string `json:"codec"`
	Language string `json:"language"`
	Name     string `json:"name"`
	Channels int    `json:"channels"`
}

// ListAudioTracksForSync renvoie les pistes audio d'un .mkv pour l'onglet Synchro.
func (a *App) ListAudioTracksForSync(path string) ([]SyncAudioTrack, error) {
	if path == "" {
		return nil, errors.New("chemin vide")
	}
	binary := a.LocateMkvmerge()
	if binary == "" {
		return nil, errors.New("mkvmerge introuvable")
	}
	info, err := mkvtool.Identify(a.ctx, binary, path)
	if err != nil {
		return nil, err
	}
	out := []SyncAudioTrack{}
	for _, t := range info.Tracks {
		if t.Type != "audio" {
			continue
		}
		out = append(out, SyncAudioTrack{
			ID:       t.ID,
			Codec:    t.Codec,
			Language: t.Properties.Language,
			Name:     t.Properties.TrackName,
			Channels: t.Properties.AudioChannels,
		})
	}
	return out, nil
}

// DetectAudioOffset mesure auto le décalage de la piste `otherID` par rapport
// à `refID` dans `path` (cross-correlation des enveloppes via ffmpeg). Vérifie
// aussi le drift sur les films >20 min en mesurant à 85% du film.
func (a *App) DetectAudioOffset(path string, refID, otherID int) (*audiosync.DetectionResult, error) {
	if path == "" {
		return nil, errors.New("chemin vide")
	}
	binDir, _ := config.BinDir()
	ffmpeg, err := audiosync.Locate(binDir)
	if err != nil {
		return nil, err
	}
	// Durée totale du film via mediainfo (pour activer la double-mesure).
	var durationSec float64
	if mibin, err := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); err == nil {
		if mi, err := mediainfo.Identify(a.ctx, mibin, path); err == nil {
			for _, t := range mi.Media.Track {
				if t.Type == "General" && t.Duration != "" {
					if d, err := strconv.ParseFloat(t.Duration, 64); err == nil {
						durationSec = d
					}
					break
				}
			}
		}
	}
	wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🔎 Détection offset piste %d vs réf %d…", otherID, refID))
	res, err := audiosync.DetectOffset(a.ctx, ffmpeg, path, refID, otherID, durationSec)
	if err != nil {
		wr.EventsEmit(a.ctx, "log", "❌ Détection : "+err.Error())
		return nil, err
	}
	msg := fmt.Sprintf("✓ Piste %d : offset = %d ms (confiance %.2f)", otherID, res.OffsetMs, res.Confidence)
	if res.DriftMs > 0 {
		msg += fmt.Sprintf(", drift %d ms", res.DriftMs)
	}
	if res.Notes != "" {
		msg += " — " + res.Notes
	}
	wr.EventsEmit(a.ctx, "log", msg)
	return res, nil
}

// AudioSyncOffset associe un track ID à un décalage en ms et un éventuel
// ratio atempo (resample requis si FPS différent → drift linéaire).
type AudioSyncOffset struct {
	TrackID     int     `json:"track_id"`
	DelayMs     int     `json:"delay_ms"`
	TempoFactor float64 `json:"tempo_factor"` // 1.0 = pas de resample ; ≠1.0 = ffmpeg atempo + réencode
}

// AudioSyncRequest : remux un .mkv en appliquant les décalages sur certaines pistes audio.
type AudioSyncRequest struct {
	InputPath  string            `json:"input_path"`
	OutputPath string            `json:"output_path"`
	Offsets    []AudioSyncOffset `json:"offsets"`
}

// MuxAudioSync remux le .mkv en appliquant les corrections sur certaines pistes audio :
//   - Offset constant uniquement (TempoFactor = 0 ou 1.0) → mkvmerge --sync TID:offset
//     (audio bit-à-bit, AC3/EAC3 préservés).
//   - Drift linéaire (TempoFactor ≠ 1.0, typiquement FPS différent) → la piste est
//     d'abord ré-encodée avec ffmpeg atempo (1 génération AC3/EAC3 perdue), puis
//     muxée comme audio externe avec son nouvel offset.
//
// Émet "audiosync:progress" / "audiosync:done".
func (a *App) MuxAudioSync(req AudioSyncRequest) error {
	binary := a.LocateMkvmerge()
	if binary == "" {
		return errMkvNotFound
	}
	info, err := mkvtool.Identify(a.ctx, binary, req.InputPath)
	if err != nil {
		return err
	}
	offsetByID := map[int]AudioSyncOffset{}
	for _, o := range req.Offsets {
		offsetByID[o.TrackID] = o
	}

	// Sépare les pistes nécessitant un resample (TempoFactor ≠ 1.0) des autres.
	resampleTIDs := map[int]bool{}
	for _, o := range req.Offsets {
		if o.TempoFactor != 0 && o.TempoFactor != 1.0 {
			resampleTIDs[o.TrackID] = true
		}
	}

	// Récupère bitrate/codec/channels via mediainfo pour les pistes à resample.
	type trackMeta struct {
		Codec       string
		Channels    int
		BitrateKbps int
		Name        string
		Language    string
	}
	metaByID := map[int]trackMeta{}
	if len(resampleTIDs) > 0 {
		if mibin, err := mediainfo.Locate(func() string { d, _ := config.BinDir(); return d }()); err == nil {
			if mi, err := mediainfo.Identify(a.ctx, mibin, req.InputPath); err == nil {
				audIdx := 0
				for _, t := range info.Tracks {
					if t.Type != "audio" {
						continue
					}
					if resampleTIDs[t.ID] {
						// L'audio mediainfo dans le même ordre que mkvmerge audio.
						var miAudio mediainfo.Track
						count := 0
						for _, m := range mi.Media.Track {
							if m.Type == "Audio" {
								if count == audIdx {
									miAudio = m
									break
								}
								count++
							}
						}
						chans := t.Properties.AudioChannels
						if chans == 0 {
							chans, _ = strconv.Atoi(miAudio.Channels)
						}
						bitrateKbps := 0
						if miAudio.BitRate != "" {
							if br, err := strconv.Atoi(miAudio.BitRate); err == nil {
								bitrateKbps = br / 1000
							}
						}
						if bitrateKbps == 0 {
							// Défauts raisonnables si non détecté.
							switch chans {
							case 1:
								bitrateKbps = 192
							case 2:
								bitrateKbps = 256
							case 6, 8:
								bitrateKbps = 640
							default:
								bitrateKbps = 384
							}
						}
						metaByID[t.ID] = trackMeta{
							Codec:       t.Codec,
							Channels:    chans,
							BitrateKbps: bitrateKbps,
							Name:        t.Properties.TrackName,
							Language:    t.Properties.Language,
						}
					}
					audIdx++
				}
			}
		}
	}

	// Resample chaque piste concernée vers /tmp/<basename>.<tid>.<ext>.
	tmpDir, err := os.MkdirTemp("", "audiosync-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	resampledFiles := map[int]string{} // tid → chemin du fichier resamplé
	if len(resampleTIDs) > 0 {
		ffmpeg, err := audiosync.Locate("")
		if err != nil {
			return fmt.Errorf("resample : %w", err)
		}
		for tid := range resampleTIDs {
			meta := metaByID[tid]
			if meta.Codec == "" {
				wr.EventsEmit(a.ctx, "log", fmt.Sprintf("⚠️ Piste %d : codec inconnu, resample skip", tid))
				continue
			}
			ext := "ac3"
			codecLower := strings.ToLower(meta.Codec)
			if strings.Contains(codecLower, "e-ac") || strings.Contains(codecLower, "eac") {
				ext = "eac3"
			}
			outPath := filepath.Join(tmpDir, fmt.Sprintf("track_%d.%s", tid, ext))
			tempo := offsetByID[tid].TempoFactor
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("🎚 Resample piste %d (%s, %d ch, %d kbps, atempo=%.6f)…", tid, meta.Codec, meta.Channels, meta.BitrateKbps, tempo))
			if err := audiosync.Resample(a.ctx, ffmpeg, audiosync.ResampleParams{
				InputPath:   req.InputPath,
				TrackID:     tid,
				Codec:       meta.Codec,
				Channels:    meta.Channels,
				BitrateKbps: meta.BitrateKbps,
				Tempo:       tempo,
				OutputPath:  outPath,
			}); err != nil {
				wr.EventsEmit(a.ctx, "log", "❌ Resample piste "+strconv.Itoa(tid)+" : "+err.Error())
				return err
			}
			resampledFiles[tid] = outPath
			wr.EventsEmit(a.ctx, "log", fmt.Sprintf("✓ Piste %d resamplée → %s", tid, filepath.Base(outPath)))
		}
	}

	// Construit les TrackSpec : pistes resamplées exclues, autres gardées avec --sync.
	var tracks []mkvtool.TrackSpec
	for i, t := range info.Tracks {
		spec := mkvtool.TrackSpec{
			ID:      t.ID,
			Type:    t.Type,
			Default: t.Properties.DefaultTrack,
			Forced:  t.Properties.ForcedTrack,
			Order:   i,
		}
		if _, isResampled := resampledFiles[t.ID]; isResampled {
			spec.Keep = false
		} else {
			spec.Keep = true
			if o, ok := offsetByID[t.ID]; ok {
				spec.DelayMs = o.DelayMs
			}
		}
		tracks = append(tracks, spec)
	}

	// Ajoute les pistes resamplées comme audios externes, en préservant leur place dans l'ordre.
	var externalAudios []mkvtool.ExternalAudio
	for tid, path := range resampledFiles {
		meta := metaByID[tid]
		// Ordre : on insère à la position originelle du tid (pour garder l'ordre des pistes).
		order := 0
		for i, t := range info.Tracks {
			if t.ID == tid {
				order = i
				break
			}
		}
		externalAudios = append(externalAudios, mkvtool.ExternalAudio{
			Path:     path,
			Name:     meta.Name,
			Language: meta.Language,
			Default:  false, // par défaut on ne ré-élève pas ; on respecte la default originelle si elle est ailleurs
			Forced:   false,
			DelayMs:  offsetByID[tid].DelayMs,
			Order:    order,
		})
	}

	a.mu.Lock()
	if a.opCancel != nil {
		a.opCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.opCtx, a.opCancel = ctx, cancel
	a.mu.Unlock()

	wr.EventsEmit(a.ctx, "log", "🔧 Recalage audio → "+filepath.Base(req.OutputPath))
	err = mkvtool.Mux(ctx, binary, mkvtool.MuxParams{
		InputPath:      req.InputPath,
		OutputPath:     req.OutputPath,
		Tracks:         tracks,
		ExternalAudios: externalAudios,
	},
		func(p mkvtool.MuxProgress) { wr.EventsEmit(a.ctx, "audiosync:progress", p) },
		func(line string) { wr.EventsEmit(a.ctx, "log", line) },
	)
	a.mu.Lock()
	a.opCtx, a.opCancel = nil, nil
	a.mu.Unlock()
	if err != nil {
		wr.EventsEmit(a.ctx, "log", "❌ "+err.Error())
		wr.EventsEmit(a.ctx, "audiosync:done", map[string]any{"ok": false})
		return err
	}
	wr.EventsEmit(a.ctx, "log", "✅ Recalage terminé : "+req.OutputPath)
	wr.EventsEmit(a.ctx, "audiosync:done", map[string]any{"ok": true, "path": req.OutputPath})
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

// --- Index Discord ---
//
// L'index Discord permet à tous les users de l'app de cliquer sur "↗ Discord"
// dans le header film-bar pour ouvrir le post Discord d'un film de la Team
// (lookup par TMDB ID). Le bot Discord n'est utilisé QUE par l'admin via
// DiscordIndexScan ; tous les autres appels sont des lookups locaux ou un
// fetch HTTP du JSON public.

// DiscordIndexScan (admin) lance un scan complet du forum Discord configuré
// et écrit le JSON localement. Émet "discordindex:progress" pendant le scan.
// Retourne le chemin du fichier JSON local généré.
func (a *App) DiscordIndexScan() (string, error) {
	cfg := config.Load()
	if strings.TrimSpace(cfg.DiscordBotToken) == "" {
		return "", errors.New("Token bot Discord non configuré (Réglages → Index Discord)")
	}
	rawIDs := strings.TrimSpace(cfg.DiscordForumID)
	if rawIDs == "" {
		return "", errors.New("ID forum channel Discord non configuré (Réglages → Index Discord)")
	}
	// Parse une ou plusieurs IDs : séparées par virgule, espace ou newline.
	splitter := func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ';' }
	rawTokens := strings.FieldsFunc(rawIDs, splitter)
	var forumIDs []string
	for _, t := range rawTokens {
		t = strings.TrimSpace(t)
		if t != "" {
			forumIDs = append(forumIDs, t)
		}
	}
	if len(forumIDs) == 0 {
		return "", errors.New("aucun ID forum channel Discord valide")
	}
	path, err := config.DiscordIndexPath()
	if err != nil {
		return "", err
	}
	// Charge l'index existant pour le scan incrémental : les threads dont le
	// last_message_id n'a pas bougé depuis le scan précédent seront skippés
	// (réutilisation directe de l'entry, pas de fetch HTTP). Best-effort : si
	// l'index local n'est pas lisible / n'existe pas → scan complet.
	existing, _ := discordindex.LoadIndex(path)
	// Merge des entries de tous les forums.
	merged := &discordindex.Index{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:     map[string]discordindex.Entry{},
	}
	for fi, fid := range forumIDs {
		forumNum := fi + 1
		totalForums := len(forumIDs)
		progress := func(scanned, total int, message string) {
			// IMPORTANT : ne JAMAIS inclure le token dans les logs / events.
			wr.EventsEmit(a.ctx, "discordindex:progress", map[string]interface{}{
				"scanned": scanned,
				"total":   total,
				"message": fmt.Sprintf("[forum %d/%d] %s", forumNum, totalForums, message),
			})
		}
		idx, err := discordindex.ScanForumIncremental(a.ctx, cfg.DiscordBotToken, fid, existing, progress)
		if err != nil {
			msg := err.Error()
			if cfg.DiscordBotToken != "" && strings.Contains(msg, cfg.DiscordBotToken) {
				msg = strings.ReplaceAll(msg, cfg.DiscordBotToken, "[redacted]")
			}
			return "", fmt.Errorf("forum %s : %s", fid, msg)
		}
		// Merge : les entries plus récentes (UpdatedAt) écrasent les plus anciennes.
		for k, v := range idx.Entries {
			if existing, ok := merged.Entries[k]; ok {
				if v.UpdatedAt > existing.UpdatedAt {
					merged.Entries[k] = v
				}
			} else {
				merged.Entries[k] = v
			}
		}
	}
	if err := discordindex.SaveIndex(merged, path); err != nil {
		return "", err
	}
	wr.EventsEmit(a.ctx, "discordindex:progress", map[string]interface{}{
		"scanned": len(merged.Entries),
		"total":   len(merged.Entries),
		"message": fmt.Sprintf("✓ Index sauvegardé : %d entrées (%d forum%s) → %s", len(merged.Entries), len(forumIDs), map[bool]string{true: "s", false: ""}[len(forumIDs) > 1], path),
		"done":    true,
	})
	return path, nil
}

// DiscordIndexRead (admin) lit le JSON local et retourne son contenu (string)
// pour copier-coller / vérification.
func (a *App) DiscordIndexRead() (string, error) {
	path, err := config.DiscordIndexPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DiscordIndexPushGitHub (admin) push le JSON local sur GitHub directement via
// l'API Contents. Met à jour le fichier s'il existe, sinon le crée.
// Retourne le SHA du nouveau commit ou une erreur sanitizée (token jamais loggé).
func (a *App) DiscordIndexPushGitHub() (string, error) {
	cfg := config.Load()
	if strings.TrimSpace(cfg.GitHubToken) == "" {
		return "", errors.New("Token GitHub non configuré (Réglages → Index Discord → Push GitHub)")
	}
	if strings.TrimSpace(cfg.GitHubRepo) == "" {
		return "", errors.New("Repo GitHub non configuré (format : owner/name)")
	}
	filePath := strings.TrimSpace(cfg.GitHubIndexFilePath)
	if filePath == "" {
		filePath = "discord_index.json"
	}
	branch := strings.TrimSpace(cfg.GitHubBranch)
	if branch == "" {
		branch = "main"
	}
	jsonPath, err := config.DiscordIndexPath()
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(jsonPath)
	if err != nil {
		return "", fmt.Errorf("lecture index local : %w (lance d'abord 'Mettre à jour l'index')", err)
	}
	msg := fmt.Sprintf("chore(discord-index): update %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	sha, err := discordindex.PushToGitHub(a.ctx, cfg.GitHubToken, cfg.GitHubRepo, branch, filePath, content, msg)
	if err != nil {
		return "", err
	}
	wr.EventsEmit(a.ctx, "log", fmt.Sprintf("📤 GitHub : index pushé sur %s@%s/%s (SHA %s)", cfg.GitHubRepo, branch, filePath, sha[:min(len(sha), 8)]))
	return sha, nil
}

// DiscordIndexLookup (user) cherche un TMDB ID dans l'index local d'abord,
// puis dans le remote si pas trouvé. Retourne l'URL Discord ou "".
// N'utilise JAMAIS le token Discord.
func (a *App) DiscordIndexLookup(tmdbID string) (string, error) {
	tmdbID = strings.TrimSpace(tmdbID)
	if tmdbID == "" {
		return "", nil
	}
	path, err := config.DiscordIndexPath()
	if err != nil {
		return "", err
	}
	// 1) cache local (qui peut être l'index admin OU le remote téléchargé).
	if idx, _ := discordindex.LoadIndex(path); idx != nil {
		if u := discordindex.LookupTmdb(idx, tmdbID); u != "" {
			return u, nil
		}
	}
	// 2) tenter un fetch remote (best-effort, silencieux si pas configuré).
	cfg := config.Load()
	if strings.TrimSpace(cfg.DiscordIndexURL) != "" {
		if idx, err := discordindex.FetchRemoteIndex(a.ctx, cfg.DiscordIndexURL, path); err == nil && idx != nil {
			if u := discordindex.LookupTmdb(idx, tmdbID); u != "" {
				return u, nil
			}
		}
	}
	return "", nil
}

// DiscordIndexRefreshRemote (user) force un téléchargement du JSON remote
// (si configuré). Best-effort : pas d'erreur si l'URL n'est pas configurée.
func (a *App) DiscordIndexRefreshRemote() error {
	cfg := config.Load()
	url := strings.TrimSpace(cfg.DiscordIndexURL)
	if url == "" {
		return nil // silencieux : pas configuré
	}
	path, err := config.DiscordIndexPath()
	if err != nil {
		return err
	}
	// Force refresh : on supprime le cache pour bypasser le TTL 24h.
	_ = os.Remove(path)
	_, err = discordindex.FetchRemoteIndex(a.ctx, url, path)
	return err
}
