package service

import (
	"context"
	"errors"
	"testing"
)

// stubVehicleFitmentRepo implements the two-method vehicleFitmentRepo
// interface. Both primary and fallback share the same call-recording
// pattern so tests can assert fallback-was/was-not called.
type stubVehicleFitmentRepo struct {
	// Primary path
	rows      []compatibleVehicleRow
	err       error
	lastId    int
	lastLimit int
	callCount int

	// Fallback path (M3.S2.T1)
	fbRows      []compatibleVehicleRow
	fbErr       error
	fbLastId    int
	fbLastLimit int
	fbCallCount int
}

func (s *stubVehicleFitmentRepo) QueryCompatibleVehicles(_ context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error) {
	s.callCount++
	s.lastId = legacyArticleId
	s.lastLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func (s *stubVehicleFitmentRepo) QueryCompatibleVehiclesFallback(_ context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error) {
	s.fbCallCount++
	s.fbLastId = legacyArticleId
	s.fbLastLimit = limit
	if s.fbErr != nil {
		return nil, s.fbErr
	}
	return s.fbRows, nil
}

func TestTecDocVehicleFindCompatibleVehicles(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows: []compatibleVehicleRow{
			{
				LinkageTargetId: 12345,
				VehicleName:     "HYUNDAI TUCSON (TL) 2.0 CRDi 4WD 136HP [08.2015-]",
				Make:            "Hyundai",
				Model:           "TUCSON",
				BeginYearMonth:  201503,
				EndYearMonth:    202012,
				FuelType:        "Diesel",
				CapacityCC:      1995,
				HorsePower:      136,
				CategoryHint:    "Oil filter",
			},
			{
				LinkageTargetId: 12346,
				VehicleName:     "KIA SPORTAGE IV (QL) 1.6 T-GDI 177HP [07.2018-]",
				Make:            "Kia",
				Model:           "SPORTAGE",
				BeginYearMonth:  201603,
				EndYearMonth:    0,
				FuelType:        "Petrol",
				CapacityCC:      1591,
				HorsePower:      177,
				CategoryHint:    "Oil filter",
			},
		},
	}
	svc := &TecDocVehicle{repo: repo}
	vehicles, err := svc.FindCompatibleVehicles(100001, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vehicles) != 2 {
		t.Fatalf("expected 2 vehicles, got %d", len(vehicles))
	}
	if vehicles[0].Make != "Hyundai" || vehicles[0].YearFrom != 2015 || vehicles[0].YearTo != 2020 {
		t.Fatalf("first vehicle year extraction failed: %+v", vehicles[0])
	}
	if vehicles[1].YearTo != 0 {
		t.Fatalf("expected YearTo=0 for zero endYearMonth, got %d", vehicles[1].YearTo)
	}
	if vehicles[0].LegacyArticleId != 100001 {
		t.Fatalf("expected legacyArticleId stamped from query arg, got %d", vehicles[0].LegacyArticleId)
	}
	if vehicles[0].FitmentDriver == "" {
		t.Fatalf("FitmentDriver should be classified from category hint")
	}
	// M3.S2.T2: description parser must populate Chassis + EngineSpec, VehicleName untouched.
	if vehicles[0].VehicleName != "HYUNDAI TUCSON (TL) 2.0 CRDi 4WD 136HP [08.2015-]" {
		t.Fatalf("VehicleName must be preserved verbatim, got %q", vehicles[0].VehicleName)
	}
	if vehicles[0].Chassis != "TL" {
		t.Fatalf("expected Chassis=TL parsed from description, got %q", vehicles[0].Chassis)
	}
	if vehicles[0].EngineSpec != "2.0 CRDi 4WD 136HP" {
		t.Fatalf("expected EngineSpec=%q, got %q", "2.0 CRDi 4WD 136HP", vehicles[0].EngineSpec)
	}
	if vehicles[1].Chassis != "QL" {
		t.Fatalf("expected Chassis=QL for Sportage IV row, got %q", vehicles[1].Chassis)
	}
	if vehicles[1].EngineSpec != "1.6 T-GDI 177HP" {
		t.Fatalf("expected EngineSpec=%q for Sportage IV row, got %q", "1.6 T-GDI 177HP", vehicles[1].EngineSpec)
	}
	// M3.S2.T1: fallback must NOT be called when primary returns rows.
	if repo.fbCallCount != 0 {
		t.Fatalf("fallback must NOT be called when primary returns rows, got fbCallCount=%d", repo.fbCallCount)
	}
}

