// Package webui embeds the compiled SPA (ui/ build output) into the binary.
// The build output is committed under public/ so `go build` needs no npm,
// matching the access-control convention.
package webui

import "embed"

//go:embed all:public
var FS embed.FS
