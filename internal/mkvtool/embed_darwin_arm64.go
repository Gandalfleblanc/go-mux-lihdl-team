//go:build darwin && arm64

package mkvtool

import _ "embed"

//go:embed binaries/darwin-arm64/mkvmerge
var embeddedBinary []byte

//go:embed binaries/darwin-arm64/mkvextract
var embeddedExtract []byte

const embeddedName = "mkvmerge"
const embeddedExtractName = "mkvextract"
