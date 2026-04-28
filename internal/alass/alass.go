// Package alass intègre l'outil alass-cli (https://github.com/kaegi/alass) pour
// synchroniser un fichier SRT avec l'audio d'un mkv. Beaucoup plus fiable que
// notre cross-correlation maison : utilise du VAD + alignment local.
//
// alass-cli est un binaire statique embarqué par plateforme. Il a besoin de
// ffprobe dans le PATH (extrait par le package audiosync au même endroit).
package alass

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Locate retourne le chemin du binaire alass-cli, en l'extrayant à appBinDir
// au 1er run si embarqué. Renvoie une erreur si pas de binaire embarqué pour la
// plateforme courante.
func Locate(appBinDir string) (string, error) {
	if appBinDir == "" {
		return "", errors.New("appBinDir vide")
	}
	if len(embeddedBinary) == 0 {
		return "", errors.New("alass-cli non disponible pour cette plateforme")
	}
	if err := os.MkdirAll(appBinDir, 0755); err != nil {
		return "", err
	}
	candidate := filepath.Join(appBinDir, embeddedName)
	if _, err := os.Stat(candidate); err != nil {
		if werr := os.WriteFile(candidate, embeddedBinary, 0755); werr != nil {
			return "", werr
		}
	}
	return candidate, nil
}

// SyncResult décrit le résultat d'une synchronisation alass.
type SyncResult struct {
	OutputPath string  // chemin du SRT corrigé (= ce que mkvmerge doit utiliser)
	OffsetMs   int     // décalage moyen détecté (informatif, déjà appliqué dans OutputPath)
	FpsRatio   string  // ratio FPS détecté (ex: "25/23.976") ou ""
	NoSplit    bool    // true si --no-split a été utilisé (offset constant)
	RawOutput  string  // sortie stderr complète d'alass (logs détaillés)
}

// Sync exécute alass-cli pour synchroniser inputSRT avec referenceMKV. Écrit le
// SRT corrigé dans outputSRT. Avec noSplit=true, applique un seul offset constant
// à l'ensemble du SRT (recommandé pour décalages constants + drift FPS).
//
// PATH doit contenir ffprobe — set par l'appelant (typiquement le dossier où
// audiosync a extrait ffprobe + ffmpeg).
func Sync(ctx context.Context, alassPath, referenceMKV, inputSRT, outputSRT string, noSplit, disableFPSGuessing bool, ffmpegBinDir string) (*SyncResult, error) {
	if alassPath == "" {
		return nil, errors.New("alass-cli introuvable")
	}
	args := []string{}
	if noSplit {
		args = append(args, "--no-split")
	}
	// --disable-fps-guessing : à activer UNIQUEMENT quand source et référence ont
	// les mêmes FPS. Sinon alass invente un faux ratio FPS (testé : 25/23.976
	// inventé sur fichiers tous deux 23.976). À l'inverse, si VRAI drift FPS
	// (ex: source 24 vs ref 25), il faut LAISSER alass deviner pour qu'il
	// applique la correction (sinon offset constant +81s délirant).
	if disableFPSGuessing {
		args = append(args, "--disable-fps-guessing")
	}
	args = append(args, referenceMKV, inputSRT, outputSRT)
	cmd := exec.CommandContext(ctx, alassPath, args...)
	// PATH augmenté avec le dossier ffmpeg/ffprobe pour qu'alass les trouve.
	env := os.Environ()
	if ffmpegBinDir != "" {
		envOut := make([]string, 0, len(env))
		injected := false
		for _, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				envOut = append(envOut, "PATH="+ffmpegBinDir+string(os.PathListSeparator)+strings.TrimPrefix(e, "PATH="))
				injected = true
			} else {
				envOut = append(envOut, e)
			}
		}
		if !injected {
			envOut = append(envOut, "PATH="+ffmpegBinDir)
		}
		cmd.Env = envOut
	}
	var stderr, stdout strings.Builder
	cmd.Stderr = &stderr
	// IMPORTANT : rediriger stdout aussi (alass écrit la barre de progression
	// dessus). Quand l'app est lancée en GUI sans terminal, un stdout non
	// redirigé fait planter le child avec SIGPIPE silencieux dès que le pipe
	// se remplit (exit 1, stderr vide).
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("alass-cli : %w — %s", err, stderr.String())
	}
	// alass écrit "shifted block of … by …" et "ratio is X/Y" sur STDOUT,
	// pas sur stderr. On parse les deux pour être robuste aux versions futures.
	combined := stderr.String() + "\n" + stdout.String()
	res := &SyncResult{
		OutputPath: outputSRT,
		NoSplit:    noSplit,
		RawOutput:  combined,
	}
	// Parse "info: 'reference file FPS/input file FPS' ratio is X/Y"
	if m := regexp.MustCompile(`ratio is (\S+)`).FindStringSubmatch(combined); len(m) >= 2 {
		res.FpsRatio = m[1]
	}
	// Parse "shifted block of N subtitles with length T by ±H:MM:SS.mmm"
	// On prend le shift le plus grand en absolu (cas no-split = 1 seul block).
	res.OffsetMs = parseLargestShiftMs(combined)
	return res, nil
}

// parseLargestShiftMs cherche les lignes "shifted block of N subtitles … by H:MM:SS.mmm"
// et retourne le shift le plus grand en absolu (en ms).
func parseLargestShiftMs(stderr string) int {
	re := regexp.MustCompile(`by (-?)(\d+):(\d{2}):(\d{2})\.(\d{3})`)
	matches := re.FindAllStringSubmatch(stderr, -1)
	bestAbs := 0
	bestSigned := 0
	for _, m := range matches {
		signNeg := m[1] == "-"
		h, _ := strconv.Atoi(m[2])
		mi, _ := strconv.Atoi(m[3])
		s, _ := strconv.Atoi(m[4])
		ms, _ := strconv.Atoi(m[5])
		total := h*3600000 + mi*60000 + s*1000 + ms
		if signNeg {
			total = -total
		}
		abs := total
		if abs < 0 {
			abs = -abs
		}
		if abs > bestAbs {
			bestAbs = abs
			bestSigned = total
		}
	}
	return bestSigned
}
