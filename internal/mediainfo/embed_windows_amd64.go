//go:build windows && amd64

package mediainfo

import _ "embed"

//go:embed binaries/windows-amd64/mediainfo.exe
var embeddedBinary []byte

const embeddedName = "mediainfo.exe"
