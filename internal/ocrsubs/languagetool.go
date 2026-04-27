// Package ocrsubs — étape LanguageTool
//
// Ce fichier ajoute une étape post-cleanup au pipeline OCR : on envoie le SRT
// (lignes texte uniquement) à l'API publique LanguageTool, on applique en
// place les corrections "safes" (TYPOS courts, PUNCTUATION, CASING sur noms
// propres connus) et on retourne la liste des matches non-auto-fixés pour
// validation humaine côté UI.
//
// API publique LanguageTool :
//   - https://api.languagetool.org/v2/check (form-encoded POST)
//   - 20 req/min sans authentification, 20 KB par req
//   - Premium : https://api.languagetoolplus.com/v2/check + username + apiKey
//
// Pour rester sous la limite, on chunke par ~50 sous-titres (~3-5 KB par req)
// et on insère 3 secondes entre requêtes.
package ocrsubs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// LangToolStats résume le passage LanguageTool : combien de matches trouvés,
// combien auto-fixés, combien à vérifier humainement (avec le détail).
type LangToolStats struct {
	TotalIssues     int           `json:"total_issues"`
	AutoFixed       int           `json:"auto_fixed"`
	NeedsReview     int           `json:"needs_review"`
	NeedsReviewList []ReviewMatch `json:"needs_review_list"`
}

// ReviewMatch : un match LanguageTool non-auto-fixé, à montrer à l'humain.
type ReviewMatch struct {
	LineNumber  int      `json:"line_number"` // ligne 1-indexée dans le SRT final
	Snippet     string   `json:"snippet"`     // ~30 chars autour de l'erreur
	Message     string   `json:"message"`     // explication LT
	Suggestions []string `json:"suggestions"` // top 3 propositions
}

