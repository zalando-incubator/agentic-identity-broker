// Package version holds build metadata injected at link time via -ldflags -X.
package version

var (
	Version = "dev"
	Commit  = "none"
)
