package embedded

import (
	"embed"
)

//go:embed *.tmpl
var FS embed.FS
