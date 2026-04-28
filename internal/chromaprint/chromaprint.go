// Package chromaprint wrappe le binaire fpcalc (Chromaprint) pour détecter
// l'offset entre deux audios via leurs fingerprints spectraux.
//
// Avantage clé sur la cross-correlation RMS classique : Chromaprint hash le
// CONTENU SPECTRAL HAUT-NIVEAU (musique, ambiance, bruit) et est très robuste
// aux différences de voix. C'est le bon outil pour aligner VFQ (réf) et VFF
// (source LiHDL) qui partagent la même bande son mais ont des dialogues
// différents.
//
// Référence : https://acoustid.org/chromaprint
//   - 1 hash uint32 par fenêtre de ~0.12413 secondes (8 hash/sec)
//   - similarité par hash : popcount(NOT(A XOR B)) / 32 = bits qui matchent
package chromaprint

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Une fenêtre Chromaprint dure 4096 samples / 11025 Hz (Chromaprint resample
// l'audio en interne à 11025 Hz). En pratique : 1 hash = 0.12413 sec = ~124.13 ms.
const HashIntervalMs = 124.13

// Locate retourne le chemin de fpcalc selon la priorité :
//  1. override explicite (config)
//  2. binaire embarqué dans l'app (extrait à appBinDir au 1er run)
//  3. binaire déjà extrait à appBinDir
//  4. binaire système sur PATH (fallback dev)
func Locate(configOverride, appBinDir string) (string, error) {
	if configOverride != "" {
		if _, err := exec.LookPath(configOverride); err == nil {
			return configOverride, nil
		}
	}
	// Embedded binary : extrait à appBinDir si non présent et que embed non vide.
	if appBinDir != "" && len(embeddedBinary) > 0 {
		candidate := filepath.Join(appBinDir, embeddedName)
		if _, err := os.Stat(candidate); err != nil {
			if werr := os.WriteFile(candidate, embeddedBinary, 0755); werr == nil {
				return candidate, nil
			}
		} else {
			return candidate, nil
		}
	}
	if appBinDir != "" {
		candidate := filepath.Join(appBinDir, embeddedName)
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("fpcalc"); err == nil {
		return p, nil
	}
	return "", errors.New("fpcalc (Chromaprint) introuvable")
}

// Fingerprint extrait le fingerprint d'un fichier audio sur les `lengthSec`
// premières secondes (0 = tout le fichier). Plus long = plus précis mais plus
// lent et plus de RAM. Pour un offset détection ~1s de précision, 600s suffit
// largement.
func Fingerprint(ctx context.Context, fpcalcPath, audioPath string, lengthSec int) ([]uint32, error) {
	args := []string{"-raw"}
	if lengthSec > 0 {
		args = append(args, "-length", strconv.Itoa(lengthSec))
	}
	args = append(args, audioPath)
	cmd := exec.CommandContext(ctx, fpcalcPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fpcalc : %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024) // big buffer pour les fingerprints longs
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "FINGERPRINT=") {
			continue
		}
		raw := strings.TrimPrefix(line, "FINGERPRINT=")
		parts := strings.Split(raw, ",")
		fp := make([]uint32, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// Les hashs Chromaprint sont signés en CSV (peuvent dépasser 2^31)
			// → parse en int64 puis cast.
			n, perr := strconv.ParseInt(p, 10, 64)
			if perr != nil {
				return nil, fmt.Errorf("hash invalide %q : %w", p, perr)
			}
			fp = append(fp, uint32(n))
		}
		return fp, nil
	}
	return nil, errors.New("aucune ligne FINGRINT= dans la sortie fpcalc")
}

// FindOffset cherche le shift `k` qui maximise la similarité entre deux
// fingerprints `a` et `b`, avec b décalé de `k` fenêtres par rapport à a.
//
// Retourne :
//   - offsetMs : décalage à appliquer à `b` pour qu'il s'aligne sur `a` (en ms,
//     positif = b est en retard par rapport à a, donc retarder b davantage
//     ne corrige rien — il faut AVANCER b ; négatif = b est en avance).
//   - confidence : 0..1, 1 = match parfait sur l'overlap.
//   - bestOverlap : nombre de fenêtres comparées (overlap effectif).
//
// maxShiftWindows borne la recherche à ±k fenêtres (~0.124s × k).
// Plus c'est grand, plus c'est lent mais plus on peut détecter de gros
// décalages. 240 fenêtres = ±30 secondes.
func FindOffset(a, b []uint32, maxShiftWindows int) (offsetMs float64, confidence float64, bestOverlap int) {
	if len(a) == 0 || len(b) == 0 {
		return 0, 0, 0
	}
	if maxShiftWindows <= 0 {
		maxShiftWindows = 240 // ±~30s par défaut
	}
	// On limite le shift à min(len_a, len_b) pour avoir au moins 1 fenêtre overlap.
	maxK := maxShiftWindows
	if minLen := len(a); len(b) < minLen {
		minLen = len(b)
	} else if maxK > minLen-1 {
		maxK = minLen - 1
	}

	bestScore := 0.0
	bestK := 0
	bestN := 0
	for k := -maxK; k <= maxK; k++ {
		// Calcule l'overlap : index de a et b où ils se rencontrent
		var ai, bi, n int
		if k >= 0 {
			ai = k
			bi = 0
		} else {
			ai = 0
			bi = -k
		}
		n = len(a) - ai
		if nb := len(b) - bi; nb < n {
			n = nb
		}
		if n < 30 {
			continue // overlap trop court (~3.7s) pour être fiable
		}
		var matchBits uint64
		for i := 0; i < n; i++ {
			x := a[ai+i] ^ b[bi+i]
			matchBits += uint64(32 - bits.OnesCount32(x))
		}
		score := float64(matchBits) / float64(n*32)
		if score > bestScore {
			bestScore = score
			bestK = k
			bestN = n
		}
	}
	// Convention de retour validée empiriquement (test sur Escape from Pretoria
	// 25→24 fps) : delay positif = retarder b par rapport à a.
	// bestK > 0 signifie le contenu de b à index 0 ressemble au contenu de a à
	// index bestK : donc b est plus en avance → faut LE RETARDER → delay positif.
	offsetMs = float64(bestK) * HashIntervalMs
	confidence = bestScore
	bestOverlap = bestN
	return
}
