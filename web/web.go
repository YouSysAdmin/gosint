// Gosint, Copyright (c) 2026 YouSysAdmin

// Package web embeds the built Vue console so a running instance can
// serve it at /app.
//
// Only a .gitkeep is committed under dist/, so a plain `go build` with
// no `npm run build` still compiles. Available() then reports false
// and the console routes are not registered.
package web

import (
	"embed"
	"io/fs"
)

// Frontend is the built console, compiled into the binary.
//
// The embed pattern needs at least one match, which is why
// dist/.gitkeep is committed and why `task web` puts it back after the
// build empties the directory.
//
//go:embed all:dist
var Frontend embed.FS

// FS returns the console root (the contents of dist), or nil when no
// console was built into this binary.
func FS() fs.FS {
	if !Available() {
		return nil
	}

	sub, err := fs.Sub(Frontend, "dist")
	if err != nil {
		return nil
	}

	return sub
}

// Available reports whether a real build was embedded.
func Available() bool {
	f, err := Frontend.Open("dist/index.html")
	if err != nil {
		return false
	}

	_ = f.Close()

	return true
}
