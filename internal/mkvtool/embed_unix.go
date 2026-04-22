//go:build !windows

package mkvtool

import _ "embed"

// embeddedBinary contient le mkvmerge pour la plateforme courante.
// Vide en dev (placeholder), rempli par le CI avant `wails build`.
//
//go:embed binaries/mkvmerge
var embeddedBinary []byte

const embeddedName = "mkvmerge"
