// Package migrations embeds the SQL migration files so they can be applied
// programmatically via golang-migrate (source/iofs) without shipping the
// raw .sql files alongside the binary.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
