// Package naming contient les normes de renommage LiHDL.
// Toutes les constantes ici sont figées et correspondent aux dropdowns de l'UI.
package naming

// AudioLabels est la liste ordonnée des libellés audio LiHDL.
// Format : "{LANG} {VERSION} : {CODEC} {CANAUX}".
// VFF = Version Française de France, VFQ = Version Française Québécoise,
// VFi = Version Française Internationale, VO = Version Originale.
var AudioLabels = []string{
	"FR VFF : AC3 5.1",
	"FR VFF : AC3 2.0",
	"FR VFF : AC3 1.0",
	"FR VFQ : AC3 5.1",
	"FR VFQ : AC3 2.0",
	"FR VFQ : AC3 1.0",
	"FR VFF : EAC3 5.1",
	"FR VFF : EAC3 2.0",
	"FR VFQ : EAC3 5.1",
	"FR VFQ : EAC3 2.0",
	"FR VFi : AC3 5.1",
	"FR VFi : AC3 2.0",
	"FR VFi : AC3 1.0",
	"FR AD : AC3 5.1",
	"FR AD : AC3 2.0",
	"FR AD : AC3 1.0",
	"FR AD : EAC3 5.1",
	"FR AD : EAC3 2.0",
	"ENG VO : AC3 5.1",
	"ENG VO : AC3 2.0",
	"ENG VO : AC3 1.0",
	"ENG VO : EAC3 5.1",
	"ENG VO : EAC3 2.0",
	"JPN VO : AC3 5.1",
	"JPN VO : AC3 2.0",
	"JPN VO : AC3 1.0",
	"ITA VO : AC3 5.1",
	"ITA VO : AC3 2.0",
	"ITA VO : AC3 1.0",
}

// SubtitleLabels est la liste ordonnée des libellés sous-titres LiHDL.
// Formats : SRT (texte) et PGS (image bluray).
var SubtitleLabels = []string{
	// SRT
	"FR Forced : SRT",
	"FR Full : SRT",
	"FR SDH : SRT",
	"FR VFF Forced : SRT",
	"FR VFF Full : SRT",
	"FR VFF SDH : SRT",
	"FR VFQ Forced : SRT",
	"FR VFQ Full : SRT",
	"FR VFQ SDH : SRT",
	"ENG Forced : SRT",
	"ENG Full : SRT",
	// PGS
	"FR Forced : PGS",
	"FR Full : PGS",
	"FR SDH : PGS",
	"FR VFF Forced : PGS",
	"FR VFF Full : PGS",
	"FR VFF SDH : PGS",
	"FR VFQ Forced : PGS",
	"FR VFQ Full : PGS",
	"FR VFQ SDH : PGS",
	"ENG Forced : PGS",
	"ENG Full : PGS",
}

// Dropdowns pour la piste vidéo.
var (
	VideoQualities = []string{"HDLight", "WEBRip", "WEB", "WEB.Light", "Custom PSA"}
	VideoEncoders  = []string{"GANDALF", "FilmZ", "Serveurperso", "Arcaldia", "Nox"}
	VideoSources   = []string{"REMUX LiHDL", "REMUX CUSTOM LiHDL", "WEBRip"}
	VideoTeams     = []string{"LiHDL"}
)

// VideoTrackName construit le nom de piste vidéo LiHDL :
// "{Qualité} By {Encodeur} Source {TypeSource} {Team}".
func VideoTrackName(quality, encoder, source, team string) string {
	return quality + " By " + encoder + " Source " + source + " " + team
}

// IsCustomSource retourne true si le type source est une variante CUSTOM
// (qui ajoute ".CUSTOM" dans le nom de fichier final).
func IsCustomSource(source string) bool {
	return source == "REMUX CUSTOM LiHDL"
}

// LangFlag calcule le flag langue pour le nom de fichier final selon les
// pistes audio sélectionnées. Règles :
//   - 1 VFF seule                  → "VFF"
//   - 1 VFQ seule                  → "VFQ"
//   - 1 VO seule (ENG/JPN/ITA)     → "VO"
//   - 1 VFF + 1 VO                 → "MULTi.VFF"
//   - 1 VFF + 1 VFQ                → "MULTi.VF2"
//   - 1 VFF + 1 VFQ + 1 VO         → "MULTi.VF2"
//   - autre combinaison            → best-effort
//
// selectedLabels contient les libellés audio LiHDL retenus (cf. AudioLabels).
func LangFlag(selectedLabels []string) string {
	hasVFF, hasVFQ, hasVFi, hasVO := false, false, false, false
	for _, lbl := range selectedLabels {
		switch {
		case containsTag(lbl, "VFF"):
			hasVFF = true
		case containsTag(lbl, "VFQ"):
			hasVFQ = true
		case containsTag(lbl, "VFi"):
			hasVFi = true
		case containsTag(lbl, "VO"):
			hasVO = true
		}
	}
	vfCount := 0
	if hasVFF {
		vfCount++
	}
	if hasVFQ {
		vfCount++
	}
	if hasVFi {
		vfCount++
	}
	switch {
	case vfCount >= 2:
		return "MULTi.VF2"
	case hasVFF && hasVO:
		return "MULTi.VFF"
	case hasVFQ && hasVO:
		return "MULTi.VFQ"
	case hasVFi && hasVO:
		return "MULTi.VFi"
	case hasVFF:
		return "VFF"
	case hasVFQ:
		return "VFQ"
	case hasVFi:
		return "VFi"
	case hasVO:
		return "VO"
	}
	return "VO"
}

