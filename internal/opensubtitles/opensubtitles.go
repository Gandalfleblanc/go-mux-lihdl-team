// Package opensubtitles : client minimal pour l'API REST v1 d'OpenSubtitles.
//
// Endpoints utilisés :
//
//   GET https://api.opensubtitles.com/api/v1/subtitles?query=<title>&year=<yyyy>&languages=<fr,en>&type=movie
//        → liste de résultats (cf. OSSearchResult)
//   POST https://api.opensubtitles.com/api/v1/download
//        → renvoie un lien temporaire pour télécharger le SRT (24h validité)
//
// Headers obligatoires :
//   Api-Key:        <clé personnelle, créée sur opensubtitles.com → Profile → Consumers>
//   User-Agent:     <App User-Agent enregistré côté OpenSubtitles>
//   Content-Type:   application/json
//   Accept:         application/json
//
// Le User-Agent doit être enregistré gratuitement sur opensubtitles.com pour
// que l'API accepte les requêtes (sinon 403). On hardcode "GoMuxLiHDL v5.x"
// par défaut et l'utilisateur peut l'override en cas de besoin.
package opensubtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultUserAgent : User-Agent par défaut. À remplacer par celui enregistré
// sur opensubtitles.com (Profile → Consumers).
const DefaultUserAgent = "GoMuxLiHDL v5.x"

// BaseURL : endpoint REST API v1 OpenSubtitles.
const BaseURL = "https://api.opensubtitles.com/api/v1"

// httpTimeout : délai max pour une requête API.
const httpTimeout = 30 * time.Second

// OSSearchResult : un résultat de recherche normalisé pour l'UI.
type OSSearchResult struct {
	ID            string  `json:"id"`              // file_id pour download
	SubtitleID    string  `json:"subtitle_id"`     // ID du sous-titre (informatif)
	Title         string  `json:"title"`
	Year          int     `json:"year"`
	Language      string  `json:"language"`        // fr, en…
	DownloadCount int     `json:"download_count"`
	Rating        float64 `json:"rating"`
	Filename      string  `json:"filename"`
	URL           string  `json:"url"`             // lien web pour preview UI
}

// rawSubtitlesResponse : structure JSON brute renvoyée par /subtitles.
type rawSubtitlesResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			SubtitleID    string  `json:"subtitle_id"`
			Language      string  `json:"language"`
			DownloadCount int     `json:"download_count"`
			Ratings       float64 `json:"ratings"`
			URL           string  `json:"url"`
			FeatureDetails struct {
				Title     string `json:"title"`
				MovieName string `json:"movie_name"`
				Year      int    `json:"year"`
			} `json:"feature_details"`
			Files []struct {
				FileID   int    `json:"file_id"`
				FileName string `json:"file_name"`
			} `json:"files"`
		} `json:"attributes"`
	} `json:"data"`
}

// rawDownloadResponse : structure JSON renvoyée par /download.
type rawDownloadResponse struct {
	Link    string `json:"link"`
	Message string `json:"message"`
	// Reset/Remaining sont informatifs (quota journalier).
}

// resolveUA retourne userAgent si non vide, sinon DefaultUserAgent.
func resolveUA(userAgent string) string {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return DefaultUserAgent
	}
	return ua
}

// Search interroge GET /subtitles. `apiKey` est obligatoire (renvoie une erreur
// si vide). `lang` est un code IETF ("fr", "en", "de"…). Si vide → "fr,en".
//
// Retourne max 50 résultats (premier page de l'API).
func Search(ctx context.Context, apiKey, userAgent, query string, year int, lang string) ([]OSSearchResult, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("OpenSubtitles : clé API manquante (Settings → OpenSubtitles)")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("OpenSubtitles : titre vide")
	}
	if lang == "" {
		lang = "fr,en"
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("languages", lang)
	q.Set("type", "movie")
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	endpoint := BaseURL + "/subtitles?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Api-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", resolveUA(userAgent))

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenSubtitles : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("OpenSubtitles : HTTP %d : %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var raw rawSubtitlesResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("OpenSubtitles : parse JSON : %w", err)
	}
	out := make([]OSSearchResult, 0, len(raw.Data))
	for _, d := range raw.Data {
		title := d.Attributes.FeatureDetails.Title
		if title == "" {
			title = d.Attributes.FeatureDetails.MovieName
		}
		var fileID, filename string
		if len(d.Attributes.Files) > 0 {
			fileID = strconv.Itoa(d.Attributes.Files[0].FileID)
			filename = d.Attributes.Files[0].FileName
		}
		// Si pas de fileID, on skip (pas downloadable).
		if fileID == "" {
			continue
		}
		out = append(out, OSSearchResult{
			ID:            fileID,
			SubtitleID:    d.Attributes.SubtitleID,
			Title:         title,
			Year:          d.Attributes.FeatureDetails.Year,
			Language:      d.Attributes.Language,
			DownloadCount: d.Attributes.DownloadCount,
			Rating:        d.Attributes.Ratings,
			Filename:      filename,
			URL:           d.Attributes.URL,
		})
	}
	return out, nil
}

// Download télécharge le SRT correspondant au fileID vers dstPath.
//
// Le flow OS API v1 :
//  1. POST /download {file_id} → renvoie un lien temporaire
//  2. GET <lien> → contenu SRT brut
//
// dstPath sera créé / écrasé. Le dossier parent doit exister.
func Download(ctx context.Context, apiKey, userAgent, fileID, dstPath string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("OpenSubtitles : clé API manquante")
	}
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return errors.New("OpenSubtitles : fileID vide")
	}

	// Étape 1 : récupère le lien de download.
	body := strings.NewReader(fmt.Sprintf(`{"file_id":%s}`, fileID))
	req, err := http.NewRequestWithContext(ctx, "POST", BaseURL+"/download", body)
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", resolveUA(userAgent))

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("OpenSubtitles download init : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("OpenSubtitles download init : HTTP %d : %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var dl rawDownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&dl); err != nil {
		return fmt.Errorf("OpenSubtitles parse download JSON : %w", err)
	}
	if dl.Link == "" {
		return fmt.Errorf("OpenSubtitles : lien de téléchargement vide (%s)", dl.Message)
	}

	// Étape 2 : GET sur le lien temporaire — pas besoin des headers API ici.
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	getReq, err := http.NewRequestWithContext(ctx, "GET", dl.Link, nil)
	if err != nil {
		return err
	}
	getReq.Header.Set("User-Agent", resolveUA(userAgent))
	getResp, err := client.Do(getReq)
	if err != nil {
		return fmt.Errorf("OpenSubtitles fetch SRT : %w", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != 200 {
		raw, _ := io.ReadAll(io.LimitReader(getResp.Body, 2048))
		return fmt.Errorf("OpenSubtitles fetch SRT : HTTP %d : %s", getResp.StatusCode, strings.TrimSpace(string(raw)))
	}
	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, getResp.Body); err != nil {
		return fmt.Errorf("OpenSubtitles écriture .srt : %w", err)
	}
	return nil
}
