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

	// LanguageTool (étape post-cleanup OCR pour passer de ~99.3% à ~99.85%).
	// Sans clé : API publique (20 req/min, 20 KB/req). Avec clé : Premium.
	LanguageToolKey  string `json:"languagetool_key"`  // apiKey LT Premium (optionnel)
	LanguageToolUser string `json:"languagetool_user"` // username LT Premium (optionnel)
	LanguageToolURL  string `json:"languagetool_url"`  // override endpoint (optionnel)

	// OpenSubtitles (recherche de SRT existant avant OCR — gain de minutes).
	// Clé API gratuite via opensubtitles.com → Profile → Consumers.
	OpenSubtitlesAPIKey string `json:"opensubtitles_api_key"`

	// Index Discord (admin only — token et forum ID).
	// DiscordIndexURL est lue par TOUS les users pour fetch le JSON public.
	DiscordBotToken string `json:"discord_bot_token"`
	DiscordForumID  string `json:"discord_forum_id"`
	DiscordIndexURL string `json:"discord_index_url"`

	// Push GitHub (admin) — pour pusher l'index Discord directement sur le repo
	// sans passer par le site web GitHub.
	GitHubToken         string `json:"github_token"`            // PAT avec scope `repo`
	GitHubRepo          string `json:"github_repo"`             // ex: Gandalfleblanc/go-mux-lihdl-team
	GitHubBranch        string `json:"github_branch"`           // default: main
	GitHubIndexFilePath string `json:"github_index_file_path"`  // ex: discord_index.json
}

// DiscordIndexPath retourne le chemin du JSON local de l'index Discord
// (utilisé en cache user et en sortie admin du scan). Le dossier parent est
// créé à la demande.
func DiscordIndexPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "discord_index.json"), nil
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

// CacheDir retourne le dossier où sont stockés les artefacts cache (OCR par
// hash du .sup, dictionnaire custom OCR, etc.). Créé à la demande.
// Sous-dossiers conventionnels : "ocr-cache/" (SRT par sha256), "ocr-custom-dict.json".
func CacheDir() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	cache := filepath.Join(dir, "ocr-cache")
	if err := os.MkdirAll(cache, 0755); err != nil {
		return "", err
	}
	return cache, nil
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
