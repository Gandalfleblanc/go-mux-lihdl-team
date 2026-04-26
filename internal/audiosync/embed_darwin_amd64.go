//go:build darwin && amd64

package audiosync

import _ "embed"

//go:embed binaries/darwin-amd64/ffmpeg
var embeddedBinary []byte

const embeddedName = "ffmpeg"

//go:embed binaries/darwin-amd64/ffprobe
var embeddedFfprobeBinary []byte

const embeddedFfprobeName = "ffprobe"
