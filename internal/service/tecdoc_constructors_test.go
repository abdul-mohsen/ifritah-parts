package service

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB opens an in-memory SQLite handle purely to exercise the
// constructor's "db != nil" branch. The SQL queries in this file's peers
// target MySQL tables that don't exist here, so we never issue a query
// against this handle — we only need a non-nil *sql.DB to construct.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestTecDocCrossRefConstructorWithRealDB(t *testing.T) {
	svc := NewTecDocCrossRef(openTestDB(t))
	if svc == nil || svc.repo == nil {
		t.Fatalf("constructor with real DB must wire the repo")
	}
}

func TestTecDocSpecificationsConstructorWithRealDB(t *testing.T) {
	svc := NewTecDocSpecifications(openTestDB(t))
	if svc == nil || svc.repo == nil {
		t.Fatalf("constructor with real DB must wire the repo")
	}
}

func TestTecDocDocumentsConstructorWithRealDB(t *testing.T) {
	svc := NewTecDocDocuments(openTestDB(t))
	if svc == nil || svc.repo == nil {
		t.Fatalf("constructor with real DB must wire the repo")
	}
}

func TestTecDocSupersessionConstructorWithRealDB(t *testing.T) {
	svc := NewTecDocSupersession(openTestDB(t))
	if svc == nil || svc.repo == nil {
		t.Fatalf("constructor with real DB must wire the repo")
	}
}

func TestTecDocFunctionalConstructorWithRealDB(t *testing.T) {
	svc := NewTecDocFunctional(openTestDB(t))
	if svc == nil || svc.repo == nil {
		t.Fatalf("constructor with real DB must wire the repo")
	}
}

func TestTecDocVehicleConstructorWithRealDB(t *testing.T) {
	svc := NewTecDocVehicle(openTestDB(t))
	if svc == nil || svc.repo == nil {
		t.Fatalf("constructor with real DB must wire the repo")
	}
}

func TestTecDocCrossBrandConstructorWithRealDB(t *testing.T) {
	svc := NewTecDocCrossBrand(openTestDB(t))
	if svc == nil || svc.repo == nil {
		t.Fatalf("constructor with real DB must wire the repo")
	}
}
