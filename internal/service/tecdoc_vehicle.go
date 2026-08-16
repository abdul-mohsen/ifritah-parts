package service

import (
	"context"
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
)

// vehicleFitmentRepo is the injectable DB dep for TecDocVehicle.
type vehicleFitmentRepo interface {
	QueryCompatibleVehicles(ctx context.Context, legacyArticleId, limit int) ([]compatibleVehicleRow, error)
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

	rows, err := s.repo.QueryCompatibleVehicles(context.Background(), legacyArticleId, limit)
	if err != nil {
		return nil, fmt.Errorf("find compatible vehicles: %w", err)
	}

	out := make([]model.CompatibleVehicle, 0, len(rows))
	seen := map[int]bool{}
	for _, r := range rows {
		if r.LinkageTargetId == 0 || seen[r.LinkageTargetId] {
			continue
		}
		seen[r.LinkageTargetId] = true
		out = append(out, model.CompatibleVehicle{
			LegacyArticleId: legacyArticleId,
			LinkageTargetId: r.LinkageTargetId,
			VehicleName:     r.VehicleName,
			Make:            r.Make,
			Model:           r.Model,
			YearFrom:        yearFromYearMonth(r.BeginYearMonth),
			YearTo:          yearFromYearMonth(r.EndYearMonth),
			FuelType:        r.FuelType,
			CapacityCC:      r.CapacityCC,
			HorsePower:      r.HorsePower,
			FitmentDriver:   driverName(ClassifyCategory(r.CategoryHint).Driver),
		})
	}
	return out, nil
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
