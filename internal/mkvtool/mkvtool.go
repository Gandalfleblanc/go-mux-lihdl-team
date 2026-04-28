// Package mkvtool fournit un wrapper autour de mkvmerge :
//   - localisation du binaire (override config → app data dir → PATH)
//   - Identify : parse la sortie "mkvmerge -J" (JSON) en struct exploitable
//   - Mux : construit la commande mkvmerge avec les flags de renommage et
//     émet des events de progression au fur et à mesure.
package mkvtool

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Locate trouve le chemin du binaire mkvmerge selon la priorité suivante :
//  1. override explicite (depuis la config)
//  2. binaire embarqué dans l'app (extrait à appBinDir au 1er run)
//  3. binaire téléchargé dans le dossier de l'app
//  4. binaire système sur PATH
func Locate(configOverride, appBinDir string) (string, error) {
	if configOverride != "" {
		if _, err := exec.LookPath(configOverride); err == nil {
			return configOverride, nil
		}
	}
	// Embedded binaries : extrait mkvmerge ET mkvextract à appBinDir si non
	// présents et embeds non vides. mkvextract est extrait au même endroit pour
	// que findMkvextract le trouve à côté de mkvmerge.
	if appBinDir != "" && len(embeddedExtract) > 0 {
		extractPath := filepath.Join(appBinDir, embeddedExtractName)
		if _, err := os.Stat(extractPath); err != nil {
			_ = os.WriteFile(extractPath, embeddedExtract, 0755)
		}
	}
	if appBinDir != "" && len(embeddedBinary) > 0 {
		candidate := filepath.Join(appBinDir, embeddedName)
		if _, err := os.Stat(candidate); err != nil {
			// Pas encore extrait → on l'écrit.
			if werr := os.WriteFile(candidate, embeddedBinary, 0755); werr == nil {
				return candidate, nil
			}
		} else {
			return candidate, nil
		}
	}
	if appBinDir != "" {
		candidate := filepath.Join(appBinDir, embeddedName)
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("mkvmerge"); err == nil {
		return p, nil
	}
	return "", errors.New("mkvmerge introuvable (ni override, ni embarqué, ni sur PATH)")
}

// Info est la vue simplifiée du résultat de "mkvmerge -J file.mkv".
type Info struct {
	Container ContainerInfo `json:"container"`
	Tracks    []Track       `json:"tracks"`
}

type ContainerInfo struct {
	Properties ContainerProperties `json:"properties"`
}

type ContainerProperties struct {
	Title string `json:"title"`
}

type Track struct {
	ID         int             `json:"id"`
	Type       string          `json:"type"` // "video" | "audio" | "subtitles"
	Codec      string          `json:"codec"`
	Properties TrackProperties `json:"properties"`
}

type TrackProperties struct {
	Language        string `json:"language"`
	TrackName       string `json:"track_name"`
	AudioChannels   int    `json:"audio_channels"`
	CodecID         string `json:"codec_id"`
	DefaultTrack    bool   `json:"default_track"`
	ForcedTrack     bool   `json:"forced_track"`
	PixelDimensions string `json:"pixel_dimensions"` // ex "1920x1080"
}

// Identify exécute "mkvmerge -J <file>" et décode le JSON.
func Identify(ctx context.Context, binary, mkvPath string) (*Info, error) {
	raw, err := IdentifyRaw(ctx, binary, mkvPath)
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, fmt.Errorf("parse JSON mkvmerge : %w", err)
	}
	return &info, nil
}

// IdentifyRaw exécute "mkvmerge -J <file>" et retourne le JSON brut.
// Utilisé par le frontend qui parse lui-même (contourne Wails).
func IdentifyRaw(ctx context.Context, binary, mkvPath string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, "-J", mkvPath)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkvmerge -J : %w", err)
	}
	return string(out), nil
}

// ExtractTrackToTemp extrait une piste via mkvextract dans un fichier
// temporaire. Le binaire mkvextract est cherché à côté de mkvmerge ; sinon
// sur le PATH système. Retourne le chemin du fichier extrait — l'appelant
// doit le supprimer (os.Remove) après usage.
func ExtractTrackToTemp(ctx context.Context, mkvmergePath, mkvPath string, trackID int, ext string) (string, error) {
	extractBin := findMkvextract(mkvmergePath)
	if extractBin == "" {
		return "", errors.New("mkvextract introuvable (à côté de mkvmerge ni sur PATH)")
	}
	tmp, err := os.CreateTemp("", "submux-extract-*."+ext)
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	os.Remove(tmpPath) // mkvextract recrée le fichier
	cmd := exec.CommandContext(ctx, extractBin, "tracks", mkvPath, fmt.Sprintf("%d:%s", trackID, tmpPath))
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("mkvextract : %w (%s)", err, string(out))
	}
	return tmpPath, nil
}

