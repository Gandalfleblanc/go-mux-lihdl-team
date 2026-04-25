// Package audiosync détecte et corrige le décalage temporel entre 2 pistes
// audio d'un .mkv (ex : VFF + VFQ d'un même film). Ne réencode jamais le son :
//   - Détection : extraction PCM mono 8 kHz via ffmpeg → enveloppe RMS @ 100 Hz
//                 → cross-correlation pour trouver le lag.
//   - Recalage : appliqué via mkvmerge --sync TID:OFFSET (timecodes uniquement,
//                copie bit-à-bit du flux compressé AC3/EAC3).
package audiosync

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// Locate retourne le chemin du binaire ffmpeg selon la priorité :
//  1. binaire embarqué dans l'app (extrait à appBinDir au 1er run)
//  2. binaire système sur PATH
func Locate(appBinDir string) (string, error) {
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
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	return "", errors.New("ffmpeg introuvable (ni embarqué, ni sur PATH) — installer brew install ffmpeg")
}

const (
	pcmRate      = 8000 // Hz, mono PCM extrait par ffmpeg
	envRate      = 100  // Hz, fenêtre RMS = pcmRate/envRate échantillons
	envWinSize   = pcmRate / envRate
	bytesPerSamp = 4 // float32 LE
	msPerEnvSamp = 1000 / envRate
)

