package docs

import (
	"os"

	assetfs "github.com/elazarl/go-bindata-assetfs"
)

func AssetFS() *assetfs.AssetFS {
	return &assetfs.AssetFS{
		Prefix:    "docs",
		Asset:     Asset,
		AssetDir:  AssetDir,
		AssetInfo: os.Stat,
	}
}
