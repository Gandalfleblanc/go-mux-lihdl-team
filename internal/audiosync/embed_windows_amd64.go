//go:build windows && amd64

package audiosync

import _ "embed"

//go:embed binaries/windows-amd64/ffmpeg.exe
var embeddedBinary []byte

const embeddedName = "ffmpeg.exe"

//go:embed binaries/windows-amd64/ffprobe.exe
var embeddedFfprobeBinary []byte

const embeddedFfprobeName = "ffprobe.exe"
