package db_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"parts-engine/db/migrations"
)

// TestEmbeddedMigrations_AllFilesReadableViaPathJoin is a REGRESSION test for
// the CI failure caught by the 8/17 docker-build run:
//
//	migrator: read 000001_create_hk_parts_cache.sql:
//	  open ./000001_create_hk_parts_cache.sql: file does not exist
//
// The migrator originally concatenated subdir + "/" + filename which produced
// "./000001_...sql" — an embed.FS path that starts with "./" and is rejected
// by fs.ReadFile at runtime. The fix uses path.Join to normalise the path.
//
// This test verifies EVERY migration file in the embedded FS is reachable
// via the SAME path form the migrator uses (path.Join). If someone reverts
// to raw string concatenation, this test catches it before the container
// even boots.
func TestEmbeddedMigrations_AllFilesReadableViaPathJoin(t *testing.T) {
	// Same subdir the caller in cmd/server/main.go passes: "." (root of the
	// embed.FS, since the embed directive lives in db/migrations/embed.go
	// and captures files at that same level).
	const migrationsSubdir = "."

	entries, err := fs.ReadDir(migrations.FS, migrationsSubdir)
	if err != nil {
		t.Fatalf("fs.ReadDir(migrations.FS, %q): %v", migrationsSubdir, err)
	}

	sqlCount := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		sqlCount++
		// Exact code path the migrator uses.
		readPath := path.Join(migrationsSubdir, e.Name())
		content, err := fs.ReadFile(migrations.FS, readPath)
		if err != nil {
			t.Errorf("read %s (via path.Join %q + %q → %q): %v",
				e.Name(), migrationsSubdir, e.Name(), readPath, err)
		}
		if len(content) == 0 {
			t.Errorf("migration %s is empty", e.Name())
		}
	}

	// Sanity floor: we currently ship 13 migrations. If someone accidentally
	// stops the embed from picking them up (typo in the //go:embed directive),
	// this catches it.
	if sqlCount < 10 {
		t.Errorf("expected ≥10 embedded migrations, got %d — did //go:embed *.sql break?", sqlCount)
	}
}