// findMkvextract cherche le binaire mkvextract à côté de mkvmerge.
func findMkvextract(mkvmergePath string) string {
	if mkvmergePath == "" {
		if p, err := exec.LookPath("mkvextract"); err == nil {
			return p
		}
		return ""
	}
	dir := filepath.Dir(mkvmergePath)
	name := "mkvextract"
	// Sur Windows, le nom du binaire est .exe (similaire à mkvmerge.exe)
	if strings.HasSuffix(strings.ToLower(mkvmergePath), ".exe") {
		name = "mkvextract.exe"
	}
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if p, err := exec.LookPath("mkvextract"); err == nil {
		return p
	}
	return ""
}

// DetectSubSDHFromContent détecte un sous-titre SDH en cherchant des marqueurs
// SPÉCIFIQUES (mots descriptifs en crochets ou notes de musique). Le but est
// d'éviter les faux positifs sur les pistes Full qui contiennent occasionnellement
// des crochets/parenthèses (notes de doublage, didascalies, etc.).
//
// Une piste est SDH si :
//   - 15+ crochets contenant un mot descriptif type sound-effect, OU
//   - 10+ notes de musique (♪, ♬, 🎵, 🎶)
//
// Tout autre cas (parenthèses, locuteurs, crochets vides) est ignoré pour
// rester conservateur (par défaut Full).
func DetectSubSDHFromContent(content string) (isSDH bool, score int) {
	// Crochets contenant des mots-clés sound-effect (FR + EN).
	sfxRe := regexp.MustCompile(`(?i)\[(bruit|son|sound|music|musique|chant|sing|sirène|siren|rire|laugh|applaudisse|applause|gasp|soupir|sigh|sanglot|sob|cri|scream|shout|chuchot|whisper|cogner|knock|frapper|fracas|crash|bang|bell|sonner|honk|klaxon|footstep|pas\b|respira|breathing|sniff|reniflé|moteur|engine|téléphone|telephone|sonnerie|ringtone|ordinateur|computer|vent|wind|eau|water|silence|porte|door|verre|glass|coup|hit|pleur|cry|halète|pant|tousse|cough|éternue|sneeze|fredonne|hum|grogne|growl|craquement|creak|tonnerre|thunder|explosion|tic-tac|tick|battement|beat|musique|music)`)
	musicRe := regexp.MustCompile(`[♪♬🎵🎶]`)
	sfxCount := len(sfxRe.FindAllStringIndex(content, -1))
	musicCount := len(musicRe.FindAllStringIndex(content, -1))
	score = sfxCount + musicCount
	isSDH = sfxCount >= 15 || musicCount >= 10
	return
}

// TrackSpec décrit comment renommer/traiter une piste lors du mux.
type TrackSpec struct {
	ID             int    // ID mkvmerge de la piste d'origine
	Type           string // "audio" | "subtitles" | "video"
	Keep           bool   // si false, la piste est exclue du mux (--audio-tracks / --subtitle-tracks)
	Name           string // nouveau nom (--track-name)
	Language       string // code iso 639-2 (fre, eng, jpn, ita, ger, spa, und…)
	Default        bool   // flag default
	Forced         bool   // flag forced
	VisualImpaired bool   // flag malvoyant (--visual-impaired-flag) — pour les pistes AD
	DelayMs        int    // décalage audio en ms (--sync TID:OFFSET), 0 = pas de décalage
	Order          int    // position dans l'ordre final (plus petit = plus haut)
}

// ExternalSub décrit un fichier de sous-titres externe ajouté au mux.
type ExternalSub struct {
	Path        string  // chemin du fichier .srt/.sup/.ass/.sub/.idx
	Name        string  // nom de piste LiHDL (--track-name)
	Language    string  // code iso 639-2 (fre, eng, …)
	Default     bool
	Forced      bool
	DelayMs     int     // décalage en ms (--sync 0:DELAY)
	TempoFactor float64 // ratio atempo détecté (1.0 = pas de drift). Si != 1, mkvmerge --sync ajoute un facteur o/p pour étirer les timecodes (drift linéaire FPS).
	Order       int     // position dans l'ordre final (plus petit = plus haut)
}

