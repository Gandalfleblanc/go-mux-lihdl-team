//go:build darwin && arm64

package audiosync

import _ "embed"

//go:embed binaries/darwin-arm64/ffmpeg
var embeddedBinary []byte

const embeddedName = "ffmpeg"
