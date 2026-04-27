// Package ocrsubs implémente le pipeline OCR PGS → SRT pour les sous-titres
// image (Blu-ray PGS) en sortie texte propre :
//
//  1. ExtractPGSTrack : mkvextract tracks → fichier .sup temporaire
//  2. RunPgsrip       : pgsrip CLI (Tesseract sous le capot) → .srt brut
//  3. CleanSRT        : nettoyage regex spécifique FR (apostrophes, espaces
//                       insécables avant ?!:;, guillemets parasites, etc.)
//
// Le binaire mkvextract est passé par l'appelant (déjà géré par mkvtool).
// tesseract et pgsrip doivent être installés sur la machine — le package les
// localise via PATH puis fallback dans ~/Library/Python/*/bin et ~/.local/bin.
package ocrsubs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go-mux-lihdl-team/internal/config"
)

// sha256OfFile calcule le SHA256 d'un fichier (utilisé comme clé du cache OCR).
func sha256OfFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// cachedSRTPath retourne le chemin "cache" prévu pour un .sup hashé.
// Le dossier est créé à la demande via config.CacheDir().
func cachedSRTPath(hash string) (string, error) {
	dir, err := config.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, hash+".srt"), nil
}

// LookupOCRCache renvoie (path, ok) si un SRT cache existe pour ce .sup.
// Best-effort : retourne (false, nil) en cas d'erreur d'IO.
func LookupOCRCache(supPath string) (string, bool) {
	hash, err := sha256OfFile(supPath)
	if err != nil {
		return "", false
	}
	p, err := cachedSRTPath(hash)
	if err != nil {
		return "", false
	}
	if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
		return p, true
	}
	return "", false
}

// StoreOCRCache copie srtPath vers le cache pour le .sup donné.
// Best-effort : ignore les erreurs (pas de raison de casser le pipeline).
func StoreOCRCache(supPath, srtPath string) error {
	hash, err := sha256OfFile(supPath)
	if err != nil {
		return err
	}
	p, err := cachedSRTPath(hash)
	if err != nil {
		return err
	}
	return copyFile(srtPath, p)
}

// ErrTesseractMissing est retournée par LocateTesseract quand le binaire
// est absent. Le frontend l'utilise pour afficher un message d'install.
var ErrTesseractMissing = errors.New("tesseract introuvable — installer via `brew install tesseract tesseract-lang`")

// ErrPgsripMissing est retournée par LocatePgsrip quand pgsrip n'est pas
// dans PATH ni dans les chemins pip user habituels.
var ErrPgsripMissing = errors.New("pgsrip introuvable — installer via `pip3 install pgsrip` (et `brew install tesseract tesseract-lang` pour le moteur OCR)")

