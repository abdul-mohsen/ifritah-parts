package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"parts-engine/internal/model"
)

// PrefixInference synthesizes OEM-part descriptions from three Postgres
// lookup tables when no direct data source has an answer:
//
//	hk_oem_prefix_map      — 5-digit prefix → part family + description
//	hk_chassis_code_map    — 2-char code → make/model/year range
//	hk_variant_suffix_map  — 3-char suffix → position/side
//
// The tables are populated by (a) a hand-curated seed migration
// (000011_create_hk_prefix_inference_tables.sql, Path B fallback) and
// (b) scripts/derive_hk_maps/main.go which auto-derives entries from
// TecDoc clustering when TecDoc MySQL is reachable (Path A primary).
//
// Design principle: never call the network. All lookups are Postgres index
// reads (~1 ms). Confidence is bounded by min(prefix, chassis, suffix)
// confidences with a slight penalty when the chassis code is unknown.
//
// A miss (no prefix match) returns nil — callers fall through to the
// existing search cascade.
type PrefixInference struct {
	db *sql.DB
}

// NewPrefixInference wires up the service. Returns nil when db is nil so
// callers can no-op the strategy without extra guards.
func NewPrefixInference(db *sql.DB) *PrefixInference {
	if db == nil {
		return nil
	}
	return &PrefixInference{db: db}
}

// hkOEMPartRegex matches the canonical Hyundai/Kia OEM format:
//
//	5 digits, optional dash, 5 alphanumerics.
//
// Examples that match: "82460-2T010", "82460 2T010", "824602T010".
var hkOEMPartRegex = regexp.MustCompile(`^([0-9]{5})[- ]?([A-Z0-9]{5})$`)

// Synthesize returns a SmartResult synthesized from prefix/chassis/suffix
// tables, or nil when the OEM does not match the HK format or when no
// prefix data is available for it.
//
// Never touches the network. Bounded runtime ~5 ms (three PK lookups).
func (p *PrefixInference) Synthesize(ctx context.Context, rawOEM string) *SmartResult {
	if p == nil || p.db == nil {
		return nil
	}
	upper := strings.ToUpper(strings.TrimSpace(rawOEM))
	// Strip common separators so the regex sees the canonical form.
	stripped := strings.NewReplacer(".", "", "/", "").Replace(upper)
	m := hkOEMPartRegex.FindStringSubmatch(stripped)
	if m == nil {
		return nil
	}
	prefix := m[1]                                // 5 digits
	suffixFull := m[2]                            // 5 alphanumerics
	chassis := suffixFull[:2]                     // first 2 = chassis code
	variant := suffixFull[2:]                     // last 3 = variant/position

	// Prefix lookup: no prefix, no synthesis.
	prefEntry, err := p.lookupPrefix(ctx, prefix)
	if err != nil || prefEntry == nil {
		return nil
	}
	// Chassis lookup: MAY miss without disqualifying the answer.
	chassisEntry, _ := p.lookupChassis(ctx, chassis)
	// Variant lookup: MAY miss without disqualifying the answer.
	variantEntry, _ := p.lookupVariant(ctx, variant)

	description := prefEntry.Description
	if variantEntry != nil && variantEntry.Position != "" {
		description = fmt.Sprintf("%s (%s)", prefEntry.Description, variantEntry.Position)
	}

	if chassisEntry != nil {
		description = fmt.Sprintf("%s for %s %s", description, chassisEntry.Make, chassisEntry.Model)
		if chassisEntry.YearStart > 0 {
			if chassisEntry.YearEnd > 0 && chassisEntry.YearEnd != chassisEntry.YearStart {
				description = fmt.Sprintf("%s (%d-%d)", description, chassisEntry.YearStart, chassisEntry.YearEnd)
			} else {
				description = fmt.Sprintf("%s (%d+)", description, chassisEntry.YearStart)
			}
		}
	}

	// Composite confidence: prefix is the required floor. Chassis and suffix
	// boost or dampen. We never claim > 0.90 from pure inference — a live
	// source (dealer_lookup, TecDoc) with corroboration is what pushes past.
	confidence := prefEntry.Confidence
	if chassisEntry == nil {
		confidence -= 0.10 // unknown chassis is a real information deficit
	}
	if variantEntry == nil {
		confidence -= 0.03 // minor
	}
	if confidence < 0.50 {
		confidence = 0.50
	}
	if confidence > 0.90 {
		confidence = 0.90
	}

	note := "Inferred from OEM number pattern (prefix + chassis code)"
	if chassisEntry == nil {
		note = "Inferred from OEM prefix — vehicle unknown for chassis code " + chassis
	}

	make := ""
	if chassisEntry != nil {
		make = chassisEntry.Make
	} else {
		make = "Hyundai / KIA"
	}

	return &SmartResult{
		Part: model.Part{
			ArticleNumber: upper,
			Description:   description,
			BrandName:     make,
			Category:      prefEntry.System + " / " + prefEntry.Category,
		},
		Confidence:     confidence,
		ConfidenceNote: note,
		FitmentDriver:  "inferred",
		BrandResolved:  make,
		SourceStrategy: "prefix_inference",
	}
}

