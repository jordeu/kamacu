package web

import "embed"

// all: prefix required — plain //go:embed dist skips dotfiles and _-prefixed files
//
//go:embed all:dist
var DistFS embed.FS
