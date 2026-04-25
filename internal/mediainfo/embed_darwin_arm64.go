//go:build darwin && arm64

package mediainfo

import _ "embed"

//go:embed binaries/darwin-arm64/mediainfo
var embeddedBinary []byte

const embeddedName = "mediainfo"
