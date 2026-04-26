//go:build linux && amd64

package alass

import _ "embed"

//go:embed binaries/linux-amd64/alass-cli
var embeddedBinary []byte

const embeddedName = "alass-cli"
