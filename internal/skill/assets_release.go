//go:build assets

package skill

import _ "embed"

//go:embed cardamom.tar.gz
var releaseArchive []byte

func init() {
	embeddedArchive = releaseArchive
}
