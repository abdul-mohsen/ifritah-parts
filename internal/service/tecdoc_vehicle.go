package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"parts-engine/internal/model"
)

// vehicleFitmentRepo is the injectable DB dep for TecDocVehicle.
//
// Two query paths, in strict order of preference:
//
//  1. QueryCompatibleVehicles — primary. 4-way join
//     (articlesvehicletrees + linkagetargets + modelseries + manufacturers).
//     Returns richly-decorated rows with Make/Model/CategoryHint. Loses
//     rows when modelseries or manufacturers has a gap.
//
//  2. QueryCompatibleVehiclesFallback — M3.S2.T1 fallback. 2-way join
//     (articlesvehicletrees + linkagetargets only). No lang filter.
//     Broader coverage; Make/Model/CategoryHint may be empty.
//     See docs/data-sources/vehicle-fitment-fallback.md.
type vehicleFitmentRepo interface {
	QueryCompatibleVehicles(ctx context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error)
	QueryCompatibleVehiclesFallback(ctx context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error)
}

type compatibleVehicleRow struct {
	LinkageTargetId int
	VehicleName     string
	Make            string
	Model           string
	BeginYearMonth  int
	EndYearMonth    int
	FuelType        string
	CapacityCC      int
	HorsePower      int
	CategoryHint    string
}

// TecDocVehicle walks the articlesvehicletrees table (651M rows) to answer
// "which vehicles does this part fit?".
//
// The returned CompatibleVehicle values are annotated with a FitmentDriver
// derived from the category hint (assembly-group name) so downstream
// evidence rendering can pick the right badge without re-classifying.
type TecDocVehicle struct {
	repo vehicleFitmentRepo
}

func NewTecDocVehicle(db *sql.DB) *TecDocVehicle {
	if db == nil {
		return &TecDocVehicle{}
	}
	return &TecDocVehicle{repo: &sqlVehicleFitmentRepo{db: db}}
}

