//go:build windows && amd64

package alass

import _ "embed"

//go:embed binaries/windows-amd64/alass-cli.exe
var embeddedBinary []byte

const embeddedName = "alass-cli.exe"
