//go:build darwin && amd64

package mediainfo

import _ "embed"

//go:embed binaries/darwin-amd64/mediainfo
var embeddedBinary []byte

const embeddedName = "mediainfo"
