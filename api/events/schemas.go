package events

import (
	"embed"
	"io/fs"
)

//go:embed v1/*.json
var publishedSchemas embed.FS

// Schemas exposes the published v1 contracts at the root of an offline filesystem.
var Schemas, _ = fs.Sub(publishedSchemas, "v1")
