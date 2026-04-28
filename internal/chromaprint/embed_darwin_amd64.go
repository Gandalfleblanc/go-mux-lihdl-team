//go:build darwin && amd64

package chromaprint

import _ "embed"

//go:embed binaries/darwin-amd64/fpcalc
var embeddedBinary []byte

const embeddedName = "fpcalc"
