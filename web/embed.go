// Package web embeds the built frontend (web/dist) into the binary.
// Run `make web-build` to populate dist before building for release.
package web

import "embed"

// DistFS contains the built SPA assets under "dist/".
//
//go:embed all:dist
var DistFS embed.FS
