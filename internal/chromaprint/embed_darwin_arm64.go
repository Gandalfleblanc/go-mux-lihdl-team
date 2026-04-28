//go:build darwin && arm64

package chromaprint

import _ "embed"

//go:embed binaries/darwin-arm64/fpcalc
var embeddedBinary []byte

const embeddedName = "fpcalc"
