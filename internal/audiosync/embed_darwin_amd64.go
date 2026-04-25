//go:build darwin && amd64

package audiosync

import _ "embed"

//go:embed binaries/darwin-amd64/ffmpeg
var embeddedBinary []byte

const embeddedName = "ffmpeg"
