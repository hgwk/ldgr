package viewer

import (
	"embed"
	"io/fs"
)

//go:embed assets
var assetsFS embed.FS

// Assets returns the embedded /assets directory as a fs.FS rooted at "assets".
func Assets() fs.FS {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
