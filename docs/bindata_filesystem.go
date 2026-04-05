package docs

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed docs
var swaggerStatic embed.FS

// AssetFS returns an http.FileSystem for swagger UI static assets.
func AssetFS() http.FileSystem {
	sub, err := fs.Sub(swaggerStatic, "docs")
	if err != nil {
		panic(err)
	}

	return http.FS(sub)
}
