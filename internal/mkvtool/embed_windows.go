//go:build windows

package mkvtool

import _ "embed"

//go:embed binaries/mkvmerge.exe
var embeddedBinary []byte

const embeddedName = "mkvmerge.exe"
