//go:build linux && amd64

package mkvtool

import _ "embed"

//go:embed binaries/linux-amd64/mkvmerge
var embeddedBinary []byte

//go:embed binaries/linux-amd64/mkvextract
var embeddedExtract []byte

const embeddedName = "mkvmerge"
const embeddedExtractName = "mkvextract"
