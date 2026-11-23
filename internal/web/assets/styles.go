// Package assets embeds the application's browser assets.
package assets

import "embed"

// Files contains the assets served by the web app.
//
//go:embed *.css *.js js
var Files embed.FS
