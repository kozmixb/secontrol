// Package assets provides the web interface embedded in the SeControl binary.
package assets

import "embed"

// Files contains the static dashboard assets.
//
//go:embed index.html styles.css app.js
var Files embed.FS