// LocateTesseract retourne le chemin du binaire tesseract sur la machine,
// sinon ErrTesseractMissing. Cherche uniquement sur PATH (brew installe
// dans /opt/homebrew/bin sur Apple Silicon, /usr/local/bin sur Intel).
func LocateTesseract() (string, error) {
	if p, err := exec.LookPath("tesseract"); err == nil {
		return p, nil
	}
	// Fallback brew explicite (au cas où PATH est tronqué dans le contexte
	// Wails — macOS GUI n'hérite pas toujours du PATH du shell).
	for _, cand := range []string{
		"/opt/homebrew/bin/tesseract",
		"/usr/local/bin/tesseract",
	} {
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	return "", ErrTesseractMissing
}

// LocatePgsrip retourne le chemin du binaire pgsrip CLI.
// Cherche sur PATH puis dans ~/Library/Python/*/bin/ (pip user macOS) et
// ~/.local/bin/ (pip user Linux). Retourne ErrPgsripMissing si absent.
func LocatePgsrip() (string, error) {
	if p, err := exec.LookPath("pgsrip"); err == nil {
		return p, nil
	}
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", ErrPgsripMissing
	}
	// ~/.local/bin/pgsrip (pip user Linux + pipx)
	candidates := []string{filepath.Join(home, ".local", "bin", "pgsrip")}
	// ~/Library/Python/<version>/bin/pgsrip (pip user macOS) — on glob.
	pyDir := filepath.Join(home, "Library", "Python")
	if entries, err := os.ReadDir(pyDir); err == nil {
		var versions []string
		for _, e := range entries {
			if e.IsDir() {
				versions = append(versions, e.Name())
			}
		}
		// Trier décroissant pour préférer la version la plus récente.
		sort.Sort(sort.Reverse(sort.StringSlice(versions)))
		for _, v := range versions {
			candidates = append(candidates, filepath.Join(pyDir, v, "bin", "pgsrip"))
		}
	}
	for _, cand := range candidates {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	return "", ErrPgsripMissing
}

// ExtractPGSTrack extrait une piste PGS d'un MKV vers un .sup.
// outDir est le répertoire cible (créé si besoin). Le fichier est nommé
// "<base>.<trackID>.sup" pour éviter toute collision.
// Retourne le chemin du .sup généré.
func ExtractPGSTrack(ctx context.Context, mkvextractPath, mkvPath string, trackID int, outDir string) (string, error) {
	if mkvextractPath == "" {
		return "", errors.New("mkvextract introuvable (chemin vide)")
	}
	if _, err := os.Stat(mkvPath); err != nil {
		return "", fmt.Errorf("mkv source introuvable : %w", err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("création outDir : %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(mkvPath), filepath.Ext(mkvPath))
	supPath := filepath.Join(outDir, fmt.Sprintf("%s.%d.sup", base, trackID))
	// Nettoie un éventuel résidu d'un run précédent.
	_ = os.Remove(supPath)
	cmd := exec.CommandContext(ctx, mkvextractPath, "tracks", mkvPath, fmt.Sprintf("%d:%s", trackID, supPath))
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(supPath)
		return "", fmt.Errorf("mkvextract : %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if st, err := os.Stat(supPath); err != nil || st.Size() == 0 {
		_ = os.Remove(supPath)
		return "", errors.New("mkvextract : fichier .sup vide ou non créé")
	}
	return supPath, nil
}

// RunPgsrip lance pgsrip sur un .sup et retourne le chemin du .srt généré.
//
// pgsrip RENOMME automatiquement le fichier en <base>.<ietf>.sup s'il ne l'est
// pas déjà — pour garder le contrôle on copie le .sup vers <base>.<ietf>.sup
// (si nécessaire) AVANT d'invoquer pgsrip, puis on lit <base>.<ietf>.srt.
//
// `lang` doit être un code ISO 639-2 (3 lettres) type "fra", "eng", "deu" — il
// est mappé en interne vers le code IETF (2 lettres) attendu par pgsrip.
func RunPgsrip(ctx context.Context, pgsripPath, supPath, lang string) (string, error) {
	if pgsripPath == "" {
		return "", ErrPgsripMissing
	}
	if _, err := os.Stat(supPath); err != nil {
		return "", fmt.Errorf(".sup source introuvable : %w", err)
	}
	if lang == "" {
		lang = "fra"
	}
	// pgsrip parle IETF (fr, en, de, es…) ; tesseract parle ISO 639-2 (fra, eng…).
	ietf := iso6392ToIETF(lang)
	dir := filepath.Dir(supPath)
	base := strings.TrimSuffix(filepath.Base(supPath), filepath.Ext(supPath))
	// Si le fichier ne se termine pas déjà par ".<ietf>", on copie vers
	// <dir>/<base>.<ietf>.sup pour figer la convention.
	suffix := "." + ietf
	var workSup string
	if strings.HasSuffix(base, suffix) {
		workSup = supPath
	} else {
		workSup = filepath.Join(dir, base+suffix+".sup")
		if err := copyFile(supPath, workSup); err != nil {
			return "", fmt.Errorf("copie .sup vers chemin lang : %w", err)
		}
	}
	expectedSrt := strings.TrimSuffix(workSup, ".sup") + ".srt"
	// pgsrip écrase la sortie si elle existe — on nettoie au cas où.
	_ = os.Remove(expectedSrt)

	cmd := exec.CommandContext(ctx, pgsripPath, "-l", ietf, workSup)
	// pgsrip a besoin de TESSDATA_PREFIX pour trouver les langues — fallback
	// sur le chemin brew standard si non défini.
	env := os.Environ()
	if os.Getenv("TESSDATA_PREFIX") == "" {
		for _, cand := range []string{
			"/opt/homebrew/share/tessdata/",
			"/usr/local/share/tessdata/",
			"/usr/share/tesseract-ocr/4.00/tessdata/",
		} {
			if _, err := os.Stat(cand); err == nil {
				env = append(env, "TESSDATA_PREFIX="+cand)
				break
			}
		}
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pgsrip : %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if st, err := os.Stat(expectedSrt); err != nil || st.Size() == 0 {
		return "", fmt.Errorf("pgsrip : .srt non généré (%s)", expectedSrt)
	}
	return expectedSrt, nil
}

// iso6392ToIETF convertit un code 3-lettres (fra, eng, deu…) vers son code
// 2-lettres (fr, en, de…) — utilisé par pgsrip qui attend de l'IETF.
func iso6392ToIETF(s string) string {
	switch strings.ToLower(s) {
	case "fra", "fre", "fr":
		return "fr"
	case "eng", "en":
		return "en"
	case "deu", "ger", "de":
		return "de"
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
		return "ja"
	case "chi", "zho", "zh":
		return "zh"
	}
	if len(s) == 2 {
		return strings.ToLower(s)
	}
	return strings.ToLower(s)[:2]
}

// copyFile copie src vers dst en streaming.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// --- Cleanup regex (port Go du POC Python /tmp/clean_srt.py) ---

var (
	reLeadColon      = regexp.MustCompile(`^\s*[:;]\s+`)
	reParasiteQuote1 = regexp.MustCompile(`(\w)"([?!.,])`)
	reParasiteQuote2 = regexp.MustCompile(`(\w)"(\s|$)`)
	reAposSpaces     = regexp.MustCompile(`([cdjlmnstCDJLMNST])'\s+(\w)`)
	reStraightApos   = regexp.MustCompile(`(\w)'(\w)`)
	reMultiSpace     = regexp.MustCompile(` {2,}`)
	rePunctSpace     = regexp.MustCompile(`\s+([?!:;])`)
	reCommaDot       = regexp.MustCompile(`\s+([,.])`)
	reBlockNumber    = regexp.MustCompile(`^\d+$`)
	reDoubleI        = regexp.MustCompile(`\bII\b`)
)

// nameFixes : noms propres / cassings connus à imposer après OCR.
// Extensible — ajouter ici les patterns qui reviennent sur tes releases.
var nameFixes = map[string]string{
	"Charli xex":    "Charli XCX",
	"Charli Xex":    "Charli XCX",
	"amazon music":  "AMAZON MUSIC",
	"Amazon music":  "Amazon Music",
}

// cleanLine applique toutes les corrections sur une ligne de dialogue.
// Les lignes timecodes (--> dedans) et numéros de bloc (\d+ seuls) doivent
// être passées TELLES QUELLES par l'appelant (CleanSRT).
func cleanLine(line string) string {
	s := line
	// 1. ":" parasite début ligne (": Les gens" → "Les gens")
	s = reLeadColon.ReplaceAllString(s, "")
	// 2. Guillemets parasites collés à un mot (lui"? → lui?)
	s = reParasiteQuote1.ReplaceAllString(s, "$1$2")
	s = reParasiteQuote2.ReplaceAllString(s, "$1$2")
	// 3. Espace parasite après apostrophe (j' aime → j'aime)
	s = reAposSpaces.ReplaceAllString(s, "$1'$2")
	// 4. Apostrophe droite → courbe (cohérence typo française)
	s = reStraightApos.ReplaceAllString(s, "$1’$2")
	// 5. Doubles espaces
	s = reMultiSpace.ReplaceAllString(s, " ")
	// 6. Espace insécable avant ?!:; (règle FR)
	s = rePunctSpace.ReplaceAllString(s, " $1")
	// 7. Pas d'espace avant , ou .
	s = reCommaDot.ReplaceAllString(s, "$1")
	// 8. Noms propres connus (case-sensitive)
	for bad, good := range nameFixes {
		s = strings.ReplaceAll(s, bad, good)
	}
	// 8.bis Dictionnaire custom enrichissable (~/Library/.../ocr-custom-dict.json).
	// Best-effort : si erreur de lecture, on continue avec nameFixes seuls.
	if custom, err := LoadCustomDict(); err == nil {
		for bad, good := range custom {
			s = strings.ReplaceAll(s, bad, good)
		}
	}
	// 9. "II" isolé → "Il" (OCR confond les minuscules I et les majuscules)
	s = reDoubleI.ReplaceAllString(s, "Il")
	return s
}

// CleanStats : statistiques retournées par CleanSRT, exposables à l'UI pour
// afficher un score de qualité estimé.
type CleanStats struct {
	TotalLines      int     `json:"total_lines"`      // total de lignes texte (hors timecodes/numéros)
	CorrectedLines  int     `json:"corrected_lines"`  // lignes modifiées par le cleanup regex
	SuspiciousLines int     `json:"suspicious_lines"` // lignes contenant encore des patterns suspects
	QualityScore    float64 `json:"quality_score"`    // score 0-100 (% estimé de lignes propres)
	Subtitles       int     `json:"subtitles"`        // nombre de blocs SRT
}

// reSuspicious : patterns d'erreurs résiduelles probables après cleanup.
// Caractère isolé entre lettres (OCR confondu) | chiffres au milieu d'un mot |
// double caractères bizarres (||, ¨¨…).
var reSuspicious = regexp.MustCompile(`[a-zàâäéèêëîïôöùûü][|\\][a-zàâäéèêëîïôöùûü]|[a-zA-Z][0-9]+[a-zA-Z]|\|\||¨¨`)

// CleanSRT applique le cleanup regex sur un SRT, en place (overwrite).
// Préserve les numéros de blocs et les lignes timecodes telles quelles.
// Retourne des stats utiles pour calculer un score de qualité.
func CleanSRT(srtPath string) (CleanStats, error) {
	var stats CleanStats
	raw, err := os.ReadFile(srtPath)
	if err != nil {
		return stats, err
	}
	// Normalise les fins de ligne en \n pour le traitement.
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out[i] = line
			continue
		}
		if reBlockNumber.MatchString(trimmed) {
			out[i] = line
			stats.Subtitles++
			continue
		}
		if strings.Contains(line, "-->") {
			out[i] = line
			continue
		}
		stats.TotalLines++
		cleaned := cleanLine(line)
		out[i] = cleaned
		if cleaned != line {
			stats.CorrectedLines++
		}
		if reSuspicious.MatchString(cleaned) {
			stats.SuspiciousLines++
		}
	}
	final := strings.Join(out, "\n")
	if !strings.HasSuffix(final, "\n") {
		final += "\n"
	}
	if err := os.WriteFile(srtPath, []byte(final), 0644); err != nil {
		return stats, err
	}
	if stats.TotalLines > 0 {
		stats.QualityScore = 100.0 * float64(stats.TotalLines-stats.SuspiciousLines) / float64(stats.TotalLines)
	} else {
		stats.QualityScore = 100
	}
	return stats, nil
}

// ProgressFn est appelée à chaque étape du pipeline pour notifier le frontend.
// status ∈ {"extract", "ocr", "clean", "done"}, percent ∈ [0..100].
type ProgressFn func(status string, percent int, message string)

// LangToolOpts : options pour activer LanguageTool dans le pipeline.
// Si Enabled=false, l'étape est skippée (compat ascendante stricte).
type LangToolOpts struct {
	Enabled bool   // true = appeler LanguageToolFix après CleanSRT
	APIURL  string // "" → API publique gratuite
	APIKey  string // "" → mode public
	APIUser string // ignoré si APIKey == ""
}

// ConvertPGSTrackToSRT exécute le pipeline complet :
//  1. ExtractPGSTrack (.sup temporaire dans tmpDir)
//  2. RunPgsrip       (.srt brut à côté du .sup)
//  3. CleanSRT        (overwrite in-place)
//  4. (optionnel) LanguageToolFix
//  5. Move final .srt vers finalDir avec un nom propre.
//
// Retourne le chemin du .srt final + stats cleanup + stats LT.
func ConvertPGSTrackToSRT(
	ctx context.Context,
	mkvextractPath, pgsripPath, mkvPath string,
	trackID int,
	lang string,
	finalDir string,
	ltOpts LangToolOpts,
	progress ProgressFn,
) (string, CleanStats, LangToolStats, error) {
	var stats CleanStats
	var ltStats LangToolStats
	if progress == nil {
		progress = func(string, int, string) {}
	}
	// Validation chemins binaires en amont — message clair pour le user.
	if mkvextractPath == "" {
		return "", stats, ltStats, errors.New("mkvextract introuvable")
	}
	if pgsripPath == "" {
		return "", stats, ltStats, ErrPgsripMissing
	}
	// On vérifie aussi tesseract pour échouer proprement avant d'extraire.
	if _, err := LocateTesseract(); err != nil {
		return "", stats, ltStats, err
	}

	if lang == "" {
		lang = "fra"
	}
	if finalDir == "" {
		finalDir = filepath.Dir(mkvPath)
	}

	// Dossier de travail temporaire unique (évite collisions, nettoyé en fin).
	tmpDir, err := os.MkdirTemp("", "ocrsubs-*")
	if err != nil {
		return "", stats, ltStats, fmt.Errorf("création dossier temp : %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. Extract PGS
	progress("extract", 10, "Extraction de la piste PGS via mkvextract…")
	supPath, err := ExtractPGSTrack(ctx, mkvextractPath, mkvPath, trackID, tmpDir)
	if err != nil {
		return "", stats, ltStats, fmt.Errorf("extract PGS : %w", err)
	}
	progress("extract", 30, "Piste PGS extraite ("+filepath.Base(supPath)+")")

	// 1.bis Lookup cache OCR par sha256(.sup) — gain de minutes si déjà OCRisé.
	if cachedPath, ok := LookupOCRCache(supPath); ok {
		progress("ocr", 80, "🚀 Cache OCR : SRT déjà calculé pour ce .sup — skip pgsrip+clean")
		// On copie vers tmpDir/srtPath pour rentrer dans le flow existant
		// (clean est skippé car la version cache a déjà passé regex+LT).
		srtPath := filepath.Join(tmpDir, strings.TrimSuffix(filepath.Base(supPath), ".sup")+".srt")
		if err := copyFile(cachedPath, srtPath); err == nil {
			// 5. Move vers finalDir avec nom propre (pas de stats/clean —
			// la version cache est déjà nettoyée).
			if mkErr := os.MkdirAll(finalDir, 0755); mkErr != nil {
				return "", stats, ltStats, fmt.Errorf("création finalDir : %w", mkErr)
			}
			mkvBase := strings.TrimSuffix(filepath.Base(mkvPath), filepath.Ext(mkvPath))
			finalName := fmt.Sprintf("%s.%s.ocr.srt", mkvBase, lang)
			finalPath := filepath.Join(finalDir, finalName)
			if _, err := os.Stat(finalPath); err == nil {
				for n := 2; n < 100; n++ {
					cand := filepath.Join(finalDir, fmt.Sprintf("%s.%s.ocr.%d.srt", mkvBase, lang, n))
					if _, err := os.Stat(cand); os.IsNotExist(err) {
						finalPath = cand
						break
					}
				}
			}
			if err := copyFile(srtPath, finalPath); err != nil {
				return "", stats, ltStats, fmt.Errorf("move SRT cache final : %w", err)
			}
			// Compte les sous-titres dans le SRT pour les stats minimales.
			if data, err := os.ReadFile(finalPath); err == nil {
				stats.Subtitles = strings.Count(string(data), "-->")
				stats.QualityScore = 100
			}
			progress("done", 100, "SRT final (cache) : "+finalPath)
			return finalPath, stats, ltStats, nil
		}
	}

	// 2. OCR via pgsrip
	progress("ocr", 40, "OCR Tesseract en cours (peut prendre plusieurs minutes)…")
	srtPath, err := RunPgsrip(ctx, pgsripPath, supPath, lang)
	if err != nil {
		return "", stats, ltStats, fmt.Errorf("pgsrip OCR : %w", err)
	}
	progress("ocr", 80, "OCR terminé")

	// 3. Cleanup regex
	progress("clean", 85, "Nettoyage regex (apostrophes, espaces FR, etc.)…")
	stats, err = CleanSRT(srtPath)
	if err != nil {
		return "", stats, ltStats, fmt.Errorf("cleanup SRT : %w", err)
	}
	progress("clean", 95, "Nettoyage terminé")

	// 4. LanguageTool (optionnel) — best-effort, n'échoue jamais le pipeline.
	if ltOpts.Enabled {
		progress("languagetool", 96, "Vérification LanguageTool…")
		ls, ltErr := LanguageToolFix(ctx, srtPath, lang, ltOpts.APIURL, ltOpts.APIKey, ltOpts.APIUser, progress)
		if ltErr != nil {
			progress("languagetool", 99, "⚠ LanguageTool : "+ltErr.Error())
		}
		ltStats = ls
		// Le score qualité reste basé sur les patterns suspects regex (vraies
		// erreurs OCR détectables). Les NeedsReview LT incluent beaucoup de
		// faux positifs (style, grammaire idiomatique, abréviations…) → ils
		// sont affichés à part mais ne pénalisent pas le score.
	}

	// 5. Move vers finalDir avec nom propre.
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return "", stats, ltStats, fmt.Errorf("création finalDir : %w", err)
	}
	mkvBase := strings.TrimSuffix(filepath.Base(mkvPath), filepath.Ext(mkvPath))
	finalName := fmt.Sprintf("%s.%s.ocr.srt", mkvBase, lang)
	finalPath := filepath.Join(finalDir, finalName)
	// Si destination existe déjà, ajoute un suffixe numérique.
	if _, err := os.Stat(finalPath); err == nil {
		for n := 2; n < 100; n++ {
			cand := filepath.Join(finalDir, fmt.Sprintf("%s.%s.ocr.%d.srt", mkvBase, lang, n))
			if _, err := os.Stat(cand); os.IsNotExist(err) {
				finalPath = cand
				break
			}
		}
	}
	// os.Rename peut échouer entre devices différents (tmpDir vs finalDir) →
	// fallback copy + delete.
	if err := os.Rename(srtPath, finalPath); err != nil {
		if cerr := copyFile(srtPath, finalPath); cerr != nil {
			return "", stats, ltStats, fmt.Errorf("move SRT final : %w", cerr)
		}
		_ = os.Remove(srtPath)
	}
	// Best-effort : sauvegarde dans le cache OCR pour les prochains usages.
	_ = StoreOCRCache(supPath, finalPath)
	progress("done", 100, "SRT final : "+finalPath)
	return finalPath, stats, ltStats, nil
}
