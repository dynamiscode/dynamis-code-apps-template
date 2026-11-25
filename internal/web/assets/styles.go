// Package assets embeds the application's browser assets.
package assets

import "embed"

// Files contains the assets served by the web app.
//
//go:embed app.css clients.css components.css tokens.css
//go:embed alpine-components.js htmx.min.js js/alpine.min.js
var Files embed.FS