// FindCompatibleVehicles returns the vehicles the given article fits.
// limit is clamped to [1, 200] — the caller usually pages the result
// via a separate handler, this is not meant to hydrate the entire
// 651M-row table in one call.
//
// Fallback (M3.S2.T1): when the primary 4-way join returns zero rows,
// retry with a looser 2-way join against articlesvehicletrees +
// linkagetargets only. See docs/data-sources/vehicle-fitment-fallback.md
// for the schema comparison and why this widens coverage without
// regressing the fast-path.
//
// Error contract:
//   - primary DB error       → returned to caller (fail loud)
//   - primary rows > 0       → returned; fallback is NOT executed
//   - primary rows = 0       → fallback attempted
//   - fallback DB error      → logged as warning; return the empty
//     primary result (not an error — we don't want a fallback-only
//     failure to mask what is legitimately "no fitment")
func (s *TecDocVehicle) FindCompatibleVehicles(legacyArticleId, limit int) ([]model.CompatibleVehicle, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if legacyArticleId <= 0 {
		return nil, fmt.Errorf("invalid legacyArticleId: %d", legacyArticleId)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	ctx := context.Background()

	rows, err := s.repo.QueryCompatibleVehicles(ctx, legacyArticleId, limit)
	if err != nil {
		return nil, fmt.Errorf("find compatible vehicles: %w", err)
	}
	primary := s.buildResults(legacyArticleId, rows)
	if len(primary) > 0 {
		return primary, nil
	}

	// Primary is empty — attempt the articlesvehicletrees-only fallback.
	fbRows, fbErr := s.repo.QueryCompatibleVehiclesFallback(ctx, legacyArticleId, limit)
	if fbErr != nil {
		log.Printf("[vehicle_fitment] primary empty, fallback errored for legacyArticleId=%d: %v",
			legacyArticleId, fbErr)
		return primary, nil
	}
	fallback := s.buildResults(legacyArticleId, fbRows)
	log.Printf("[vehicle_fitment] primary empty, fallback returned %d rows for legacyArticleId=%d",
		len(fallback), legacyArticleId)
	return fallback, nil
}

// buildResults dedups compatibleVehicleRow entries by LinkageTargetId and
// projects them into the CompatibleVehicle model. Rows with
// LinkageTargetId == 0 or duplicate LinkageTargetId are dropped. The
// first-seen row per LinkageTargetId wins, so callers should ORDER BY
// their preferred-locale/preferred-year signal first.
func (s *TecDocVehicle) buildResults(legacyArticleId int, rows []compatibleVehicleRow) []model.CompatibleVehicle {
	out := make([]model.CompatibleVehicle, 0, len(rows))
	seen := map[int]bool{}
	for _, r := range rows {
		if r.LinkageTargetId == 0 || seen[r.LinkageTargetId] {
			continue
		}
		seen[r.LinkageTargetId] = true

		// M3.S2.T2: extract structured chassis + engine spec from the
		// raw description so the frontend can render "Tucson (TL) 2.0
		// CRDi 4WD" without re-parsing the label client-side.
		parsed := parseVehicleDescription(r.VehicleName)

		out = append(out, model.CompatibleVehicle{
			LegacyArticleId: legacyArticleId,
			LinkageTargetId: r.LinkageTargetId,
			VehicleName:     r.VehicleName,
			Make:            r.Make,
			Model:           r.Model,
			Chassis:         parsed.Chassis,
			EngineSpec:      parsed.EngineSpec,
			YearFrom:        yearFromYearMonth(r.BeginYearMonth),
			YearTo:          yearFromYearMonth(r.EndYearMonth),
			FuelType:        r.FuelType,
			CapacityCC:      r.CapacityCC,
			HorsePower:      r.HorsePower,
			FitmentDriver:   driverName(ClassifyCategory(r.CategoryHint).Driver),
		})
	}
	return out
}

// yearFromYearMonth extracts the 4-digit year from a TecDoc YYYYMM integer
// (e.g. 201503). 0 → 0 to preserve "unknown" semantics.
func yearFromYearMonth(ym int) int {
	if ym < 100 {
		return 0
	}
	return ym / 100
}

// sqlVehicleFitmentRepo is the production repo bound to MySQL.
type sqlVehicleFitmentRepo struct {
	db *sql.DB
}

func (r *sqlVehicleFitmentRepo) QueryCompatibleVehicles(ctx context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error) {
	const q = `
		SELECT DISTINCT
			avt.linkingTargetId,
			COALESCE(lt.description, ''),
			COALESCE(m.manuName, ''),
			COALESCE(ms.modelname, ''),
			COALESCE(lt.beginYearMonth, 0),
			COALESCE(lt.endYearMonth, 0),
			COALESCE(lt.fuelType, ''),
			COALESCE(lt.capacityCC, 0),
			COALESCE(lt.horsePowerFrom, 0),
			COALESCE(agn.assemblyGroupName, '')
		FROM articlesvehicletrees avt
		JOIN linkagetargets lt ON lt.linkageTargetId = avt.linkingTargetId AND lt.lang = 'en'
		JOIN modelseries ms ON ms.modelId = lt.vehicleModelSeriesId
		JOIN manufacturers m ON m.manuId = ms.manuId
		LEFT JOIN assemblygroupnodenames agn ON agn.assemblyGroupNodeId = avt.assemblyGroupNodeId AND agn.lang = 'en'
		WHERE avt.legacyArticleId = ?
		  AND avt.linkingTargetType = 'P'
		ORDER BY m.manuName, ms.modelname, lt.beginYearMonth
		LIMIT ?`

	rows, err := logQueryCtx(r.db, ctx, "TecDocVehicle.FindCompatibleVehicles", q, legacyArticleId, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []compatibleVehicleRow
	for rows.Next() {
		var row compatibleVehicleRow
		if err := rows.Scan(
			&row.LinkageTargetId,
			&row.VehicleName,
			&row.Make,
			&row.Model,
			&row.BeginYearMonth,
			&row.EndYearMonth,
			&row.FuelType,
			&row.CapacityCC,
			&row.HorsePower,
			&row.CategoryHint,
		); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

// QueryCompatibleVehiclesFallback runs the 2-way articlesvehicletrees +
// linkagetargets query when the primary 4-way join finds nothing. Drops
// the modelseries + manufacturers inner joins and the lang='en' filter
// so rows survive when either denormalization side has a gap.
//
// Trade-off: Make / Model / CategoryHint come back empty because we
// removed the tables that populated them. The description in
// VehicleName still starts with "HYUNDAI TUCSON (TL) …" so
// parseVehicleDescription can still extract Chassis + EngineSpec, and
// downstream renderers can display the raw description as fallback.
//
// The ORDER BY `(lt.lang = 'en') DESC` prefix makes the English row
// sort first when it exists so buildResults' first-seen dedup keeps the
// English label; a non-English description is only surfaced when
// English is genuinely absent for that linkage target.
//
// See docs/data-sources/vehicle-fitment-fallback.md.
func (r *sqlVehicleFitmentRepo) QueryCompatibleVehiclesFallback(ctx context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error) {
	const q = `
		SELECT
			avt.linkingTargetId,
			COALESCE(lt.description, ''),
			COALESCE(lt.beginYearMonth, 0),
			COALESCE(lt.endYearMonth, 0),
			COALESCE(lt.fuelType, ''),
			COALESCE(lt.capacityCC, 0),
			COALESCE(lt.horsePowerFrom, 0)
		FROM articlesvehicletrees avt
		JOIN linkagetargets lt ON lt.linkageTargetId = avt.linkingTargetId
		WHERE avt.legacyArticleId = ?
		  AND avt.linkingTargetType = 'P'
		ORDER BY (lt.lang = 'en') DESC, lt.beginYearMonth DESC, avt.linkingTargetId
		LIMIT ?`

	rows, err := logQueryCtx(r.db, ctx, "TecDocVehicle.FindCompatibleVehicles.fallback", q, legacyArticleId, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []compatibleVehicleRow
	for rows.Next() {
		var row compatibleVehicleRow
		if err := rows.Scan(
			&row.LinkageTargetId,
			&row.VehicleName,
			&row.BeginYearMonth,
			&row.EndYearMonth,
			&row.FuelType,
			&row.CapacityCC,
			&row.HorsePower,
		); err != nil {
			continue
		}
		// Make, Model, CategoryHint remain empty — not available from
		// the 2-way join. Consumers get raw VehicleName and can parse.
		out = append(out, row)
	}
	return out, nil
}
