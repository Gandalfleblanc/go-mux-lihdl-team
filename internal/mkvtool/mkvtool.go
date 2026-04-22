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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Locate trouve le chemin du binaire mkvmerge selon la priorité suivante :
//  1. override explicite (depuis la config)
//  2. binaire téléchargé dans le dossier de l'app (auto-download au 1er run)
//  3. binaire système sur PATH
func Locate(configOverride, appBinDir string) (string, error) {
	if configOverride != "" {
		if _, err := exec.LookPath(configOverride); err == nil {
			return configOverride, nil
		}
	}
	if appBinDir != "" {
		name := "mkvmerge"
		if runtime.GOOS == "windows" {
			name = "mkvmerge.exe"
		}
		candidate := filepath.Join(appBinDir, name)
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("mkvmerge"); err == nil {
		return p, nil
	}
	return "", errors.New("mkvmerge introuvable (ni override, ni téléchargé, ni sur PATH)")
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
}

// MuxParams regroupe toutes les instructions pour exécuter le mux.
type MuxParams struct {
	InputPath  string      // .mkv source
	OutputPath string      // .mkv cible (chemin complet)
	Title      string      // titre global du conteneur (optionnel)
	Tracks     []TrackSpec // une entrée par piste source (audio + video + subs)
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
// --language, --default-track-flag, --forced-track-flag s'appliquent par ID.
// Les pistes non gardées sont exclues via --audio-tracks/--subtitle-tracks.
func buildArgs(p MuxParams) []string {
	args := []string{"-o", p.OutputPath}
	if p.Title != "" {
		args = append(args, "--title", p.Title)
	}

	// Applique le renommage sur les pistes gardées.
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
		args = append(args, "--forced-track-flag", id+":"+boolFlag(t.Forced))
	}

	// Filtrage des pistes : --audio-tracks / --subtitle-tracks listent les
	// IDs À GARDER. Si aucune piste d'un type n'est gardée, on utilise
	// --no-audio / --no-subtitles pour être explicite.
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
	return args
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
