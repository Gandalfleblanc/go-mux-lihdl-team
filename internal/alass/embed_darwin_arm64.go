//go:build darwin && arm64

package alass

import _ "embed"

//go:embed binaries/darwin-arm64/alass-cli
var embeddedBinary []byte

const embeddedName = "alass-cli"
