package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// SQLite doesn't have Postgres advisory locks or the INTERVAL type. To
// unit-test the derive worker's control flow (nil-MySQL no-op, cadence
// check, context cancellation) we substitute enough stubs.
//
// End-to-end derivation is exercised against a real TecDoc MySQL + real
// Postgres in the CI integration job — see .github/workflows/*.

func TestDeriveWorker_NilPgIsNoOp(t *testing.T) {
	w := NewDeriveWorker(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled
	// Should return immediately without panic
	w.Start(ctx)
	if err := w.RunOnce(ctx); err == nil {
		t.Error("RunOnce on nil pg should return an error")
	}
}

func TestDeriveWorker_NilMysqlLogsAndReturns(t *testing.T) {
	// Non-nil pg (sqlite stand-in), nil mysql — Start should log + return
	// without launching the goroutine.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w := NewDeriveWorker(db, nil)
	// Should NOT panic and should NOT hang.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	select {
	case <-done:
		// good — Start returned immediately
	case <-time.After(500 * time.Millisecond):
		t.Error("Start should have returned immediately when mysql is nil")
	}
}

func TestDeriveWorker_SetForceAndInterval(t *testing.T) {
	w := NewDeriveWorker(nil, nil)
	w.SetForce(true)
	if !w.force {
		t.Error("SetForce(true) should set force flag")
	}
	w.SetInterval(1 * time.Hour)
	if w.interval != 1*time.Hour {
		t.Errorf("SetInterval failed: %v", w.interval)
	}
	// SetInterval(0) should NOT reset to zero — protective default.
	w.SetInterval(0)
	if w.interval != 1*time.Hour {
		t.Error("SetInterval(0) should not zero the interval")
	}
	// Nil receiver safety
	var wn *DeriveWorker
	wn.SetForce(true)      // must not panic
	wn.SetInterval(1 * time.Second) // must not panic
}

func TestClassifyDeriveDescription_Categories(t *testing.T) {
	cases := []struct {
		desc     string
		wantCat  string
		wantSys  string
	}{
		{"Oil Filter, engine", "Oil Filter", "Engine"},
		{"Air Filter", "Air Filter", "Engine"},
		{"Fuel Filter", "Fuel Filter", "Engine"},
		{"Cabin Air Filter", "Cabin Air Filter", "HVAC"},
		{"Brake Pad Set, disc brake", "Brake Pad Set", "Brakes"},
		{"Brake Disc, ventilated", "Brake Disc", "Brakes"},
		{"Shock Absorber, front axle", "Shock Absorber / Strut", "Suspension"},
		{"Front Power Window Motor Assembly", "Power Window Motor", "Body"},
		{"Radiator, engine cooling", "Radiator", "Cooling"},
		{"Ignition Coil", "Ignition Coil", "Engine"},
		{"Oxygen Sensor / Lambda", "Oxygen Sensor", "Electrical"},
		{"unknown widget", "unknown widget", ""},
	}
	for _, c := range cases {
		gotCat, gotSys := classifyDeriveDescription(c.desc)
		if gotCat != c.wantCat || gotSys != c.wantSys {
			t.Errorf("classifyDeriveDescription(%q) = (%q,%q), want (%q,%q)",
				c.desc, gotCat, gotSys, c.wantCat, c.wantSys)
		}
	}
}

func TestNormalizeDeriveMake(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"HYUNDAI", "Hyundai"},
		{"HYUNDAI MOTOR", "Hyundai"},
		{"KIA", "Kia"},
		{"KIA MOTORS", "Kia"},
		{"GENESIS", "Genesis"},
		{"BMW", "BMW"}, // unknown stays as-is
		{"  kia  ", "Kia"},
	}
	for _, c := range cases {
		if got := normalizeDeriveMake(c.in); got != c.want {
			t.Errorf("normalizeDeriveMake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
