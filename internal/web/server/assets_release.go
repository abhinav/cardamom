//go:build webassets

package server

import _ "embed"

//go:embed static.tar.gz
var releaseArchive []byte

func init() {
	embeddedApplicationArchive = releaseArchive
}
