// Package tmdb fournit la recherche de fiches films via l'index de scrape
// serveurperso.com + fetch du poster depuis themoviedb.org. Code adapté
// depuis LiHDL Post Discord (même source de vérité).
package tmdb

import (
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
