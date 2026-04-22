package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"go-mux-lihdl-team/internal/config"
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

	// Drop-zone : .mkv glissés dans l'app → émis au frontend via "file:dropped".
	wr.OnFileDrop(ctx, func(x, y int, paths []string) {
		var mkvs []string
		for _, p := range paths {
			if strings.EqualFold(filepath.Ext(p), ".mkv") {
				mkvs = append(mkvs, p)
			}
		}
		if len(mkvs) == 0 {
			wr.EventsEmit(ctx, "log", "⚠ Aucun fichier .mkv détecté dans le drop")
			return
		}
		// Pour le MVP on traite le 1er .mkv déposé. Batch à ajouter plus tard.
		wr.EventsEmit(ctx, "file:dropped", mkvs[0])
	})
}

// --- Version ---

func (a *App) GetVersion() string { return "0.1.0-dev" }

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

func (a *App) SelectOutputDir() (string, error) {
	return wr.OpenDirectoryDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Choisir le dossier de sortie",
	})
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
		// On rebuild en []map[string]any — Wails EventsEmit a des soucis
		// avec les slices de structs imbriqués, mais accepte bien les maps.
		tracksPayload := make([]map[string]any, 0, len(info.Tracks))
		for _, t := range info.Tracks {
			tracksPayload = append(tracksPayload, map[string]any{
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
			})
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

// --- TMDB ---

func (a *App) SearchTmdb(query string) ([]tmdb.Result, error) {
	c := config.Load()
	return tmdb.Search(c.ServeurPersoURL, query)
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
	InputPath  string              `json:"input_path"`
	OutputPath string              `json:"output_path"`
	Title      string              `json:"title"`
	Tracks     []mkvtool.TrackSpec `json:"tracks"`
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
		InputPath:  req.InputPath,
		OutputPath: req.OutputPath,
		Title:      req.Title,
		Tracks:     req.Tracks,
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
