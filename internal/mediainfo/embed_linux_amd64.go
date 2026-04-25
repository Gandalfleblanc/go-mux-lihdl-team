//go:build linux && amd64

package mediainfo

import _ "embed"

//go:embed binaries/linux-amd64/mediainfo
var embeddedBinary []byte

const embeddedName = "mediainfo"
