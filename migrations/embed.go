package migrations

import "embed"

// Files contains the immutable SQL migration set. The migration runner verifies
// checksums so an already-applied migration cannot be silently rewritten.
//
//go:embed *.sql
var Files embed.FS
