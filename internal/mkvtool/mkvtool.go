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
	// Embedded binary : extrait à appBinDir si non présent et que embed non vide.
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

// TrackSpec décrit comment renommer/traiter une piste lors du mux.
type TrackSpec struct {
	ID       int    // ID mkvmerge de la piste d'origine
	Type     string // "audio" | "subtitles" | "video"
	Keep     bool   // si false, la piste est exclue du mux (--audio-tracks / --subtitle-tracks)
	Name     string // nouveau nom (--track-name)
	Language string // code iso 639-2 (fre, eng, jpn, ita, ger, spa, und…)
	Default  bool   // flag default
	Forced   bool   // flag forced
	Order    int    // position dans l'ordre final (plus petit = plus haut)
}

// ExternalSub décrit un fichier de sous-titres externe ajouté au mux.
type ExternalSub struct {
	Path     string // chemin du fichier .srt/.sup/.ass/.sub/.idx
	Name     string // nom de piste LiHDL (--track-name)
	Language string // code iso 639-2 (fre, eng, …)
	Default  bool
	Forced   bool
	Order    int // position dans l'ordre final (plus petit = plus haut)
}

// ExternalAudio décrit un fichier audio externe ajouté au mux.
type ExternalAudio struct {
	Path     string // chemin du fichier audio (.ac3/.eac3/.dts/.aac/.flac/.mka…)
	Name     string // nom de piste LiHDL (--track-name)
	Language string // code iso 639-2 (fre, eng, …)
	Default  bool
	Forced   bool
	Order    int // position dans l'ordre final (plus petit = plus haut)
}

// MuxParams regroupe toutes les instructions pour exécuter le mux.
type MuxParams struct {
	InputPath      string          // .mkv source
	OutputPath     string          // .mkv cible (chemin complet)
	Title          string          // titre global du conteneur (optionnel)
	Tracks         []TrackSpec     // pistes internes du .mkv source (avec Order)
	ExternalAudios []ExternalAudio // audios externes à ajouter
	ExternalSubs   []ExternalSub   // subs externes à ajouter
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
	// External audios : fileID 1..N, external subs : fileID N+1..N+M
	for i, a := range p.ExternalAudios {
		all = append(all, ordered{order: a.Order, fileID: i + 1, trkID: 0})
	}
	nAud := len(p.ExternalAudios)
	for i, s := range p.ExternalSubs {
		all = append(all, ordered{order: s.Order, fileID: nAud + i + 1, trkID: 0})
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
	args = append(args, p.InputPath)

	// ---- Fichiers audio externes (fileID 1..N) ----
	for _, a := range p.ExternalAudios {
		if a.Name != "" {
			args = append(args, "--track-name", "0:"+a.Name)
		}
		if a.Language != "" {
			args = append(args, "--language", "0:"+a.Language)
		}
		args = append(args, "--default-track-flag", "0:"+boolFlag(a.Default))
		args = append(args, "--forced-display-flag", "0:"+boolFlag(a.Forced))
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
		args = append(args, s.Path)
	}

	return args
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
