//go:build darwin && arm64

package audiosync

import _ "embed"

//go:embed binaries/darwin-arm64/ffmpeg
var embeddedBinary []byte

const embeddedName = "ffmpeg"

//go:embed binaries/darwin-arm64/ffprobe
var embeddedFfprobeBinary []byte

const embeddedFfprobeName = "ffprobe"