type prefixEntry struct {
	Prefix      string
	System      string
	Category    string
	Description string
	Confidence  float64
	Source      string
}

type chassisEntry struct {
	ChassisCode string
	Make        string
	Model       string
	Platform    sql.NullString
	YearStart   int
	YearEnd     int
	Confidence  float64
	Source      string
}

type variantEntry struct {
	Suffix   string
	Position string
	Side     string
	Note     sql.NullString
}

func (p *PrefixInference) lookupPrefix(ctx context.Context, prefix string) (*prefixEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	var e prefixEntry
	err := p.db.QueryRowContext(ctx,
		`SELECT prefix, system, category, description, confidence, source
		 FROM hk_oem_prefix_map WHERE prefix = $1 LIMIT 1`, prefix).
		Scan(&e.Prefix, &e.System, &e.Category, &e.Description, &e.Confidence, &e.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		log.Printf("[PrefixInference.lookupPrefix] prefix=%s err=%v", prefix, err)
		return nil, err
	}
	return &e, nil
}

func (p *PrefixInference) lookupChassis(ctx context.Context, code string) (*chassisEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	var e chassisEntry
	var yEnd sql.NullInt32
	err := p.db.QueryRowContext(ctx,
		`SELECT chassis_code, make, model, platform, year_start, year_end, confidence, source
		 FROM hk_chassis_code_map WHERE chassis_code = $1 LIMIT 1`, code).
		Scan(&e.ChassisCode, &e.Make, &e.Model, &e.Platform, &e.YearStart, &yEnd, &e.Confidence, &e.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if yEnd.Valid {
		e.YearEnd = int(yEnd.Int32)
	}
	return &e, nil
}

func (p *PrefixInference) lookupVariant(ctx context.Context, suffix string) (*variantEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	var e variantEntry
	var pos, side sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT suffix, position, side, variant_note
		 FROM hk_variant_suffix_map WHERE suffix = $1 LIMIT 1`, suffix).
		Scan(&e.Suffix, &pos, &side, &e.Note)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Position = pos.String
	e.Side = side.String
	return &e, nil
}

// Stats returns counts across all three tables for debugging / admin. Used
// by the diagnostics endpoint to show operators whether TecDoc-derived
// entries are landing (Path A worked) or only seed baseline is in place.
type PrefixInferenceStats struct {
	Prefixes struct {
		Total   int `json:"total"`
		Derived int `json:"derived"`
		Seed    int `json:"seed"`
	} `json:"prefixes"`
	ChassisCodes struct {
		Total   int `json:"total"`
		Derived int `json:"derived"`
		Seed    int `json:"seed"`
	} `json:"chassisCodes"`
	Variants int `json:"variants"`
}

func (p *PrefixInference) Stats(ctx context.Context) (*PrefixInferenceStats, error) {
	if p == nil || p.db == nil {
		return &PrefixInferenceStats{}, nil
	}
	var s PrefixInferenceStats
	// NOTE: no `::int` casts here — Postgres auto-coerces COUNT(*) return type
	// and SQLite (used in tests) rejects the explicit cast. Go's Scan into
	// int handles both backends transparently.
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COUNT(*) FILTER (WHERE source = 'tecdoc_derived'),
		        COUNT(*) FILTER (WHERE source = 'seed')
		 FROM hk_oem_prefix_map`).
		Scan(&s.Prefixes.Total, &s.Prefixes.Derived, &s.Prefixes.Seed); err != nil {
		return nil, err
	}
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COUNT(*) FILTER (WHERE source = 'tecdoc_derived'),
		        COUNT(*) FILTER (WHERE source = 'seed')
		 FROM hk_chassis_code_map`).
		Scan(&s.ChassisCodes.Total, &s.ChassisCodes.Derived, &s.ChassisCodes.Seed); err != nil {
		return nil, err
	}
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM hk_variant_suffix_map`).
		Scan(&s.Variants); err != nil {
		return nil, err
	}
	return &s, nil
}
