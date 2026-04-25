// Package mediainfo fournit un wrapper autour de la CLI mediainfo :
//   - localisation du binaire (override → embed → PATH)
//   - Identify : parse la sortie "mediainfo --Output=JSON"
//
// Sert à enrichir l'analyse des pistes (track names détaillés, format profile,
// service kind, etc.) en complément de mkvmerge -J.
package mediainfo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// Locate retourne le chemin du binaire mediainfo selon la priorité :
//  1. binaire embarqué dans l'app (extrait à appBinDir au 1er run)
//  2. binaire système sur PATH
func Locate(appBinDir string) (string, error) {
	if appBinDir != "" && len(embeddedBinary) > 0 {
		candidate := filepath.Join(appBinDir, embeddedName)
		if _, err := os.Stat(candidate); err != nil {
			if werr := os.WriteFile(candidate, embeddedBinary, 0755); werr == nil {
				return candidate, nil
			}
		} else {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("mediainfo"); err == nil {
		return p, nil
	}
	return "", errors.New("mediainfo introuvable (ni embarqué, ni sur PATH)")
}

// Track est la vue simplifiée d'une piste depuis mediainfo JSON.
type Track struct {
	Type                     string `json:"@type"`                    // "Video" | "Audio" | "Text" | "General"
	ID                       string `json:"ID"`                       // ID de la piste (string dans mediainfo)
	Title                    string `json:"Title"`                    // titre de la piste (équivalent track_name)
	Language                 string `json:"Language"`                 // code ISO
	Format                   string `json:"Format"`                   // ex: "AC-3", "E-AC-3", "DTS"
	FormatProfile            string `json:"Format_Profile"`           // ex: "MA", "Core", "JOC"
	FormatCommercial         string `json:"Format_Commercial"`        // ex: "Dolby Digital Plus with Dolby Atmos"
	FormatCommercialIfAny    string `json:"Format_Commercial_IfAny"`  // alternative key (présente sur certaines versions)
	FormatAdditionalFeatures string `json:"Format_AdditionalFeatures"` // ex: "JOC", "XLL"
	Channels                 string `json:"Channels"`                 // string "6", "2", etc.
	Default                  string `json:"Default"`                  // "Yes" / "No"
	Forced                   string `json:"Forced"`                   // "Yes" / "No"
	ServiceKind              string `json:"ServiceKind"`              // ex: "VI", "HI"
	ServiceKindNames         string `json:"ServiceKind/String"`       // texte du service kind
	StreamSize               string `json:"StreamSize"`               // octets
	ElementCount             string `json:"ElementCount"`             // nombre d'éléments (subs)
	Width                    string `json:"Width"`
	Height                   string `json:"Height"`
}

// Info est la structure renvoyée par mediainfo --Output=JSON.
type Info struct {
	Media struct {
		Track []Track `json:"track"`
	} `json:"media"`
}

// Identify exécute "mediainfo --Output=JSON <file>" et décode le JSON.
func Identify(ctx context.Context, binary, mkvPath string) (*Info, error) {
	cmd := exec.CommandContext(ctx, binary, "--Output=JSON", mkvPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
