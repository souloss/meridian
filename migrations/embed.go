// Package migrations embeds the SQL schema migrations into the Meridian binary.
package migrations

import "embed"

// FS contains the application-owned SQL migrations.
//
//go:embed *.sql
var FS embed.FS