// ExternalAudio décrit un fichier audio externe ajouté au mux.
type ExternalAudio struct {
	Path           string
	Name           string
	Language       string
	Default        bool
	Forced         bool
	VisualImpaired bool    // flag malvoyant pour les pistes AD
	DelayMs        int     // décalage audio en ms (--sync 0:DELAY)
	TempoFactor    float64 // ratio atempo détecté (1.0 = pas de drift). Si != 1, mkvmerge --sync ajoute un facteur o/p pour étirer les timecodes.
	Order          int
}

// SecondaryTrack décrit une piste à reprendre depuis un .mkv secondaire
// (typiquement un release SUPPLY/FW pour récupérer ses audios/subs).
type SecondaryTrack struct {
	ID             int
	Name           string
	Language       string
	Default        bool
	Forced         bool
	VisualImpaired bool // flag malvoyant pour les pistes AD
	DelayMs        int  // décalage audio en ms (--sync TID:OFFSET sur le mkv secondaire)
	Order          int
}

// MuxParams regroupe toutes les instructions pour exécuter le mux.
type MuxParams struct {
	InputPath       string           // .mkv source primaire (vidéo gardée)
	OutputPath      string           // .mkv cible (chemin complet)
	Title           string           // titre global du conteneur (optionnel)
	Tracks          []TrackSpec      // pistes internes du .mkv source (avec Order)
	ExternalAudios  []ExternalAudio  // audios externes à ajouter
	ExternalSubs    []ExternalSub    // subs externes à ajouter
	SecondaryPath   string           // .mkv secondaire (audios/subs uniquement, sans vidéo)
	SecondaryAudios []SecondaryTrack // audios à reprendre depuis le secondaire
	SecondarySubs   []SecondaryTrack // subs à reprendre depuis le secondaire
	NoChapters      bool             // si true, ajoute --no-chapters sur l'input primaire
}

// MuxProgress est émis pendant le mux (0..100).
type MuxProgress struct {
	Percent int
}

// Mux exécute mkvmerge avec les paramètres demandés. Progress est appelé
// pour chaque ligne "Progress: XX%" parsée. Le ctx permet d'annuler le mux
// proprement (bouton Stop côté UI).
func Mux(ctx context.Context, binary string, p MuxParams, progress func(MuxProgress), logLine func(string)) error {
	args := buildArgs(p)
	if logLine != nil {
		// Log chaque argument sur sa propre ligne pour voir exactement ce
		// qui est envoyé à mkvmerge (évite toute ambiguïté de séparateur).
		logLine("CMD>> " + binary)
		for i, a := range args {
			logLine("ARG[" + strconv.Itoa(i) + "]=" + a)
		}
		// Version inline (compacte) pour debug rapide.
		var sb strings.Builder
		sb.WriteString("CMDLINE: ")
		sb.WriteString(binary)
		for _, a := range args {
			sb.WriteByte(' ')
			sb.WriteByte('\'')
			sb.WriteString(a)
			sb.WriteByte('\'')
		}
		logLine(sb.String())
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadString('\r') // mkvmerge émet \r pour progress
			if line == "" && err == io.EOF {
				return
			}
			line = strings.TrimSpace(strings.TrimRight(line, "\r\n"))
			if line == "" {
				if err == io.EOF {
					return
				}
				continue
			}
			if strings.HasPrefix(line, "Progress:") {
				s := strings.TrimPrefix(line, "Progress:")
				s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
				if pct, perr := strconv.Atoi(s); perr == nil && progress != nil {
					progress(MuxProgress{Percent: pct})
				}
				continue
			}
			if logLine != nil {
				logLine(line)
			}
			if err == io.EOF {
				return
			}
		}
	}()

	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() && logLine != nil {
			logLine(sc.Text())
		}
	}()

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

