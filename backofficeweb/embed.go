package backofficeweb

import "embed"

// Files contains the backoffice templates and static assets.
//
//go:embed static/* templates/*.html
var Files embed.FS
