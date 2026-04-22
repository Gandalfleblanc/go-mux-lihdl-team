//go:build windows && amd64

package mkvtool

import _ "embed"

//go:embed binaries/windows-amd64/mkvmerge.exe
var embeddedBinary []byte

const embeddedName = "mkvmerge.exe"