// extractEnvelope lit `durationSec` secondes audio de la piste `tid` à partir
// de `startSec`, downmixée mono PCM 8 kHz, et calcule son enveloppe RMS @ 100 Hz.
func extractEnvelope(ctx context.Context, ffmpeg, mkvPath string, tid, startSec, durationSec int) ([]float64, error) {
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-ss", strconv.Itoa(startSec),
		"-i", mkvPath,
		"-map", fmt.Sprintf("0:%d", tid),
		"-t", strconv.Itoa(durationSec),
		"-vn", "-sn",
		"-ac", "1",
		"-ar", strconv.Itoa(pcmRate),
		"-f", "f32le",
		"-",
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	buf := make([]byte, envWinSize*bytesPerSamp)
	env := make([]float64, 0, durationSec*envRate)
	for {
		_, err := io.ReadFull(stdout, buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			_ = cmd.Wait()
			return nil, err
		}
		var sum float64
		for i := 0; i < envWinSize; i++ {
			f := math.Float32frombits(binary.LittleEndian.Uint32(buf[i*bytesPerSamp:]))
			sum += float64(f) * float64(f)
		}
		env = append(env, math.Sqrt(sum/float64(envWinSize)))
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	if len(env) < 100 {
		return nil, errors.New("piste vide ou trop courte")
	}
	return env, nil
}

// crossCorrelate trouve le lag (en pas de 10 ms) maximisant la corrélation de
// Pearson entre `other` et `ref`. Retourne :
//   - offsetMs : décalage à donner à mkvmerge --sync (positif = retarder la piste).
//   - confidence : pic de corrélation dans [-1,1] ; >0.7 = sync très fiable.
func crossCorrelate(ref, other []float64, maxLagMs int) (offsetMs int, confidence float64) {
	maxLag := maxLagMs / msPerEnvSamp
	if maxLag > len(ref)-100 {
		maxLag = len(ref) - 100
	}
	if maxLag > len(other)-100 {
		maxLag = len(other) - 100
	}
	if maxLag < 1 {
		return 0, 0
	}

	bestLag := 0
	bestCorr := -math.MaxFloat64

	for lag := -maxLag; lag <= maxLag; lag++ {
		// Overlap : ref[i0..i0+n] vs other[j0..j0+n]
		i0, j0 := 0, lag
		if lag < 0 {
			i0, j0 = -lag, 0
		}
		n := len(ref) - i0
		if len(other)-j0 < n {
			n = len(other) - j0
		}
		if n < 100 {
			continue
		}
		var sR, sO, sRO, sR2, sO2 float64
		for k := 0; k < n; k++ {
			r := ref[i0+k]
			o := other[j0+k]
			sR += r
			sO += o
			sRO += r * o
			sR2 += r * r
			sO2 += o * o
		}
		nf := float64(n)
		cov := sRO - sR*sO/nf
		varR := sR2 - sR*sR/nf
		varO := sO2 - sO*sO/nf
		denom := math.Sqrt(varR * varO)
		if denom <= 0 {
			continue
		}
		c := cov / denom
		if c > bestCorr {
			bestCorr = c
			bestLag = lag
		}
	}
	// bestLag > 0 : `other` est en retard de bestLag*10 ms par rapport à `ref`.
	// Pour aligner `other` sur `ref`, on doit l'AVANCER → mkvmerge --sync négatif.
	return -bestLag * msPerEnvSamp, bestCorr
}

// DetectionResult est le résultat d'une détection d'offset.
type DetectionResult struct {
	OffsetMs    int     `json:"offset_ms"`    // mkvmerge --sync (positif = retarder)
	Confidence  float64 `json:"confidence"`   // [-1,1] : >0.7 = très fiable
	DriftMs     int     `json:"drift_ms"`     // |offset_début - offset_fin| (0 si non mesuré)
	Method      string  `json:"method"`       // "constant" | "drift_linear" | "drift_unstable" | "low_confidence"
	TempoFactor float64 `json:"tempo_factor"` // ratio atempo à appliquer si drift linéaire (1.0 = pas de resample)
	Notes       string  `json:"notes"`
}

// DetectOffset mesure le décalage de `otherTID` par rapport à `refTID`. Si la
// durée est >20 min, mesure une 2ᵉ fois à 85% du film pour détecter un drift.
//
//   - durationSec : durée totale du film en secondes (0 si inconnue → pas de check drift).
func DetectOffset(ctx context.Context, ffmpeg, mkvPath string, refTID, otherTID int, durationSec float64) (*DetectionResult, error) {
	const window = 180     // 3 min par mesure
	const maxLagMs = 60000 // ±60s de recherche
	const startSkip = 30   // skip 30s pour éviter les intros silencieuses

	refStart, err := extractEnvelope(ctx, ffmpeg, mkvPath, refTID, startSkip, window)
	if err != nil {
		return nil, fmt.Errorf("extraction réf début : %w", err)
	}
	otherStart, err := extractEnvelope(ctx, ffmpeg, mkvPath, otherTID, startSkip, window)
	if err != nil {
		return nil, fmt.Errorf("extraction autre début : %w", err)
	}
	offStart, confStart := crossCorrelate(refStart, otherStart, maxLagMs)

	res := &DetectionResult{
		OffsetMs:    offStart,
		Confidence:  confStart,
		Method:      "constant",
		TempoFactor: 1.0,
	}
	if confStart < 0.4 {
		res.Method = "low_confidence"
		res.Notes = fmt.Sprintf("Corrélation faible (%.2f) — résultat peu fiable. Vérifie manuellement.", confStart)
		return res, nil
	}

	// Vérification drift : 2ᵉ mesure vers 85% du film.
	// Si drift > 200ms ET les 2 mesures sont fiables → drift linéaire (probablement
	// FPS différent) → calcule le ratio atempo nécessaire pour resample.
	if durationSec > 1200 { // > 20 min
		late := int(durationSec * 0.85)
		refLate, err := extractEnvelope(ctx, ffmpeg, mkvPath, refTID, late, window)
		if err == nil {
			otherLate, err := extractEnvelope(ctx, ffmpeg, mkvPath, otherTID, late, window)
			if err == nil {
				offLate, confLate := crossCorrelate(refLate, otherLate, maxLagMs)
				drift := offStart - offLate
				if drift < 0 {
					drift = -drift
				}
				res.DriftMs = drift
				if confLate < 0.4 {
					// Mesure fin peu fiable, on ne peut pas trancher drift vs noise
					if drift > 200 {
						res.Method = "drift_unstable"
						res.Notes = fmt.Sprintf("Drift %d ms mais conf fin faible (%.2f) — pas de resample auto.", drift, confLate)
					}
				} else if drift > 200 {
					// Drift fiable → linéaire, on peut compenser via atempo.
					// T_s = startSkip, T_e = late, en secondes.
					// Sur l'intervalle (T_e - T_s) du ref, "other" couvre (T_e - T_s) + (offLate - offStart)/1000 secondes.
					// Pour accélérer/ralentir other afin de matcher ref :
					//   tempo = ref_span / other_span = (T_e - T_s) / ((T_e - T_s) + (offLate - offStart)/1000)
					span := float64(late - startSkip)
					otherSpan := span + float64(offLate-offStart)/1000.0
					if otherSpan > 0 {
						tempo := span / otherSpan
						// Garde-fou : atempo accepte [0.5, 100], et un ratio raisonnable pour FPS est [0.9, 1.1].
						if tempo > 0.9 && tempo < 1.1 {
							res.TempoFactor = tempo
							res.Method = "drift_linear"
							pct := (tempo - 1.0) * 100.0
							res.Notes = fmt.Sprintf("Drift linéaire %d ms sur %.0fs (%.0f→%.0f) → resample atempo=%.6f (%+.3f%%) requis.", drift, span, float64(offStart), float64(offLate), tempo, pct)
						} else {
							res.Method = "drift_unstable"
							res.Notes = fmt.Sprintf("Drift %d ms mais ratio atempo (%.4f) hors plage raisonnable — vérifier manuellement.", drift, tempo)
						}
					}
				}
			}
		}
	}
	return res, nil
}

// ResampleParams configure une passe de réencode atempo (gestion drift FPS).
type ResampleParams struct {
	InputPath   string  // .mkv source
	TrackID     int     // piste audio à extraire+resample+réencoder
	Codec       string  // "ac3" | "eac3"
	Channels    int     // 2, 6, 8…
	BitrateKbps int     // ex 384, 640
	Tempo       float64 // ratio atempo (typiquement 0.95-1.05)
	OutputPath  string  // chemin de sortie .ac3 ou .eac3
}

// Resample décode la piste, applique atempo (resample SoX-quality dans ffmpeg),
// puis réencode dans le codec d'origine au même bitrate/canaux.
// 1 génération lossy mais sync parfaite après mux.
func Resample(ctx context.Context, ffmpeg string, p ResampleParams) error {
	var codecName string
	switch p.Codec {
	case "ac3", "AC3", "AC-3":
		codecName = "ac3"
	case "eac3", "EAC3", "E-AC3", "E-AC-3":
		codecName = "eac3"
	default:
		return fmt.Errorf("codec non géré pour resample : %s (seuls AC3/EAC3 supportés)", p.Codec)
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", p.InputPath,
		"-map", fmt.Sprintf("0:%d", p.TrackID),
		"-vn", "-sn",
		"-filter:a", fmt.Sprintf("atempo=%.6f", p.Tempo),
		"-c:a", codecName,
		"-ac", strconv.Itoa(p.Channels),
		"-b:a", fmt.Sprintf("%dk", p.BitrateKbps),
		p.OutputPath,
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg resample : %v — %s", err, string(out))
	}
	return nil
}
