// Package audiosync détecte et corrige le décalage temporel entre 2 pistes
// audio d'un .mkv (ex : VFF + VFQ d'un même film). Ne réencode jamais le son :
//   - Détection : extraction PCM mono 8 kHz via ffmpeg → enveloppe RMS @ 100 Hz
//                 → cross-correlation pour trouver le lag.
//   - Recalage : appliqué via mkvmerge --sync TID:OFFSET (timecodes uniquement,
//                copie bit-à-bit du flux compressé AC3/EAC3).
package audiosync

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

// LocateFfprobe : même logique que Locate mais pour ffprobe (utilisé par alass-cli).
func LocateFfprobe(appBinDir string) (string, error) {
	if appBinDir != "" && len(embeddedFfprobeBinary) > 0 {
		candidate := filepath.Join(appBinDir, embeddedFfprobeName)
		if _, err := os.Stat(candidate); err != nil {
			if werr := os.WriteFile(candidate, embeddedFfprobeBinary, 0755); werr == nil {
				return candidate, nil
			}
		} else {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p, nil
	}
	return "", errors.New("ffprobe introuvable (ni embarqué, ni sur PATH)")
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

// parseSRTSpeechSignal lit un fichier SRT et construit un signal binaire à
// 100 Hz (1 échantillon = 10 ms) qui vaut 1.0 quand une ligne de sub est active,
// 0.0 sinon. Le signal couvre la fenêtre [startSec, startSec+durationSec].
// Format SRT attendu : "HH:MM:SS,mmm --> HH:MM:SS,mmm".
func parseSRTSpeechSignal(srtPath string, startSec, durationSec int) ([]float64, error) {
	f, err := os.Open(srtPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	const sampleRate = 100 // 100 Hz = 10 ms par sample
	totalSamples := durationSec * sampleRate
	signal := make([]float64, totalSamples)
	startMs := startSec * 1000
	endMs := (startSec + durationSec) * 1000
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, "-->") {
			continue
		}
		parts := strings.Split(line, "-->")
		if len(parts) != 2 {
			continue
		}
		startTC, err1 := parseSRTTimecode(strings.TrimSpace(parts[0]))
		endTC, err2 := parseSRTTimecode(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			continue
		}
		// Clamp à la fenêtre [startMs, endMs]
		if endTC <= startMs || startTC >= endMs {
			continue
		}
		s := startTC - startMs
		e := endTC - startMs
		if s < 0 {
			s = 0
		}
		if e > durationSec*1000 {
			e = durationSec * 1000
		}
		fromIdx := s / 10
		toIdx := e / 10
		for i := fromIdx; i < toIdx && i < totalSamples; i++ {
			signal[i] = 1.0
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return signal, nil
}

// parseSRTTimecode convertit "HH:MM:SS,mmm" en millisecondes.
func parseSRTTimecode(tc string) (int, error) {
	tc = strings.ReplaceAll(tc, ",", ".")
	parts := strings.Split(tc, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("timecode invalide : %s", tc)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	secStr := parts[2]
	secF, err := strconv.ParseFloat(secStr, 64)
	if err != nil {
		return 0, err
	}
	return h*3600000 + m*60000 + int(secF*1000), nil
}

// extractSpeechSignalSilenceDetect utilise le filtre ffmpeg `silencedetect` pour
// produire un signal binaire de présence vocale à 100 Hz. 1 = audio actif (voix
// ou son), 0 = silence détecté. Beaucoup plus précis que la binarisation par
// seuillage de l'enveloppe RMS (qui confond musique/SFX et voix).
func extractSpeechSignalSilenceDetect(ctx context.Context, ffmpeg, mkvPath string, audioTID, startSec, durationSec int) ([]float64, error) {
	args := []string{
		"-hide_banner", "-nostdin",
		"-ss", strconv.Itoa(startSec),
		"-i", mkvPath,
		"-map", fmt.Sprintf("0:%d", audioTID),
		"-t", strconv.Itoa(durationSec),
		"-vn", "-sn",
		"-af", "silencedetect=noise=-25dB:d=0.25",
		"-f", "null",
		"-",
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg silencedetect : %v", err)
	}
	output := stderr.String()
	totalSamples := durationSec * envRate
	signal := make([]float64, totalSamples)
	for i := range signal {
		signal[i] = 1.0 // par défaut : présence vocale
	}
	startRe := regexp.MustCompile(`silence_start:\s*(-?\d+(?:\.\d+)?)`)
	endRe := regexp.MustCompile(`silence_end:\s*(-?\d+(?:\.\d+)?)`)
	starts := startRe.FindAllStringSubmatch(output, -1)
	ends := endRe.FindAllStringSubmatch(output, -1)
	for i, sm := range starts {
		startSec, _ := strconv.ParseFloat(sm[1], 64)
		var endSec float64
		if i < len(ends) {
			endSec, _ = strconv.ParseFloat(ends[i][1], 64)
		} else {
			endSec = float64(durationSec)
		}
		startIdx := int(startSec * float64(envRate))
		endIdx := int(endSec * float64(envRate))
		if startIdx < 0 {
			startIdx = 0
		}
		if endIdx > totalSamples {
			endIdx = totalSamples
		}
		for j := startIdx; j < endIdx; j++ {
			signal[j] = 0.0
		}
	}
	return signal, nil
}

// DetectSRTOffset compare un fichier SRT à l'audio d'une piste mkv et retourne
// le décalage à appliquer au sub (en ms) pour qu'il colle aux voix. Cross-
// correlation entre l'enveloppe d'amplitude audio et un signal binaire de
// présence des subs.
//
// Pour gérer des décalages jusqu'à ±90 s, l'enveloppe audio est extraite sur
// une fenêtre [audioStart, audioStart+audioWin] et le signal SRT sur une
// fenêtre PLUS LARGE [srtStart, srtStart+srtWin] qui englobe ±90 s autour de
// la fenêtre audio. La cross-correlation cherche le lag qui aligne les 2.
func DetectSRTOffset(ctx context.Context, ffmpeg, mkvPath string, audioTID int, srtPath string) (*DetectionResult, error) {
	const audioWin = 180       // 3 min d'audio
	const audioStart = 90      // skip 90 s pour avoir 90s avant + 90s après dans le SRT
	const srtPad = 90          // 90 s de marge autour de la fenêtre audio dans le SRT
	const srtStart = 0         // SRT commence à 0
	const srtWin = audioStart + audioWin + srtPad // 360 s
	const maxLagMs = 180000    // ±180 s de recherche dans la cross-correlation

	// Signal de présence vocale audio via silencedetect ffmpeg : binaire
	// 1 = parole/son, 0 = silence détecté (-30dB pendant 0.3s+).
	audioBin, err := extractSpeechSignalSilenceDetect(ctx, ffmpeg, mkvPath, audioTID, audioStart, audioWin)
	if err != nil {
		return nil, fmt.Errorf("extraction speech audio : %w", err)
	}
	srtSig, err := parseSRTSpeechSignal(srtPath, srtStart, srtWin)
	if err != nil {
		return nil, fmt.Errorf("parse SRT : %w", err)
	}
	rawOffsetMs, conf := crossCorrelate(audioBin, srtSig, maxLagMs)
	// Compensation : audio[0] correspond à audio_time=audioStart, mais SRT[0]
	// correspond à srt_time=srtStart=0. La cross-correlation a trouvé bestLag
	// tel que audio[0] s'aligne avec SRT[bestLag]. Naturellement, pour un SRT
	// non-décalé, audio_time=audioStart correspond à srt_time=audioStart, donc
	// audio[0] devrait s'aligner avec SRT[audioStart*100]. La fonction renvoie
	// -bestLag*10 ms. On compense en ajoutant (audioStart-srtStart)*1000 ms.
	naturalLagMs := (audioStart - srtStart) * 1000
	offsetMs := rawOffsetMs + naturalLagMs
	res := &DetectionResult{
		OffsetMs:    offsetMs,
		Confidence:  conf,
		Method:      "constant",
		TempoFactor: 1.0,
	}
	if conf < 0.3 {
		res.Method = "low_confidence"
		res.Notes = fmt.Sprintf("Corrélation faible (%.2f) — résultat peu fiable.", conf)
	}
	return res, nil
}

// DetectOffsetCross mesure le décalage entre la piste `otherTID` du fichier
// `otherPath` et la piste `refTID` du fichier `refPath`. Cas typique : comparer
// une piste audio FR de la source LiHDL avec une piste audio FR extraite d'un
// autre fichier (VFF/VFQ récupérés depuis une release différente). Mêmes garde-
// fous (drift sur films > 20 min, low confidence threshold) que DetectOffset.
func DetectOffsetCross(ctx context.Context, ffmpeg, refPath string, refTID int, otherPath string, otherTID int, durationSec float64) (*DetectionResult, error) {
	const window = 180
	const maxLagMs = 60000
	const startSkip = 30

	refStart, err := extractEnvelope(ctx, ffmpeg, refPath, refTID, startSkip, window)
	if err != nil {
		return nil, fmt.Errorf("extraction réf début : %w", err)
	}
	otherStart, err := extractEnvelope(ctx, ffmpeg, otherPath, otherTID, startSkip, window)
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
		res.Notes = fmt.Sprintf("Corrélation faible (%.2f) — résultat peu fiable.", confStart)
		return res, nil
	}
	if durationSec > 1200 {
		late := int(durationSec * 0.85)
		refLate, err := extractEnvelope(ctx, ffmpeg, refPath, refTID, late, window)
		if err == nil {
			otherLate, err := extractEnvelope(ctx, ffmpeg, otherPath, otherTID, late, window)
			if err == nil {
				offLate, confLate := crossCorrelate(refLate, otherLate, maxLagMs)
				drift := offStart - offLate
				if drift < 0 {
					drift = -drift
				}
				res.DriftMs = drift
				if confLate < 0.4 {
					if drift > 200 {
						res.Method = "drift_unstable"
						res.Notes = fmt.Sprintf("Drift %d ms mais conf fin faible (%.2f).", drift, confLate)
					}
				} else if drift > 200 {
					span := float64(late - startSkip)
					otherSpan := span + float64(offLate-offStart)/1000.0
					if otherSpan > 0 {
						tempo := span / otherSpan
						if tempo > 0.9 && tempo < 1.1 {
							res.TempoFactor = tempo
							res.Method = "drift_linear"
							pct := (tempo - 1.0) * 100.0
							res.Notes = fmt.Sprintf("Drift linéaire %d ms → atempo=%.6f (%+.3f%%).", drift, tempo, pct)
						} else {
							res.Method = "drift_unstable"
							res.Notes = fmt.Sprintf("Drift %d ms mais ratio atempo hors plage.", drift)
						}
					}
				}
			}
		}
	}
	return res, nil
}

// ConvertAudioToAC3 décode une piste audio depuis un MKV (n'importe quel codec
// source : EAC3, DTS, TrueHD, FLAC, AAC, etc.) et la réencode en AC3 dans un
// fichier de sortie. Le nombre de canaux est respecté pour 1.0/2.0/5.1, et
// downmixé en 5.1 pour 7.1 (AC3 ne supporte pas nativement 8 canaux). Bitrate
// LiHDL : 96k mono, 192k stéréo, 448k 5.1.
//
// Si durationSec > 0 et progress != nil, parse la sortie -progress de ffmpeg
// pour appeler progress(percent) au fur et à mesure (0..100).
func ConvertAudioToAC3(ctx context.Context, ffmpeg, mkvPath string, trackID, channels int, outputPath string, durationSec float64, progress func(percent int)) error {
	// JAMAIS d'upmix : on ne peut que conserver ou downmixer (jamais ajouter de canaux).
	// AC3 supporte max 6 canaux → 7.1 (8) est downmixé en 5.1 (6).
	// Pour 5.1 (6), 5.0 (5), 2.0 (2), 1.0 (1) : on conserve le nombre source.
	targetCh := channels
	if targetCh > 6 {
		targetCh = 6 // Downmix 7.1 → 5.1 autorisé
	}
	// Bitrate norme LiHDL : 192k pour 2.0, 448k pour 5.1.
	bitrate := 448
	switch {
	case targetCh == 1:
		bitrate = 96
	case targetCh == 2:
		bitrate = 192
	case targetCh >= 5:
		bitrate = 448
	default:
		bitrate = 256
	}
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-i", mkvPath,
		"-map", fmt.Sprintf("0:%d", trackID),
		"-vn", "-sn",
		"-c:a", "ac3",
	}
	// Ne passe -ac que si on connaît un nombre de canaux valide (sinon ffmpeg
	// utilise les canaux source par défaut — pas d'upmix accidentel).
	if targetCh >= 1 {
		args = append(args, "-ac", strconv.Itoa(targetCh))
	}
	args = append(args, "-b:a", fmt.Sprintf("%dk", bitrate))
	// -progress pipe:1 : ffmpeg écrit out_time_us=... key=value sur stdout.
	if progress != nil && durationSec > 0 {
		args = append(args, "-progress", "pipe:1", "-loglevel", "error")
	} else {
		args = append(args, "-loglevel", "error")
	}
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if progress == nil || durationSec <= 0 {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("ffmpeg convert→AC3 : %v — %s", err, string(out))
		}
		return nil
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg pipe stdout : %w", err)
	}
	stderrBuf := &strings.Builder{}
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start : %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	totalUs := int64(durationSec * 1e6)
	lastPct := -1
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_us=") {
			us, _ := strconv.ParseInt(strings.TrimPrefix(line, "out_time_us="), 10, 64)
			if us > 0 && totalUs > 0 {
				pct := int(us * 100 / totalUs)
				if pct > 100 {
					pct = 100
				}
				if pct != lastPct {
					lastPct = pct
					progress(pct)
				}
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg convert→AC3 : %v — %s", err, stderrBuf.String())
	}
	progress(100)
	return nil
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

// ResampleAudioFile applique un changement de vitesse à un fichier audio AC3/EAC3
// standalone, conversion PAL↔NTSC style : la vitesse ET le pitch changent ensemble
// (comme jouer une cassette à 96% de sa vitesse — voix légèrement plus graves).
//
// On utilise asetrate (modifie la SR perçue → vitesse + pitch) puis aresample
// (rétablit la SR de sortie). C'est la méthode standard pour adapter un audio
// d'un FPS source vers un autre FPS (ex: 25fps → 24fps).
//
// atempo (utilisé pour la sync sub) GARDE le pitch et change juste la vitesse —
// c'est correct pour un drift de sync, mais FAUX pour un changement de FPS.
//
// 1 génération lossy mais durée parfaitement adaptée + pitch correct.
func ResampleAudioFile(ctx context.Context, ffmpeg, srcPath, dstPath, codec string, channels, bitrateKbps int, tempo float64) error {
	codecName := "ac3"
	switch codec {
	case "ac3", "AC3", "AC-3":
		codecName = "ac3"
	case "eac3", "EAC3", "E-AC3", "E-AC-3":
		codecName = "eac3"
	}
	// AC3/EAC3 sont quasi toujours en 48 kHz. asetrate=newSR change la SR
	// perçue (=> vitesse+pitch), aresample=48000 force la SR de sortie standard.
	const baseSR = 48000
	newSR := int(float64(baseSR) * tempo)
	filter := fmt.Sprintf("asetrate=%d,aresample=%d", newSR, baseSR)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", srcPath,
		"-filter:a", filter,
		"-c:a", codecName,
		"-ac", strconv.Itoa(channels),
		"-b:a", fmt.Sprintf("%dk", bitrateKbps),
		dstPath,
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg resample audio : %v — %s", err, string(out))
	}
	return nil
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
