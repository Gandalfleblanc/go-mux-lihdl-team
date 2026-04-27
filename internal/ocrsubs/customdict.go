// Package ocrsubs — dictionnaire custom enrichissable.
//
// Stocke un mapping "fragment fautif → correction" persistant dans
// ~/Library/Application Support/go-mux-lihdl-team/ocr-custom-dict.json.
// Le contenu est fusionné avec `nameFixes` à chaque appel de cleanLine.
//
// Format JSON :
//
//	{
//	  "version": 1,
//	  "entries": [
//	    {"wrong": "Charli xex", "right": "Charli XCX", "added_at": "...", "auto": true}
//	  ]
//	}
//
// `auto` = true quand l'entrée a été ajoutée automatiquement après validation
// d'un fix par l'utilisateur dans le modal "lignes à vérifier".
package ocrsubs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-mux-lihdl-team/internal/config"
)

// CustomDictEntry : une paire wrong→right enrichie de métadonnées.
type CustomDictEntry struct {
	Wrong   string `json:"wrong"`
	Right   string `json:"right"`
	AddedAt string `json:"added_at"`
	Auto    bool   `json:"auto"`
}

// CustomDictFile : structure racine du fichier JSON.
type CustomDictFile struct {
	Version int               `json:"version"`
	Entries []CustomDictEntry `json:"entries"`
}

const customDictFileName = "ocr-custom-dict.json"
const customDictVersion = 1

// dictMu protège l'accès concurrent au fichier JSON.
var dictMu sync.RWMutex

// dictCache : map wrong→right chargée en mémoire pour cleanLine.
// Invalidé après chaque Add/Remove.
var (
	dictCache    map[string]string
	dictCacheSet bool
)

// customDictPath retourne le chemin du fichier JSON. Le dossier parent est
// créé via config.Dir() (au besoin par configDir()).
func customDictPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, customDictFileName), nil
}

// readDictFile lit le fichier JSON sur disque (sans verrou — appelants).
// Retourne un fichier vide si absent (premier lancement).
func readDictFile() (CustomDictFile, error) {
	var f CustomDictFile
	f.Version = customDictVersion
	path, err := customDictPath()
	if err != nil {
		return f, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		// Fichier corrompu : on log via le retour, l'appelant peut décider.
		return f, err
	}
	if f.Version == 0 {
		f.Version = customDictVersion
	}
	return f, nil
}

// writeDictFile écrit le fichier JSON sur disque (sans verrou — appelants).
func writeDictFile(f CustomDictFile) error {
	path, err := customDictPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadCustomDict charge le dictionnaire en mémoire (cache) et retourne la
// map wrong→right utilisée par cleanLine. Best-effort : retourne map vide si
// erreur de lecture/parse (on ne casse jamais le pipeline OCR).
func LoadCustomDict() (map[string]string, error) {
	dictMu.RLock()
	if dictCacheSet {
		m := dictCache
		dictMu.RUnlock()
		return m, nil
	}
	dictMu.RUnlock()

	dictMu.Lock()
	defer dictMu.Unlock()
	// Re-check après promotion du verrou.
	if dictCacheSet {
		return dictCache, nil
	}
	f, err := readDictFile()
	if err != nil {
		dictCache = map[string]string{}
		dictCacheSet = true
		return dictCache, err
	}
	m := make(map[string]string, len(f.Entries))
	for _, e := range f.Entries {
		w := strings.TrimSpace(e.Wrong)
		r := strings.TrimSpace(e.Right)
		if w == "" || r == "" {
			continue
		}
		m[w] = r
	}
	dictCache = m
	dictCacheSet = true
	return dictCache, nil
}

// invalidateDictCache force le rechargement à la prochaine LoadCustomDict.
func invalidateDictCache() {
	dictCacheSet = false
	dictCache = nil
}

// AddCustomDictEntry ajoute (ou met à jour) une entrée dans le dictionnaire.
// `auto` = true si l'entrée vient du flow "validation modal review".
// Si la même clé `wrong` existe déjà, son `right` est remplacé.
func AddCustomDictEntry(wrong, right string, auto bool) error {
	wrong = strings.TrimSpace(wrong)
	right = strings.TrimSpace(right)
	if wrong == "" || right == "" {
		return errors.New("wrong/right vide")
	}
	if wrong == right {
		// Pas la peine de stocker une entrée identité — possible si la
		// "correction" choisie est identique au snippet d'origine.
		return nil
	}
	dictMu.Lock()
	defer dictMu.Unlock()
	f, err := readDictFile()
	if err != nil && !os.IsNotExist(err) {
		// Fichier corrompu ou permission denied : on continue best-effort
		// et on réécrit un fichier propre avec uniquement la nouvelle entrée.
		f = CustomDictFile{Version: customDictVersion}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	found := false
	for i, e := range f.Entries {
		if e.Wrong == wrong {
			f.Entries[i].Right = right
			f.Entries[i].AddedAt = now
			f.Entries[i].Auto = auto
			found = true
			break
		}
	}
	if !found {
		f.Entries = append(f.Entries, CustomDictEntry{
			Wrong:   wrong,
			Right:   right,
			AddedAt: now,
			Auto:    auto,
		})
	}
	if err := writeDictFile(f); err != nil {
		return err
	}
	invalidateDictCache()
	return nil
}

// RemoveCustomDictEntry retire une entrée par sa clé `wrong`. No-op si absent.
func RemoveCustomDictEntry(wrong string) error {
	wrong = strings.TrimSpace(wrong)
	if wrong == "" {
		return errors.New("wrong vide")
	}
	dictMu.Lock()
	defer dictMu.Unlock()
	f, err := readDictFile()
	if err != nil {
		return err
	}
	out := f.Entries[:0]
	for _, e := range f.Entries {
		if e.Wrong == wrong {
			continue
		}
		out = append(out, e)
	}
	f.Entries = out
	if err := writeDictFile(f); err != nil {
		return err
	}
	invalidateDictCache()
	return nil
}

// ListCustomDictEntries retourne toutes les entrées (pour la modal Settings).
func ListCustomDictEntries() ([]CustomDictEntry, error) {
	dictMu.RLock()
	defer dictMu.RUnlock()
	f, err := readDictFile()
	if err != nil {
		return nil, err
	}
	if f.Entries == nil {
		return []CustomDictEntry{}, nil
	}
	return f.Entries, nil
}
