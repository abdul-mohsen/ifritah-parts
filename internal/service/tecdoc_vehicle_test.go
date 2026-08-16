package service

import (
	"context"
	"errors"
	"testing"
)

type stubVehicleFitmentRepo struct {
	rows      []compatibleVehicleRow
	err       error
	lastId    int
	lastLimit int
	callCount int
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

func TestTecDocVehicleFindCompatibleVehicles(t *testing.T) {
	repo := &stubVehicleFitmentRepo{
		rows: []compatibleVehicleRow{
			{
				LinkageTargetId: 12345,
				VehicleName:     "TUCSON 2.0 CRDi 4WD",
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
				VehicleName:     "SPORTAGE 2.0 CRDi 4WD",
				Make:            "Kia",
				Model:           "SPORTAGE",
				BeginYearMonth:  201603,
				EndYearMonth:    0,
				FuelType:        "Diesel",
				CapacityCC:      1995,
				HorsePower:      136,
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
	svc := &TecDocVehicle{repo: &stubVehicleFitmentRepo{err: errors.New("db")}}
	if _, err := svc.FindCompatibleVehicles(1, 50); err == nil {
		t.Fatalf("expected repo error to surface")
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
