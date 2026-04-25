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
	TmdbKey         string `json:"tmdb_key"`
	ServeurPersoURL string `json:"serveurperso_url"` // index TMDB primaire (par défaut tmdb.uklm.xyz)
	FallbackIndex   string `json:"fallback_index"`   // index TMDB fallback (par défaut serveurperso)
	HydrackerKey    string `json:"hydracker_key"`    // clé API Hydracker (recherche fiches par TMDB ID)
	UnfrKey         string `json:"unfr_key"`         // clé API UNFR.pw
	OutputDir       string `json:"output_dir"`       // dossier de sortie par défaut (legacy / fallback)
	OutputDirLihdl  string `json:"output_dir_lihdl"` // dossier de sortie spécifique au mode MUX LiHDL
	OutputDirPSA    string `json:"output_dir_psa"`   // dossier de sortie spécifique au mode MUX CUSTOM PSA SERIES
	MkvmergePath    string `json:"mkvmerge_path"`    // override manuel du binaire mkvmerge (sinon auto-détection)
	DefaultEncoder  string `json:"default_encoder"`  // encodeur pré-sélectionné
	DefaultTeam     string `json:"default_team"`     // team pré-sélectionnée
	DefaultQuality  string `json:"default_quality"`  // qualité pré-sélectionnée
	DefaultSource   string `json:"default_source"`   // source pré-sélectionnée
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
		ServeurPersoURL: "https://tmdb.uklm.xyz/search.php",
		FallbackIndex:   "https://www.serveurperso.com/stats/search.php",
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
