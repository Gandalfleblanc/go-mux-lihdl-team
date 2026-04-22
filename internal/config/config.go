// Package config gère la persistance de la configuration utilisateur.
// Stockage dans ~/Library/Application Support/go-mux-lihdl-team/config.json (macOS)
// ou équivalent OS via os.UserConfigDir(). Permissions 0600.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	TmdbKey          string `json:"tmdb_key"`
	ServeurPersoURL  string `json:"serveurperso_url"` // base URL de l'index serveurperso (ex: https://www.serveurperso.com/stats/search.php)
	OutputDir        string `json:"output_dir"`       // dossier de sortie pour les .mkv muxés
	MkvmergePath     string `json:"mkvmerge_path"`    // override manuel du binaire mkvmerge (sinon auto-détection)
	DefaultEncoder   string `json:"default_encoder"`  // encodeur pré-sélectionné (ex: GANDALF)
	DefaultTeam      string `json:"default_team"`     // team pré-sélectionnée (ex: LiHDL)
	DefaultQuality   string `json:"default_quality"`  // qualité pré-sélectionnée (ex: HDLight)
	DefaultSource    string `json:"default_source"`   // source pré-sélectionnée (ex: REMUX LiHDL)
}

func configDir() (string, error) {
	home, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "go-mux-lihdl-team")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func Dir() (string, error) { return configDir() }

func Path() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// BinDir retourne le dossier où sera téléchargé mkvmerge au 1er lancement
// (pattern auto-download). Créé à la demande.
func BinDir() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		return "", err
	}
	return bin, nil
}

func Load() Config {
	path, err := Path()
	if err != nil {
		return defaultConfig()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig()
	}
	c := defaultConfig()
	_ = json.Unmarshal(data, &c)
	return c
}

func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func defaultConfig() Config {
	return Config{
		ServeurPersoURL: "https://www.serveurperso.com/stats/search.php",
		DefaultTeam:     "LiHDL",
		DefaultQuality:  "HDLight",
		DefaultSource:   "REMUX LiHDL",
	}
}

// Complete retourne true si la config minimum est renseignée (au moins
// un dossier de sortie). La clé TMDB est optionnelle — l'app peut muxer
// sans TMDB mais il faudra saisir le titre manuellement.
func (c Config) Complete() bool {
	return c.OutputDir != ""
}
