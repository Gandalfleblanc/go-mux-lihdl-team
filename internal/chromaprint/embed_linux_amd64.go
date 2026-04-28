//go:build linux && amd64

package chromaprint

import _ "embed"

//go:embed binaries/linux-amd64/fpcalc
var embeddedBinary []byte

const embeddedName = "fpcalc"
