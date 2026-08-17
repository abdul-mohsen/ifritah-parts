// Package migrations bundles the schema-migration SQL files into the Go
// binary so deployments never need to ship the db/migrations/ directory
// separately. Used by internal/db/migrator.go.
//
// Every file matched by the embed directive below is a valid SQL script
// (verified by internal/db/migrator_test.go — it parses each file and
// checks it doesn't fail to open).
package migrations

import "embed"

// FS is the read-only filesystem of every migration script. Files are
// sorted lexicographically at run time so the numeric prefix
// (000001…000013…) drives execution order.
//
//go:embed *.sql
var FS embed.FS
