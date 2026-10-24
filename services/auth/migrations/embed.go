// Package migrations embeds the versioned Auth SQL for the migration command.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
