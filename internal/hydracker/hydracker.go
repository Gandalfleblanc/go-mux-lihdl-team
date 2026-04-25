// Package hydracker fournit un wrapper minimal autour de l'API Hydracker
// pour résoudre l'URL fiche d'un titre à partir de son ID TMDB.
package hydracker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Cache mémoire des résolutions TMDB → URL fiche (évite spam + rate limit 1 req/s).
type cacheEntry struct {
	url string
	at  time.Time
}

var (
	urlCache   = map[int]cacheEntry{}
	urlCacheMu sync.Mutex
	cacheTTL   = 12 * time.Hour
)

// Title est la vue minimale d'un titre depuis l'API Hydracker /titles?tmdb_id=...
type Title struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// TestKey vérifie qu'une clé API Hydracker est valide en interrogeant
// /user-profile/me (endpoint authentifié). Retourne ok=true si 200,
// false avec un message décrivant l'erreur sinon.
func TestKey(apiKey string) (bool, string) {
	if apiKey == "" {
		return false, "clé vide"
	}
	req, err := http.NewRequest("GET", "https://hydracker.com/api/v1/user-profile/me", nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "GoMuxLiHDLTeam/1.0 (mux-app)")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		return true, "clé valide ✓"
	case 401:
		return false, "clé invalide (401)"
	case 403:
		return false, "accès refusé (403) — vérifie les permissions API"
	case 429:
		return false, "rate limit (429) — réessaie dans quelques secondes"
	default:
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
}

// LookupURL retourne l'URL complète de la fiche Hydracker pour un ID TMDB,
// ou chaîne vide si introuvable. Cache 12h pour limiter les requêtes.
// Nécessite la clé API Hydracker.
func LookupURL(tmdbID int, apiKey string) (string, error) {
	if tmdbID <= 0 || apiKey == "" {
		return "", fmt.Errorf("tmdbID ou clé API manquants")
	}

	// Cache
	urlCacheMu.Lock()
	if e, ok := urlCache[tmdbID]; ok && time.Since(e.at) < cacheTTL {
		urlCacheMu.Unlock()
		return e.url, nil
	}
	urlCacheMu.Unlock()

	u := "https://hydracker.com/api/v1/titles?tmdb_id=" + strconv.Itoa(tmdbID)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "GoMuxLiHDLTeam/1.0 (mux-app)")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Hydracker HTTP %d", resp.StatusCode)
	}

	// Format paginé : { status, pagination: { data: [{id, name, slug}] } }
	// On essaie aussi le format direct { data: [...] } au cas où.
	var body struct {
		Pagination struct {
			Data []Title `json:"data"`
		} `json:"pagination"`
		Data []Title `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}

	titles := body.Pagination.Data
	if len(titles) == 0 {
		titles = body.Data
	}
	if len(titles) == 0 {
		// Cache le vide pour éviter de re-spam pour rien
		urlCacheMu.Lock()
		urlCache[tmdbID] = cacheEntry{url: "", at: time.Now()}
		urlCacheMu.Unlock()
		return "", nil
	}

	t := titles[0]
	url := "https://hydracker.com/titles/" + strconv.Itoa(t.ID)
	if t.Slug != "" {
		url += "/" + t.Slug
	}

	urlCacheMu.Lock()
	urlCache[tmdbID] = cacheEntry{url: url, at: time.Now()}
	urlCacheMu.Unlock()
	return url, nil
}
