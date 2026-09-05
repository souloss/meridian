// Package buildinfo contains build-time identity shared by the HTTP server and CLI.
package buildinfo

import "os"

var (
	Version = "dev"
	Commit  = "unknown"
)

func CurrentVersion() string {
	if value := os.Getenv("MERIDIAN_VERSION"); value != "" {
		return value
	}
	return Version
}

func CurrentCommit() string {
	if value := os.Getenv("MERIDIAN_BUILD_COMMIT"); value != "" {
		return value
	}
	return Commit
}
