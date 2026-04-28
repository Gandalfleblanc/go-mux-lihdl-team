//go:build windows && amd64

package mkvtool

import _ "embed"

//go:embed binaries/windows-amd64/mkvmerge.exe
var embeddedBinary []byte

//go:embed binaries/windows-amd64/mkvextract.exe
var embeddedExtract []byte

const embeddedName = "mkvmerge.exe"
const embeddedExtractName = "mkvextract.exe"
