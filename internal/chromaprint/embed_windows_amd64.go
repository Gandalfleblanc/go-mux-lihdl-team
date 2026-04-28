//go:build windows && amd64

package chromaprint

import _ "embed"

//go:embed binaries/windows-amd64/fpcalc.exe
var embeddedBinary []byte

const embeddedName = "fpcalc.exe"