func TestTecDocVehicleDeduplicatesByLinkage(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows: []compatibleVehicleRow{
			{LinkageTargetId: 42, VehicleName: "A", Make: "H"},
			{LinkageTargetId: 42, VehicleName: "A-dup", Make: "H"}, // dup
			{LinkageTargetId: 43, VehicleName: "B", Make: "K"},
			{LinkageTargetId: 0, VehicleName: "C", Make: "?"}, // dropped
		},
	}
	svc := &TecDocVehicle{repo: repo}
	vs, err := svc.FindCompatibleVehicles(1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 2 {
		t.Fatalf("expected 2 vehicles after dedup, got %d: %+v", len(vs), vs)
	}
}

func TestTecDocVehicleInvalidId(t *testing.T) {
	svc := &TecDocVehicle{repo: &stubVehicleFitmentRepo{}}
	if _, err := svc.FindCompatibleVehicles(0, 50); err == nil {
		t.Fatalf("expected error for zero id")
	}
	if _, err := svc.FindCompatibleVehicles(-1, 50); err == nil {
		t.Fatalf("expected error for negative id")
	}
}

func TestTecDocVehicleRepoError(t *testing.T) {
	repo := &stubVehicleFitmentRepo{err: errors.New("db")}
	svc := &TecDocVehicle{repo: repo}
	if _, err := svc.FindCompatibleVehicles(1, 50); err == nil {
		t.Fatalf("expected repo error to surface")
	}
	if repo.fbCallCount != 0 {
		t.Fatalf("fallback must NOT be called when primary errors, got fbCallCount=%d", repo.fbCallCount)
	}
}