// buildArgs construit la ligne de commande mkvmerge. Les flags --track-name,
// --language, --default-track-flag, --forced-display-flag s'appliquent par ID.
// Les pistes non gardées sont exclues via --audio-tracks/--subtitle-tracks.
// Les subs externes sont ajoutés comme fichiers d'entrée supplémentaires.
// L'ordre final est contrôlé par --track-order (fileID:trackID).
func buildArgs(p MuxParams) []string {
	args := []string{"-o", p.OutputPath}
	if p.Title != "" {
		args = append(args, "--title", p.Title)
	}

	// File IDs :
	//   0 = InputPath (primaire)
	//   1 = SecondaryPath (si présent)
	//   suivants = ExternalAudios puis ExternalSubs
	hasSecondary := p.SecondaryPath != ""
	extAudFileIDStart := 1
	if hasSecondary {
		extAudFileIDStart = 2
	}
	extSubFileIDStart := extAudFileIDStart + len(p.ExternalAudios)

	// ---- Track order (option globale, placée au début) ----
	type ordered struct {
		order  int
		fileID int
		trkID  int
	}
	var all []ordered
	for _, t := range p.Tracks {
		if !t.Keep {
			continue
		}
		all = append(all, ordered{order: t.Order, fileID: 0, trkID: t.ID})
	}
	if hasSecondary {
		for _, st := range p.SecondaryAudios {
			all = append(all, ordered{order: st.Order, fileID: 1, trkID: st.ID})
		}
		for _, st := range p.SecondarySubs {
			all = append(all, ordered{order: st.Order, fileID: 1, trkID: st.ID})
		}
	}
	for i, a := range p.ExternalAudios {
		all = append(all, ordered{order: a.Order, fileID: extAudFileIDStart + i, trkID: 0})
	}
	for i, s := range p.ExternalSubs {
		all = append(all, ordered{order: s.Order, fileID: extSubFileIDStart + i, trkID: 0})
	}
	if len(all) > 1 {
		sort.SliceStable(all, func(i, j int) bool { return all[i].order < all[j].order })
		parts := make([]string, 0, len(all))
		for _, o := range all {
			parts = append(parts, strconv.Itoa(o.fileID)+":"+strconv.Itoa(o.trkID))
		}
		args = append(args, "--track-order", strings.Join(parts, ","))
	}

	// ---- Fichier source (fileID 0) ----

	// Renommage des pistes gardées du fichier source.
	for _, t := range p.Tracks {
		if !t.Keep {
			continue
		}
		id := strconv.Itoa(t.ID)
		if t.Name != "" {
			args = append(args, "--track-name", id+":"+t.Name)
		}
		if t.Language != "" {
			args = append(args, "--language", id+":"+t.Language)
		}
		args = append(args, "--default-track-flag", id+":"+boolFlag(t.Default))
		args = append(args, "--forced-display-flag", id+":"+boolFlag(t.Forced))
		if t.VisualImpaired {
			args = append(args, "--visual-impaired-flag", id+":1")
		}
		if t.DelayMs != 0 {
			args = append(args, "--sync", id+":"+strconv.Itoa(t.DelayMs))
		}
	}

	// Filtrage audio/subs internes.
	audioKept, subsKept := []string{}, []string{}
	audioAny, subsAny := false, false
	for _, t := range p.Tracks {
		switch t.Type {
		case "audio":
			audioAny = true
			if t.Keep {
				audioKept = append(audioKept, strconv.Itoa(t.ID))
			}
		case "subtitles":
			subsAny = true
			if t.Keep {
				subsKept = append(subsKept, strconv.Itoa(t.ID))
			}
		}
	}
	if audioAny {
		if len(audioKept) == 0 {
			args = append(args, "--no-audio")
		} else {
			args = append(args, "--audio-tracks", strings.Join(audioKept, ","))
		}
	}
	if subsAny {
		if len(subsKept) == 0 {
			args = append(args, "--no-subtitles")
		} else {
			args = append(args, "--subtitle-tracks", strings.Join(subsKept, ","))
		}
	}
	if p.NoChapters {
		args = append(args, "--no-chapters")
	}
	args = append(args, p.InputPath)

	// ---- Fichier secondaire (fileID 1) — audios + subs uniquement ----
	if hasSecondary {
		// Renommage des pistes du secondaire (clé = ID dans le mkv secondaire).
		for _, st := range p.SecondaryAudios {
			id := strconv.Itoa(st.ID)
			if st.Name != "" {
				args = append(args, "--track-name", id+":"+st.Name)
			}
			if st.Language != "" {
				args = append(args, "--language", id+":"+st.Language)
			}
			args = append(args, "--default-track-flag", id+":"+boolFlag(st.Default))
			args = append(args, "--forced-display-flag", id+":"+boolFlag(st.Forced))
			if st.VisualImpaired {
				args = append(args, "--visual-impaired-flag", id+":1")
			}
			if st.DelayMs != 0 {
				args = append(args, "--sync", id+":"+strconv.Itoa(st.DelayMs))
			}
		}
		for _, st := range p.SecondarySubs {
			id := strconv.Itoa(st.ID)
			if st.Name != "" {
				args = append(args, "--track-name", id+":"+st.Name)
			}
			if st.Language != "" {
				args = append(args, "--language", id+":"+st.Language)
			}
			args = append(args, "--default-track-flag", id+":"+boolFlag(st.Default))
			args = append(args, "--forced-display-flag", id+":"+boolFlag(st.Forced))
		}
		// Filtrage : pas de vidéo, ne garder que les pistes listées.
		args = append(args, "--no-video")
		secAudIDs, secSubIDs := []string{}, []string{}
		for _, st := range p.SecondaryAudios {
			secAudIDs = append(secAudIDs, strconv.Itoa(st.ID))
		}
		for _, st := range p.SecondarySubs {
			secSubIDs = append(secSubIDs, strconv.Itoa(st.ID))
		}
		if len(secAudIDs) > 0 {
			args = append(args, "--audio-tracks", strings.Join(secAudIDs, ","))
		} else {
			args = append(args, "--no-audio")
		}
		if len(secSubIDs) > 0 {
			args = append(args, "--subtitle-tracks", strings.Join(secSubIDs, ","))
		} else {
			args = append(args, "--no-subtitles")
		}
		args = append(args, p.SecondaryPath)
	}

	// ---- Fichiers audio externes ----
	for _, a := range p.ExternalAudios {
		if a.Name != "" {
			args = append(args, "--track-name", "0:"+a.Name)
		}
		if a.Language != "" {
			args = append(args, "--language", "0:"+a.Language)
		}
		args = append(args, "--default-track-flag", "0:"+boolFlag(a.Default))
		args = append(args, "--forced-display-flag", "0:"+boolFlag(a.Forced))
		if a.VisualImpaired {
			args = append(args, "--visual-impaired-flag", "0:1")
		}
		if syncVal := buildSyncValue(a.DelayMs, a.TempoFactor); syncVal != "" {
			args = append(args, "--sync", "0:"+syncVal)
		}
		args = append(args, a.Path)
	}

	// ---- Fichiers subs externes (fileID N+1..N+M) ----
	for _, s := range p.ExternalSubs {
		if s.Name != "" {
			args = append(args, "--track-name", "0:"+s.Name)
		}
		if s.Language != "" {
			args = append(args, "--language", "0:"+s.Language)
		}
		args = append(args, "--default-track-flag", "0:"+boolFlag(s.Default))
		args = append(args, "--forced-display-flag", "0:"+boolFlag(s.Forced))
		if syncVal := buildSyncValue(s.DelayMs, s.TempoFactor); syncVal != "" {
			args = append(args, "--sync", "0:"+syncVal)
		}
		args = append(args, s.Path)
	}

	return args
}

// buildSyncValue construit la valeur du flag --sync mkvmerge :
//   - "" si pas de décalage et pas de drift (rien à passer)
//   - "DELAY" si juste un offset constant (ex: "20")
//   - "DELAY,o/p" si en plus un drift linéaire (ex: "20,1001/1000")
//
// Le ratio o/p compense un drift FPS : new_ts = (old_ts + d) * o/p.
// On veut new_ts = old_ts * (1/tempoFactor), donc o/p = 1/tempoFactor.
func buildSyncValue(delayMs int, tempoFactor float64) string {
	hasDelay := delayMs != 0
	hasTempo := tempoFactor > 0 && tempoFactor != 1.0
	if !hasDelay && !hasTempo {
		return ""
	}
	val := strconv.Itoa(delayMs)
	if hasTempo {
		// Convertit 1/tempoFactor en fraction o/p avec 6 chiffres de précision.
		// Ex : tempoFactor = 0.999 → 1/0.999 ≈ 1.001001 → 1001001/1000000.
		ratio := 1.0 / tempoFactor
		num := int(ratio*1000000 + 0.5) // arrondi
		val += "," + strconv.Itoa(num) + "/1000000"
	}
	return val
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
