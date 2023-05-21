package docs

//nolint:lll
//go:generate go run github.com/swaggo/swag/cmd/swag@master i --parseDependency --instanceName api --outputTypes go  --parseInternal --dir ../ -g main.go --output .
//go:generate go run github.com/kevinburke/go-bindata/v4/go-bindata@latest -pkg docs -modtime 1587356420 -o bindata.go docs