func TestTecDocVehicleNilRepo(t *testing.T) {
	svc := &TecDocVehicle{}
	if _, err := svc.FindCompatibleVehicles(1, 50); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocVehicleNilDBConstructor(t *testing.T) {
	svc := NewTecDocVehicle(nil)
	if svc == nil {
		t.Fatalf("NewTecDocVehicle(nil) must not return nil")
	}
	if _, err := svc.FindCompatibleVehicles(1, 50); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocVehicleLimitClamp(t *testing.T) {
	repo := &stubVehicleFitmentRepo{}
	svc := &TecDocVehicle{repo: repo}
	_, _ = svc.FindCompatibleVehicles(1, 0)
	if repo.lastLimit != 50 {
		t.Fatalf("expected zero limit to clamp to 50, got %d", repo.lastLimit)
	}
	_, _ = svc.FindCompatibleVehicles(1, 5000)
	if repo.lastLimit != 50 {
		t.Fatalf("expected oversized limit to clamp to 50, got %d", repo.lastLimit)
	}
	_, _ = svc.FindCompatibleVehicles(1, 75)
	if repo.lastLimit != 75 {
		t.Fatalf("expected in-range limit to pass through, got %d", repo.lastLimit)
	}
}

func TestYearFromYearMonth(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{201503, 2015},
		{202012, 2020},
		{0, 0},
		{99, 0},
		{100, 1},
		{197001, 1970},
	}
	for _, c := range cases {
		if got := yearFromYearMonth(c.in); got != c.want {
			t.Errorf("yearFromYearMonth(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// M3.S2.T1 — articlesvehicletrees fallback
// ---------------------------------------------------------------------------

// TestTecDocVehicleFallback_PrimaryHit_FallbackNotCalled explicitly guards
// the fast-path contract: when the primary query returns any rows, the
// fallback query must NEVER execute. This is the throughput invariant —
// production traffic must not incur the fallback join on the majority of
// requests that are perfectly served by the primary.
func TestTecDocVehicleFallback_PrimaryHit_FallbackNotCalled(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows: []compatibleVehicleRow{
			{LinkageTargetId: 100, VehicleName: "HYUNDAI TUCSON (TL) 2.0", Make: "Hyundai", Model: "TUCSON"},
		},
		fbRows: []compatibleVehicleRow{
			// Fallback has different data — if this leaked into the
			// result, we'd know the fast-path guard failed.
			{LinkageTargetId: 999, VehicleName: "SHOULD NOT APPEAR"},
		},
	}
	svc := &TecDocVehicle{repo: repo}
	vs, err := svc.FindCompatibleVehicles(1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 1 || vs[0].LinkageTargetId != 100 {
		t.Fatalf("expected primary result [100], got %+v", vs)
	}
	if repo.callCount != 1 {
		t.Fatalf("expected primary called exactly once, got %d", repo.callCount)
	}
	if repo.fbCallCount != 0 {
		t.Fatalf("fallback must NOT be called when primary hits, got fbCallCount=%d", repo.fbCallCount)
	}
}

// TestTecDocVehicleFallback_PrimaryEmpty_FallbackHit covers the coverage
// win: primary 4-way join finds zero, but the 2-way fallback finds real
// linkages. The fallback rows may have empty Make/Model (that's the
// point — those fields need modelseries+manufacturers joins) but Chassis
// and EngineSpec still parse from the description.
func TestTecDocVehicleFallback_PrimaryEmpty_FallbackHit(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows: nil, // primary returns nothing
		fbRows: []compatibleVehicleRow{
			{
				LinkageTargetId: 55555,
				VehicleName:     "HYUNDAI TUCSON (TL) 2.0 CRDi 4WD 136HP [08.2015-]",
				// Make/Model deliberately empty (fallback can't
				// denormalize them without the joins we skipped).
				BeginYearMonth: 201508,
				EndYearMonth:   0,
				FuelType:       "Diesel",
				CapacityCC:     1995,
				HorsePower:     136,
			},
		},
	}
	svc := &TecDocVehicle{repo: repo}
	vs, err := svc.FindCompatibleVehicles(100001, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.callCount != 1 || repo.fbCallCount != 1 {
		t.Fatalf("expected primary+fallback each called once, got primary=%d fallback=%d",
			repo.callCount, repo.fbCallCount)
	}
	if len(vs) != 1 {
		t.Fatalf("expected 1 vehicle from fallback, got %d", len(vs))
	}
	v := vs[0]
	if v.LinkageTargetId != 55555 {
		t.Errorf("linkage target id: got %d, want 55555", v.LinkageTargetId)
	}
	if v.LegacyArticleId != 100001 {
		t.Errorf("legacyArticleId stamp: got %d, want 100001", v.LegacyArticleId)
	}
	if v.YearFrom != 2015 {
		t.Errorf("YearFrom parse: got %d, want 2015", v.YearFrom)
	}
	if v.YearTo != 0 {
		t.Errorf("YearTo when endYearMonth=0: got %d, want 0", v.YearTo)
	}
	if v.FuelType != "Diesel" || v.CapacityCC != 1995 || v.HorsePower != 136 {
		t.Errorf("fuel/cc/hp not carried through: %+v", v)
	}
	if v.Chassis != "TL" {
		t.Errorf("Chassis parse from description: got %q, want %q", v.Chassis, "TL")
	}
	if v.EngineSpec == "" {
		t.Errorf("EngineSpec should be non-empty after parseVehicleDescription")
	}
	// Contract: fallback path leaves Make/Model empty — verifies we
	// really skipped the denormalization joins.
	if v.Make != "" || v.Model != "" {
		t.Errorf("fallback path expected empty Make/Model, got Make=%q Model=%q", v.Make, v.Model)
	}
	// Contract: id + limit are forwarded to the fallback repo call so
	// production wiring produces the right SQL args.
	if repo.fbLastId != 100001 || repo.fbLastLimit != 50 {
		t.Errorf("fallback got wrong args: id=%d limit=%d", repo.fbLastId, repo.fbLastLimit)
	}
}

// TestTecDocVehicleFallback_BothEmpty covers the legitimate "no fitment"
// case — some articles simply have no vehicle linkages in TecDoc. Both
// query paths return empty, we return an empty slice with no error.
// This is critical: the vehicle_fitment strategy must be able to
// distinguish "no fitment" from "database error" via the (nil, nil) shape.
func TestTecDocVehicleFallback_BothEmpty(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows:   nil,
		fbRows: nil,
	}
	svc := &TecDocVehicle{repo: repo}
	vs, err := svc.FindCompatibleVehicles(1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected 0 vehicles, got %d: %+v", len(vs), vs)
	}
	if repo.callCount != 1 || repo.fbCallCount != 1 {
		t.Fatalf("expected primary+fallback each called once, got primary=%d fallback=%d",
			repo.callCount, repo.fbCallCount)
	}
}

// TestTecDocVehicleFallback_PrimaryError_FallbackNotCalled: a primary DB
// error is a real system fault (connection lost, table gone). It must
// surface to the caller; the fallback path is NOT a recovery mechanism
// for DB failure, only for schema-gap coverage. If we called fallback
// after a primary error, we'd hide the fault AND double the load on the
// same broken DB.
func TestTecDocVehicleFallback_PrimaryError_FallbackNotCalled(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		err:    errors.New("primary connection reset"),
		fbRows: []compatibleVehicleRow{{LinkageTargetId: 1, VehicleName: "MUST NOT APPEAR"}},
	}
	svc := &TecDocVehicle{repo: repo}
	vs, err := svc.FindCompatibleVehicles(1, 50)
	if err == nil {
		t.Fatalf("expected primary error to surface, got nil")
	}
	if vs != nil {
		t.Fatalf("expected nil result on error, got %+v", vs)
	}
	if repo.fbCallCount != 0 {
		t.Fatalf("fallback must NOT run when primary errors, got fbCallCount=%d", repo.fbCallCount)
	}
}

// TestTecDocVehicleFallback_FallbackError_ReturnsEmpty covers the
// graceful-degradation invariant: if the fallback query itself fails
// after primary was empty, we log a warning and return an empty
// (nil-error) result. Reason: primary already gave us "no fitment" —
// a fallback-only error should not upgrade that to a hard failure
// downstream. This is intentional asymmetry; primary errors fail loud,
// fallback errors fail quiet.
func TestTecDocVehicleFallback_FallbackError_ReturnsEmpty(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows:  nil, // primary empty (no error)
		fbErr: errors.New("fallback timeout"),
	}
	svc := &TecDocVehicle{repo: repo}
	vs, err := svc.FindCompatibleVehicles(1, 50)
	if err != nil {
		t.Fatalf("fallback error must NOT surface, got %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected empty result, got %+v", vs)
	}
	if repo.fbCallCount != 1 {
		t.Fatalf("fallback should have been attempted once, got %d", repo.fbCallCount)
	}
}

// TestTecDocVehicleFallback_DedupAndFieldMapping covers the fallback-side
// dedup + description-parsing pipeline. Fallback rows are already
// ORDER BY'd server-side to prefer English descriptions, but the Go
// dedup layer must still collapse duplicate LinkageTargetIds and drop
// zero ids. Also verifies YearFrom / YearTo / FuelType / Chassis /
// EngineSpec parse through the fallback path the same way they do on
// primary.
func TestTecDocVehicleFallback_DedupAndFieldMapping(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows: nil,
		fbRows: []compatibleVehicleRow{
			// English row surfaced first by server ORDER BY.
			{
				LinkageTargetId: 200,
				VehicleName:     "KIA SORENTO (XM) 2.4 GDi AWD 189HP [05.2012-06.2020]",
				BeginYearMonth:  201205,
				EndYearMonth:    202006,
				FuelType:        "Petrol",
				CapacityCC:      2400,
				HorsePower:      189,
			},
			// Same linkage id, German language row — must be dedup'd.
			{
				LinkageTargetId: 200,
				VehicleName:     "KIA SORENTO (XM) 2.4 GDi Allrad [05.2012-06.2020]",
			},
			// Zero-id row — must be dropped.
			{LinkageTargetId: 0, VehicleName: "junk"},
			// Distinct second linkage id — kept.
			{
				LinkageTargetId: 201,
				VehicleName:     "HYUNDAI ELANTRA 1.6 [01.2011-]",
				BeginYearMonth:  201101,
				EndYearMonth:    0,
				FuelType:        "Petrol",
			},
		},
	}
	svc := &TecDocVehicle{repo: repo}
	vs, err := svc.FindCompatibleVehicles(1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 2 {
		t.Fatalf("expected 2 unique vehicles after dedup + zero-drop, got %d: %+v", len(vs), vs)
	}
	// First survivor is the English SORENTO row (server ordering
	// preserved through Go dedup).
	if vs[0].LinkageTargetId != 200 || vs[0].Chassis != "XM" {
		t.Errorf("expected English SORENTO row first, got %+v", vs[0])
	}
	if vs[0].YearFrom != 2012 || vs[0].YearTo != 2020 {
		t.Errorf("year range: got %d-%d, want 2012-2020", vs[0].YearFrom, vs[0].YearTo)
	}
	if vs[1].LinkageTargetId != 201 || vs[1].YearTo != 0 {
		t.Errorf("expected ELANTRA row second with open-ended year, got %+v", vs[1])
	}
}
