//go:build darwin && amd64

package alass

import _ "embed"

//go:embed binaries/darwin-amd64/alass-cli
var embeddedBinary []byte

const embeddedName = "alass-cli"
