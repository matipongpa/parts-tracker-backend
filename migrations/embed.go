package migrations

import "embed"

// FS holds the versioned SQL files applied by internal/platform/migrate.
//
//go:embed *.sql
var FS embed.FS