// ltMatch : structure JSON renvoyée par /v2/check.
type ltMatch struct {
	Message      string `json:"message"`
	ShortMessage string `json:"shortMessage"`
	Offset       int    `json:"offset"`
	Length       int    `json:"length"`
	Replacements []struct {
		Value string `json:"value"`
	} `json:"replacements"`
	Rule struct {
		ID       string `json:"id"`
		Category struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"category"`
	} `json:"rule"`
	Sentence string `json:"sentence"`
	Context  struct {
		Text   string `json:"text"`
		Offset int    `json:"offset"`
		Length int    `json:"length"`
	} `json:"context"`
}

type ltResponse struct {
	Matches []ltMatch `json:"matches"`
}

// chunkBudget : taille cible d'un chunk envoyé à LT (caractères). 5 KB ≈ marge
// confortable sous la limite 20 KB de l'API publique gratuite.
const chunkBudget = 5000

// ltRequestDelay : délai entre 2 requêtes pour rester sous 20 req/min.
const ltRequestDelay = 3 * time.Second

// ltHTTPTimeout : timeout pour une requête /v2/check.
const ltHTTPTimeout = 30 * time.Second

// ltUserAgent : header User-Agent envoyé à LT.
const ltUserAgent = "GoMuxLiHDL/5.x"

// langToolLang mappe ISO 639-2 (fra/eng/deu…) vers le code attendu par
// LanguageTool (fr / en-US / de-DE / es / it / pt / nl / ru / ja / zh).
func langToolLang(iso6392 string) string {
	switch strings.ToLower(iso6392) {
	case "fra", "fre", "fr":
		return "fr"
	case "eng", "en":
		return "en-US"
	case "deu", "ger", "de":
		return "de-DE"
	case "spa", "es":
		return "es"
	case "ita", "it":
		return "it"
	case "por", "pt":
		return "pt"
	case "nld", "dut", "nl":
		return "nl"
	case "rus", "ru":
		return "ru"
	case "jpn", "ja":
		return "ja-JP"
	case "chi", "zho", "zh":
		return "zh-CN"
	}
	if len(iso6392) == 2 {
		return strings.ToLower(iso6392)
	}
	return "fr"
}

// editDistance calcule la distance de Levenshtein entre deux chaînes.
// Utilisée pour décider si une suggestion TYPO est "safe" (≤ 2).
func editDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	la, lb := len(ar), len(br)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			d := prev[j] + 1
			if curr[j-1]+1 < d {
				d = curr[j-1] + 1
			}
			if prev[j-1]+cost < d {
				d = prev[j-1] + cost
			}
			curr[j] = d
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// knownProperNouns : casing à imposer sur des noms propres déjà repérés. La
// même map que `nameFixes` sauf qu'on l'utilise ici pour valider qu'un fix LT
// CASING tombe bien sur un nom déjà connu — on ne veut PAS écraser une
// majuscule artistique avec la suggestion LT au pif.
var knownProperNouns = map[string]bool{
	"Charli XCX":   true,
	"AMAZON MUSIC": true,
	"Amazon Music": true,
}

// shouldAutoFix décide si un match LT peut être appliqué sans validation.
//
// Stratégie :
//   - 1 seule suggestion ET edit distance ≤ 2 ET catégorie TYPOS  → auto
//   - catégorie PUNCTUATION (toutes règles) → auto
//   - catégorie CASING uniquement si la suggestion ∈ knownProperNouns → auto
//   - sinon → false (à mettre dans NeedsReviewList)
func shouldAutoFix(m ltMatch, original string) bool {
	if len(m.Replacements) == 0 {
		return false
	}
	cat := strings.ToUpper(m.Rule.Category.ID)
	first := m.Replacements[0].Value
	switch cat {
	case "PUNCTUATION":
		return true
	case "TYPOS":
		if len(m.Replacements) == 1 {
			if editDistance(strings.ToLower(original), strings.ToLower(first)) <= 2 {
				return true
			}
		}
	case "CASING":
		if knownProperNouns[first] {
			return true
		}
	}
	return false
}

// extractTextLines découpe un SRT en lignes-texte avec leur position globale.
// On renvoie les indices (1-indexés) des lignes de dialogue dans le fichier
// d'origine et le bloc texte concaténé envoyé à LT.
//
// Ce paquet est passé à LT en chunks ; pour reconstituer la position globale
// d'un match (offset/length dans le chunk), on garde un mapping (chunkOffset,
// origLineIdx, origLineStart). Voir buildChunks.
type srtLine struct {
	idx  int    // index 0-based dans le tableau lines (≠ ligne SRT humaine)
	text string // contenu de la ligne (sans \n)
}

// isMeta retourne true si la ligne doit être ignorée par LT (timecode, numéro
// de bloc, ligne vide).
func isMeta(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return true
	}
	if strings.Contains(line, "-->") {
		return true
	}
	if reBlockNumber.MatchString(t) {
		return true
	}
	return false
}

// chunk : un paquet envoyé à LT en une seule requête.
type chunk struct {
	text       string    // contenu agrégé (lignes séparées par \n)
	lineStarts []int     // offset (en chars) de chaque ligne dans `text`
	lines      []srtLine // mapping vers les lignes d'origine
}

// buildChunks découpe la liste des lignes-texte en paquets ≤ chunkBudget.
func buildChunks(lines []srtLine) []chunk {
	var out []chunk
	var cur chunk
	for _, l := range lines {
		piece := l.text
		// +1 pour le séparateur "\n" entre 2 lignes du chunk.
		extra := len(piece)
		if len(cur.lines) > 0 {
			extra++
		}
		if len(cur.lines) > 0 && len(cur.text)+extra > chunkBudget {
			out = append(out, cur)
			cur = chunk{}
		}
		if len(cur.lines) > 0 {
			cur.text += "\n"
		}
		cur.lineStarts = append(cur.lineStarts, len(cur.text))
		cur.text += piece
		cur.lines = append(cur.lines, l)
	}
	if len(cur.lines) > 0 {
		out = append(out, cur)
	}
	return out
}

// findLineForOffset retrouve quelle ligne du chunk contient un offset donné.
// Retourne (indexInChunk, offsetInLine).
func findLineForOffset(c chunk, offset int) (int, int) {
	// Cherche le plus grand lineStarts[i] ≤ offset.
	idx := 0
	for i, st := range c.lineStarts {
		if st <= offset {
			idx = i
		} else {
			break
		}
	}
	return idx, offset - c.lineStarts[idx]
}

// callLanguageTool envoie un chunk à l'API LT et retourne les matches.
func callLanguageTool(ctx context.Context, apiURL, apiKey, apiUser, lang, text string) ([]ltMatch, error) {
	form := url.Values{}
	form.Set("text", text)
	form.Set("language", lang)
	form.Set("enabledOnly", "false")
	if apiKey != "" {
		form.Set("apiKey", apiKey)
		if apiUser != "" {
			form.Set("username", apiUser)
		}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ltUserAgent)
	client := &http.Client{Timeout: ltHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("HTTP %d : %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed ltResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse JSON LT : %w", err)
	}
	return parsed.Matches, nil
}

// snippetAround extrait ~radius chars autour de l'erreur dans la ligne.
func snippetAround(line string, off, length, radius int) string {
	if off < 0 {
		off = 0
	}
	if off > len(line) {
		off = len(line)
	}
	end := off + length
	if end > len(line) {
		end = len(line)
	}
	start := off - radius
	if start < 0 {
		start = 0
	}
	stop := end + radius
	if stop > len(line) {
		stop = len(line)
	}
	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "…"
	}
	if stop < len(line) {
		suffix = "…"
	}
	return prefix + line[start:stop] + suffix
}

// LanguageToolFix interroge l'API publique LanguageTool, applique les
// corrections "safes" en place dans le SRT, et retourne les stats + la liste
// des matches à valider humainement.
//
// `apiKey` peut être "" pour l'API publique gratuite (20 req/min, 20KB/req).
// `apiURL` peut être "" → défaut "https://api.languagetool.org/v2/check".
// `apiUser` est ignoré si `apiKey` est vide (mode public).
//
// En cas d'erreur réseau (timeout, 429, 500…) sur un chunk, on log mais on
// continue avec les autres chunks — on ne fait JAMAIS échouer tout le pipeline
// OCR juste parce que LT est down.
func LanguageToolFix(ctx context.Context, srtPath, lang, apiURL, apiKey, apiUser string, progress ProgressFn) (LangToolStats, error) {
	var stats LangToolStats
	if progress == nil {
		progress = func(string, int, string) {}
	}
	if apiURL == "" {
		apiURL = "https://api.languagetool.org/v2/check"
	}
	ltLang := langToolLang(lang)

	raw, err := os.ReadFile(srtPath)
	if err != nil {
		return stats, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	// Compte les lignes humaines (1-indexées) en sautant les lignes meta
	// pour produire le rapport. On garde le mapping idx 0-based → ligne SRT
	// humaine = idx+1 (l'utilisateur peut aller à cette ligne dans son éditeur).
	var dialogue []srtLine
	for i, l := range lines {
		if isMeta(l) {
			continue
		}
		dialogue = append(dialogue, srtLine{idx: i, text: l})
	}
	if len(dialogue) == 0 {
		return stats, nil
	}

	chunks := buildChunks(dialogue)
	totalChunks := len(chunks)

	// Patches appliqués en fin (par ligne d'origine), du dernier offset au
	// premier pour ne pas décaler les positions.
	type patch struct {
		offset      int
		length      int
		replacement string
	}
	patchesByLineIdx := map[int][]patch{}

	for ci, c := range chunks {
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		default:
		}
		if ci > 0 {
			// rate-limit : 3 sec entre 2 requêtes (≈ 20 req/min - marge)
			select {
			case <-ctx.Done():
				return stats, ctx.Err()
			case <-time.After(ltRequestDelay):
			}
		}
		pct := 96 + int(float64(ci+1)/float64(totalChunks)*3.0) // 96 → 99
		if pct > 99 {
			pct = 99
		}
		progress("languagetool", pct, fmt.Sprintf("LanguageTool chunk %d/%d…", ci+1, totalChunks))

		matches, err := callLanguageTool(ctx, apiURL, apiKey, apiUser, ltLang, c.text)
		if err != nil {
			// Best-effort : on log via progress et on continue avec les autres
			// chunks. Ne jamais faire échouer tout l'OCR à cause de LT.
			progress("languagetool", pct, fmt.Sprintf("⚠ LT chunk %d/%d : %s", ci+1, totalChunks, err.Error()))
			continue
		}

		for _, m := range matches {
			stats.TotalIssues++
			lineIdx, offInLine := findLineForOffset(c, m.Offset)
			if lineIdx >= len(c.lines) {
				continue
			}
			lineRef := c.lines[lineIdx]
			origLine := lineRef.text
			// Sécurité : si le match déborde sur la ligne suivante, on skip.
			if offInLine < 0 || offInLine+m.Length > len(origLine) {
				stats.NeedsReview++
				if len(stats.NeedsReviewList) < 50 {
					var sugg []string
					for i, r := range m.Replacements {
						if i >= 3 {
							break
						}
						sugg = append(sugg, r.Value)
					}
					stats.NeedsReviewList = append(stats.NeedsReviewList, ReviewMatch{
						LineNumber:  lineRef.idx + 1,
						Snippet:     snippetAround(origLine, offInLine, m.Length, 30),
						Message:     m.Message,
						Suggestions: sugg,
					})
				}
				continue
			}
			original := origLine[offInLine : offInLine+m.Length]
			if shouldAutoFix(m, original) {
				patchesByLineIdx[lineRef.idx] = append(patchesByLineIdx[lineRef.idx], patch{
					offset:      offInLine,
					length:      m.Length,
					replacement: m.Replacements[0].Value,
				})
				stats.AutoFixed++
			} else {
				stats.NeedsReview++
				if len(stats.NeedsReviewList) < 50 {
					var sugg []string
					for i, r := range m.Replacements {
						if i >= 3 {
							break
						}
						sugg = append(sugg, r.Value)
					}
					stats.NeedsReviewList = append(stats.NeedsReviewList, ReviewMatch{
						LineNumber:  lineRef.idx + 1,
						Snippet:     snippetAround(origLine, offInLine, m.Length, 30),
						Message:     m.Message,
						Suggestions: sugg,
					})
				}
			}
		}
	}

	// Applique les patches : du dernier offset au premier pour préserver les
	// positions de la même ligne.
	if len(patchesByLineIdx) > 0 {
		for idx, ps := range patchesByLineIdx {
			sort.Slice(ps, func(i, j int) bool { return ps[i].offset > ps[j].offset })
			line := lines[idx]
			for _, p := range ps {
				if p.offset+p.length > len(line) {
					continue
				}
				line = line[:p.offset] + p.replacement + line[p.offset+p.length:]
			}
			lines[idx] = line
		}
		final := strings.Join(lines, "\n")
		if !strings.HasSuffix(final, "\n") {
			final += "\n"
		}
		if err := os.WriteFile(srtPath, []byte(final), 0644); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// TestLanguageToolKey teste une clé Premium LanguageTool en envoyant une
// requête minuscule. Retourne (ok, message).
func TestLanguageToolKey(apiURL, apiKey, apiUser string) (bool, string) {
	if apiURL == "" {
		// Quand on teste avec apiKey, l'endpoint Premium est laguagetoolplus.
		if apiKey != "" {
			apiURL = "https://api.languagetoolplus.com/v2/check"
		} else {
			apiURL = "https://api.languagetool.org/v2/check"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := callLanguageTool(ctx, apiURL, apiKey, apiUser, "fr", "Bonjour le monde.")
	if err != nil {
		return false, err.Error()
	}
	return true, "API LanguageTool OK"
}
