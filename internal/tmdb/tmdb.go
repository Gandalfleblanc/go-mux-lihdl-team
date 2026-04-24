// Package tmdb fournit la recherche de fiches films via l'index de scrape
// serveurperso.com + fetch du poster depuis themoviedb.org. Code adapté
// depuis LiHDL Post Discord (même source de vérité).
package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Result struct {
	TmdbID    string  `json:"tmdb_id"`
	Note      float64 `json:"note"`
	TitreFR   string  `json:"titre_fr"`
	AnneeFR   string  `json:"annee_fr"`
	TitreVO   string  `json:"titre_vo"`
	Duree     string  `json:"duree"`
	URL       string  `json:"url"`
	PosterURL string  `json:"poster_url"`
}

var regexOgImage = regexp.MustCompile(`<meta\s+property="og:image"\s+content="([^"]+)"`)

func fetchPoster(tmdbID string) string {
	u := "https://www.themoviedb.org/movie/" + tmdbID + "?language=fr"
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	if m := regexOgImage.FindSubmatch(raw); m != nil {
		return string(m[1])
	}
	return ""
}

var videoExtRe = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|mov|wmv)$`)
var regexTMDB = regexp.MustCompile(`themoviedb\.org/movie/(\d+)[^<]*</a>\s*\d+\s*<b>([\d.]+)</b>`)
var regexTitreFR = regexp.MustCompile(`FR\s*<b>([^<]+)</b>\s*(\d{4})`)
var regexTitreVO = regexp.MustCompile(`VO\s*<b>([^<]+)</b>\s*(\d{4})`)
var regexDuree = regexp.MustCompile(`(\d+)\s*h\s*(\d+)\s*min`)

var htmlEntities = map[string]string{
	"&eacute;": "é", "&egrave;": "è", "&ecirc;": "ê",
	"&agrave;": "à", "&acirc;": "â", "&auml;": "ä",
	"&ocirc;": "ô", "&ouml;": "ö", "&ucirc;": "û",
	"&uuml;": "ü", "&icirc;": "î", "&iuml;": "ï",
	"&ccedil;": "ç", "&laquo;": "«", "&raquo;": "»",
	"&#039;": "'", "&quot;": "\"", "&amp;": "&",
	"&nbsp;": " ", "&hellip;": "…",
}

func decodeHTML(s string) string {
	for k, v := range htmlEntities {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}

// FetchByID récupère une fiche TMDB directement via l'API officielle par son ID
// numérique. Nécessite une clé API TMDB. Retourne un Result unique.
func FetchByID(id, apiKey string) (*Result, error) {
	if id == "" || apiKey == "" {
		return nil, fmt.Errorf("id ou clé TMDB manquants")
	}
	u := "https://api.themoviedb.org/3/movie/" + url.PathEscape(id) + "?language=fr&api_key=" + url.QueryEscape(apiKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB HTTP %d", resp.StatusCode)
	}
	var body struct {
		ID            int     `json:"id"`
		Title         string  `json:"title"`
		OriginalTitle string  `json:"original_title"`
		ReleaseDate   string  `json:"release_date"`
		Runtime       int     `json:"runtime"`
		PosterPath    string  `json:"poster_path"`
		VoteAverage   float64 `json:"vote_average"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.ID == 0 {
		return nil, fmt.Errorf("TMDB ID %s introuvable", id)
	}
	year := ""
	if len(body.ReleaseDate) >= 4 {
		year = body.ReleaseDate[:4]
	}
	duree := ""
	if body.Runtime > 0 {
		duree = fmt.Sprintf("%dh%02dmin", body.Runtime/60, body.Runtime%60)
	}
	poster := ""
	if body.PosterPath != "" {
		poster = "https://image.tmdb.org/t/p/w500" + body.PosterPath
	}
	return &Result{
		TmdbID:    strconv.Itoa(body.ID),
		Note:      body.VoteAverage,
		TitreFR:   body.Title,
		AnneeFR:   year,
		TitreVO:   body.OriginalTitle,
		Duree:     duree,
		URL:       "https://www.themoviedb.org/movie/" + strconv.Itoa(body.ID) + "?language=fr",
		PosterURL: poster,
	}, nil
}

// Search interroge l'index serveurperso et retourne les fiches TMDB matchées.
// baseURL par défaut = https://www.serveurperso.com/stats/search.php
func Search(baseURL, query string) ([]Result, error) {
	if baseURL == "" {
		baseURL = "https://www.serveurperso.com/stats/search.php"
	}
	requete := videoExtRe.ReplaceAllString(query, "")
	u := baseURL + "?query=" + url.QueryEscape(requete)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(raw)

	matches := regexTMDB.FindAllStringSubmatchIndex(html, -1)
	results := make([]Result, 0, len(matches))
	for _, m := range matches {
		idStart, idEnd := m[2], m[3]
		noteStart, noteEnd := m[4], m[5]
		note, _ := strconv.ParseFloat(html[noteStart:noteEnd], 64)
		tmdbID := html[idStart:idEnd]

		posIdx := m[0]
		end := posIdx + 3000
		if end > len(html) {
			end = len(html)
		}
		extrait := html[posIdx:end]

		r := Result{
			TmdbID: tmdbID,
			Note:   note,
			URL:    "https://www.themoviedb.org/movie/" + tmdbID + "?language=fr",
		}
		if tm := regexTitreFR.FindStringSubmatch(extrait); tm != nil {
			r.TitreFR = decodeHTML(strings.TrimSpace(tm[1]))
			r.AnneeFR = tm[2]
		}
		if tm := regexTitreVO.FindStringSubmatch(extrait); tm != nil {
			r.TitreVO = decodeHTML(strings.TrimSpace(tm[1]))
		}
		if dm := regexDuree.FindStringSubmatch(extrait); dm != nil {
			r.Duree = dm[1] + "h" + dm[2] + "min"
		}
		results = append(results, r)
	}

	if len(results) >= 2 {
		var wg sync.WaitGroup
		for i := range results {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				results[idx].PosterURL = fetchPoster(results[idx].TmdbID)
			}(i)
		}
		wg.Wait()
	}
	return results, nil
}