// FilenameParams regroupe les infos nécessaires à la construction du nom
// de fichier final LiHDL.
type FilenameParams struct {
	Title        string   // titre TMDB (espaces autorisés, remplacés par .)
	Year         string   // année (4 chiffres)
	AudioLabels  []string // libellés LiHDL des pistes audio gardées
	Resolution   string   // ex "1080p", "2160p"
	Source       string   // ex "BluRay", "WEB-DL", "WEBRip"
	Format       string   // ex "REMUX" (vide pour pas de format)
	AudioCodecs  []string // codecs+canaux détectés, ex ["AC3.5.1", "DTS-HD.MA.5.1"]
	VideoCodec   string   // "AVC" | "HEVC" | "AV1"
	Team         string   // ex "LiHDL"
	CustomSource bool     // si true, insère ".CUSTOM" après l'année
}

// BuildFilename assemble le nom de fichier final LiHDL selon le template :
//
//	{Title}.{Year}[.CUSTOM].{Flag}.{Resolution}.{Source}[.{Format}].{AudioCodecs}.{VideoCodec}-{Team}.mkv
//
// Exemples :
//
//	Not.Without.Hope.2025.MULTi.VF2.1080p.WEBRip.AC3.5.1.H264-LiHDL.mkv
//	Fortress.2012.CUSTOM.MULTi.VFF.1080p.BluRay.REMUX.AC3.5.1.DTS-HD.MA.5.1.AVC-LiHDL.mkv
func BuildFilename(p FilenameParams) string {
	var parts []string
	parts = append(parts, dotify(p.Title))
	if p.Year != "" {
		parts = append(parts, p.Year)
	}
	if p.CustomSource {
		parts = append(parts, "CUSTOM")
	}
	parts = append(parts, LangFlag(p.AudioLabels))
	if p.Resolution != "" {
		parts = append(parts, p.Resolution)
	}
	if p.Source != "" {
		parts = append(parts, p.Source)
	}
	if p.Format != "" {
		parts = append(parts, p.Format)
	}
	for _, ac := range p.AudioCodecs {
		if ac != "" {
			parts = append(parts, ac)
		}
	}
	if p.VideoCodec != "" {
		parts = append(parts, p.VideoCodec)
	}
	name := joinDot(parts)
	if p.Team != "" {
		name += "-" + p.Team
	}
	return name + ".mkv"
}

// dotify remplace les espaces par des points (norme LiHDL pour les noms
// de fichier). Trim les espaces en début/fin.
func dotify(s string) string {
	s = trimSpace(s)
	out := make([]byte, 0, len(s))
	prevDot := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' {
			if !prevDot {
				out = append(out, '.')
				prevDot = true
			}
			continue
		}
		out = append(out, c)
		prevDot = c == '.'
	}
	return string(out)
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && s[i] == ' ' {
		i++
	}
	for j > i && s[j-1] == ' ' {
		j--
	}
	return s[i:j]
}

func joinDot(parts []string) string {
	n := 0
	for _, p := range parts {
		if p != "" {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += "."
		}
		out += p
	}
	return out
}

// MapCodecToLiHDL convertit un codec vidéo brut (de mkvmerge) vers la
// nomenclature LiHDL : H264 / H265 / AV1.
func MapCodecToLiHDL(codecID, codec string) string {
	c := codecID + " " + codec
	switch {
	case containsAny(c, "AVC", "MPEG4", "H.264", "H264", "h264"):
		return "H264"
	case containsAny(c, "HEVC", "MPEGH", "H.265", "H265", "h265"):
		return "H265"
	case containsAny(c, "AV1", "av01"):
		return "AV1"
	}
	return ""
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func containsTag(s, tag string) bool {
	// cherche le tag entouré d'espaces ou en début/fin.
	idx := indexOf(s, tag)
	if idx < 0 {
		return false
	}
	before := idx == 0 || s[idx-1] == ' '
	end := idx + len(tag)
	after := end == len(s) || s[end] == ' ' || s[end] == ':'
	return before && after
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
