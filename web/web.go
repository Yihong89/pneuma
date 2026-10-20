// Package web holds the browser app, embedded into the binary.
package web

import "embed"

//go:embed index.html app.js style.css sw.js manifest.webmanifest icons fonts
var FS embed.FS
