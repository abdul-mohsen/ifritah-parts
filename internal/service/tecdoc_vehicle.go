package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"parts-engine/internal/model"
)

// vehicleFitmentRepo is the injectable DB dep for TecDocVehicle.
//
// Three query paths, in strict order of preference:
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
//
//  3. QueryCompatibleVehiclesBatch — M3.S1.T2 batch. Same 4-way join as
//     path 1 but WHERE avt.legacyArticleId IN (?,?,?). Used by enrichResults
//     to collapse N per-result DB round-trips into one. No fallback path
//     at the batch layer (per-result FindCompatibleVehicles fills gaps).
type vehicleFitmentRepo interface {
	QueryCompatibleVehicles(ctx context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error)
	QueryCompatibleVehiclesFallback(ctx context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error)
	QueryCompatibleVehiclesBatch(ctx context.Context, legacyArticleIds []int, limitPerId int) (map[int][]compatibleVehicleRow, error)
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

// FindCompatibleVehiclesBatch returns compatible-vehicle rows for MANY article
// ids in one SQL round-trip. Used by enrichResults (M3.S1.T2) to cut the N+1
// query fan-out on the vehicle path — for a 20-result response the batch
// version is one IN-list query instead of twenty separate ones.
//
// Runs the primary 4-way join only. Rows are keyed by legacyArticleId; ids
// with no rows are absent from the map (caller distinguishes from nil-value).
// Ids <= 0 and duplicates are dropped from the query set. Caps len(ids) at
// 100 to bound query cost; caller should chunk larger sets.
//
// No fallback path in the batch call. When a specific id returns no rows,
// the caller can still fall back to per-result FindCompatibleVehicles which
// runs the M3.S2.T1 2-way fallback. Doing the fallback here would require
// tracking per-id emptiness inside a single map result, and the observed
// coverage lift (audit log line
//
//	"[vehicle_fitment] primary empty, fallback returned N rows for legacyArticleId=X")
//
// would be lost. Batch-first, per-result-fallback keeps both concerns pure.
func (s *TecDocVehicle) FindCompatibleVehiclesBatch(legacyArticleIds []int, limitPerId int) (map[int][]model.CompatibleVehicle, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if len(legacyArticleIds) == 0 {
		return map[int][]model.CompatibleVehicle{}, nil
	}
	if limitPerId <= 0 || limitPerId > 200 {
		limitPerId = 50
	}
	// Dedupe zero + duplicates
	seen := make(map[int]bool, len(legacyArticleIds))
	ids := make([]int, 0, len(legacyArticleIds))
	for _, id := range legacyArticleIds {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return map[int][]model.CompatibleVehicle{}, nil
	}

	rowsById, err := s.repo.QueryCompatibleVehiclesBatch(context.Background(), ids, limitPerId)
	if err != nil {
		return nil, fmt.Errorf("find compatible vehicles batch: %w", err)
	}

	out := make(map[int][]model.CompatibleVehicle, len(rowsById))
	for id, rows := range rowsById {
		vs := s.buildResults(id, rows)
		if len(vs) > 0 {
			out[id] = vs
		}
	}
	return out, nil
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

// QueryCompatibleVehiclesBatch runs one IN-list variant of the primary
// 4-way join for multiple article ids. Returns rows keyed by
// legacyArticleId; ids with no rows are absent from the map.
//
// Caps len(ids) at 100 to protect against runaway batches; caller should
// chunk larger sets. Per-id row cap = limitPerId; total rows returned is
// bounded by len(ids) * limitPerId which is enforced by the ORDER BY +
// LIMIT below.
//
// Query strategy: use the same 4-way join as QueryCompatibleVehicles but
// substitute `WHERE ... IN (?,?,...)` for `WHERE ... = ?`. MySQL should
// range-scan idx_articlesvehicletrees_legacyArticleId once per id and
// merge — one query plan instead of N.
//
// No fallback for the batch. When a specific id in the batch returns 0
// rows, the caller (enrichResults) can still fall back to per-result
// FindCompatibleVehicles which chains primary + M3.S2.T1 fallback.
func (r *sqlVehicleFitmentRepo) QueryCompatibleVehiclesBatch(ctx context.Context, legacyArticleIds []int, limitPerId int) (map[int][]compatibleVehicleRow, error) {
	if len(legacyArticleIds) == 0 {
		return nil, nil
	}
	if len(legacyArticleIds) > 100 {
		legacyArticleIds = legacyArticleIds[:100]
	}
	if limitPerId <= 0 || limitPerId > 200 {
		limitPerId = 50
	}
	placeholders := make([]string, len(legacyArticleIds))
	args := make([]any, len(legacyArticleIds))
	for i, id := range legacyArticleIds {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `
		SELECT DISTINCT
			avt.legacyArticleId,
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
		WHERE avt.legacyArticleId IN (` + strings.Join(placeholders, ",") + `)
		  AND avt.linkingTargetType = 'P'
		ORDER BY avt.legacyArticleId, m.manuName, ms.modelname, lt.beginYearMonth
		LIMIT ` + fmt.Sprintf("%d", len(legacyArticleIds)*limitPerId)

	rows, err := logQueryCtx(r.db, ctx, "TecDocVehicle.FindCompatibleVehiclesBatch", q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int][]compatibleVehicleRow, len(legacyArticleIds))
	for rows.Next() {
		var legacyId int
		var row compatibleVehicleRow
		if err := rows.Scan(
			&legacyId,
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
		out[legacyId] = append(out[legacyId], row)
	}
	return out, nil
}
