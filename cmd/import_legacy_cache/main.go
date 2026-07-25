package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"parts-engine/internal/config"
	internaldb "parts-engine/internal/db"
)

func main() {
	cfg := config.Load()
	sqlitePath := os.Getenv("LEGACY_SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath = filepath.Join(cfg.DataDir, "hk_parts.db")
	}

	sqliteDB, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		log.Fatalf("open legacy sqlite: %v", err)
	}
	defer sqliteDB.Close()

	pgDB := internaldb.NewPostgres(cfg)
	if pgDB == nil {
		log.Fatal("postgres connection unavailable")
	}
	defer pgDB.Close()

	ctx := context.Background()
	tx, err := pgDB.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("begin postgres transaction: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
TRUNCATE TABLE
    hk_parts_cache,
    hk_platform_map,
    nhtsa_tecdoc_bridge,
    oem_search_index,
    substitution_links,
    vehicle_lookup
RESTART IDENTITY`); err != nil {
		log.Fatalf("truncate postgres tables: %v", err)
	}

	counts := map[string]int{}
	counts["hk_parts_cache"], err = importHKPartsCache(ctx, sqliteDB, tx)
	if err != nil {
		log.Fatalf("import hk_parts_cache: %v", err)
	}
	counts["hk_platform_map"], err = importHKPlatformMap(ctx, sqliteDB, tx)
	if err != nil {
		log.Fatalf("import hk_platform_map: %v", err)
	}
	counts["nhtsa_tecdoc_bridge"], err = importNHTSATecdocBridge(ctx, sqliteDB, tx)
	if err != nil {
		log.Fatalf("import nhtsa_tecdoc_bridge: %v", err)
	}
	counts["oem_search_index"], err = importOEMSearchIndex(ctx, sqliteDB, tx)
	if err != nil {
		log.Fatalf("import oem_search_index: %v", err)
	}
	counts["substitution_links"], err = importSubstitutionLinks(ctx, sqliteDB, tx)
	if err != nil {
		log.Fatalf("import substitution_links: %v", err)
	}
	counts["vehicle_lookup"], err = importVehicleLookup(ctx, sqliteDB, tx)
	if err != nil {
		log.Fatalf("import vehicle_lookup: %v", err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("commit postgres import: %v", err)
	}

	for _, table := range []string{"hk_parts_cache", "hk_platform_map", "nhtsa_tecdoc_bridge", "oem_search_index", "substitution_links", "vehicle_lookup"} {
		fmt.Printf("%s: imported %d rows\n", table, counts[table])
	}
}

func importHKPartsCache(ctx context.Context, sqliteDB *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := sqliteDB.QueryContext(ctx, `
SELECT
    linkingTargetId,
    legacyArticleId,
    assemblyGroupNodeId,
    articleNumber,
    genericArticleDesc,
    dataSupplierId,
    brandName,
    categoryName,
    vehicleDesc,
    manuId,
    modelId,
    modelName,
    beginYearMonth,
    endYearMonth,
    fuelType,
    capacityCC,
    horsePowerFrom
FROM hk_parts_cache`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO hk_parts_cache (
    linking_target_id,
    legacy_article_id,
    assembly_group_node_id,
    article_number,
    generic_article_desc,
    data_supplier_id,
    brand_name,
    category_name,
    vehicle_desc,
    manu_id,
    model_id,
    model_name,
    begin_year_month,
    end_year_month,
    fuel_type,
    capacity_cc,
    horse_power_from
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var linkingTargetID, legacyArticleID, assemblyGroupNodeID int
		var articleNumber, genericArticleDesc, brandName, categoryName, vehicleDesc, modelName, beginYearMonth, endYearMonth, fuelType sql.NullString
		var dataSupplierID, manuID, modelID, capacityCC, horsePowerFrom sql.NullInt64
		if err := rows.Scan(
			&linkingTargetID,
			&legacyArticleID,
			&assemblyGroupNodeID,
			&articleNumber,
			&genericArticleDesc,
			&dataSupplierID,
			&brandName,
			&categoryName,
			&vehicleDesc,
			&manuID,
			&modelID,
			&modelName,
			&beginYearMonth,
			&endYearMonth,
			&fuelType,
			&capacityCC,
			&horsePowerFrom,
		); err != nil {
			return count, err
		}
		if _, err := stmt.ExecContext(ctx,
			linkingTargetID,
			legacyArticleID,
			assemblyGroupNodeID,
			nullStringArg(articleNumber),
			nullStringArg(genericArticleDesc),
			nullIntArg(dataSupplierID),
			nullStringArg(brandName),
			nullStringArg(categoryName),
			nullStringArg(vehicleDesc),
			nullIntArg(manuID),
			nullIntArg(modelID),
			nullStringArg(modelName),
			nullStringArg(beginYearMonth),
			nullStringArg(endYearMonth),
			nullStringArg(fuelType),
			nullIntArg(capacityCC),
			nullIntArg(horsePowerFrom),
		); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func importHKPlatformMap(ctx context.Context, sqliteDB *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := sqliteDB.QueryContext(ctx, `SELECT platform_code, hyundai_model, kia_model FROM hk_platform_map`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO hk_platform_map (
    platform_code,
    hyundai_model,
    kia_model,
    gen_start_year,
    gen_end_year,
    notes
) VALUES ($1,$2,$3,NULL,NULL,NULL)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var platformCode, hyundaiModel, kiaModel sql.NullString
		if err := rows.Scan(&platformCode, &hyundaiModel, &kiaModel); err != nil {
			return count, err
		}
		if _, err := stmt.ExecContext(ctx,
			emptyToNull(platformCode.String),
			emptyToNull(hyundaiModel.String),
			emptyToNull(kiaModel.String),
		); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func importNHTSATecdocBridge(ctx context.Context, sqliteDB *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := sqliteDB.QueryContext(ctx, `
SELECT
    nhtsa_make,
    nhtsa_model,
    year_from,
    year_to,
    tecdoc_model_id
FROM nhtsa_tecdoc_bridge`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO nhtsa_tecdoc_bridge (
    nhtsa_make,
    nhtsa_model,
    year_from,
    year_to,
    tecdoc_manu_id,
    tecdoc_model_id,
    tecdoc_model_name
) VALUES ($1,$2,$3,$4,$5,$6,$7)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var nhtsaMake, nhtsaModel sql.NullString
		var yearFrom, yearTo, tecdocModelID sql.NullInt64
		if err := rows.Scan(&nhtsaMake, &nhtsaModel, &yearFrom, &yearTo, &tecdocModelID); err != nil {
			return count, err
		}
		makeName := strings.ToUpper(strings.TrimSpace(nhtsaMake.String))
		manuID, ok := tecdocManufacturerID(makeName)
		if !ok || !tecdocModelID.Valid || !yearFrom.Valid || !yearTo.Valid {
			continue
		}
		if _, err := stmt.ExecContext(ctx,
			makeName,
			emptyToNull(nhtsaModel.String),
			yearFrom.Int64,
			yearTo.Int64,
			manuID,
			tecdocModelID.Int64,
			emptyToNull(nhtsaModel.String),
		); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func importOEMSearchIndex(ctx context.Context, sqliteDB *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := sqliteDB.QueryContext(ctx, `
SELECT
    raw_number,
    normalized,
    legacyArticleId,
    source_table,
    mfr_name,
    brand_name,
    article_number,
    description
FROM oem_search_index`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO oem_search_index (
    raw_number,
    normalized,
    legacy_article_id,
    source_table,
    mfr_name,
    brand_name,
    article_number,
    description
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var rawNumber, normalized, sourceTable, mfrName, brandName, articleNumber, description sql.NullString
		var legacyArticleID sql.NullInt64
		if err := rows.Scan(
			&rawNumber,
			&normalized,
			&legacyArticleID,
			&sourceTable,
			&mfrName,
			&brandName,
			&articleNumber,
			&description,
		); err != nil {
			return count, err
		}
		if !legacyArticleID.Valid {
			continue
		}
		if _, err := stmt.ExecContext(ctx,
			emptyToNull(rawNumber.String),
			emptyToNull(normalized.String),
			legacyArticleID.Int64,
			emptyToNull(sourceTable.String),
			nullStringArg(mfrName),
			nullStringArg(brandName),
			nullStringArg(articleNumber),
			nullStringArg(description),
		); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func importSubstitutionLinks(ctx context.Context, sqliteDB *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := sqliteDB.QueryContext(ctx, `
SELECT from_part, to_part, description
FROM discovered_substitutions`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO substitution_links (
    from_part_number,
    to_part_number,
    description,
    source_key,
    source_label,
    source_detail,
    source_warning,
    confidence
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var fromPart, toPart, description sql.NullString
		if err := rows.Scan(&fromPart, &toPart, &description); err != nil {
			return count, err
		}
		if !fromPart.Valid || !toPart.Valid || strings.TrimSpace(fromPart.String) == "" || strings.TrimSpace(toPart.String) == "" {
			continue
		}
		if _, err := stmt.ExecContext(
			ctx,
			strings.ToUpper(strings.TrimSpace(fromPart.String)),
			strings.ToUpper(strings.TrimSpace(toPart.String)),
			nullStringArg(description),
			"legacy_discovered_substitutions",
			"Imported substitution evidence",
			"This link was imported from the legacy discovered_substitutions cache and preserves the source relationship for review.",
			"Imported substitution evidence is not an OEM-confirmed supersession chain. Confirm fitment and official replacement status before ordering.",
			0.72,
		); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func importVehicleLookup(ctx context.Context, sqliteDB *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := sqliteDB.QueryContext(ctx, `
SELECT
    nhtsa_make,
    nhtsa_model,
    year_from,
    year_to,
    linkageTargetId,
    description,
    beginYearMonth,
    endYearMonth,
    fuelType,
    capacityCC,
    horsePowerFrom
FROM vehicle_lookup`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO vehicle_lookup (
    nhtsa_make,
    nhtsa_model,
    year_from,
    year_to,
    linkage_target_id,
    description,
    begin_year_month,
    end_year_month,
    fuel_type,
    capacity_cc,
    horse_power_from
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var nhtsaMake, nhtsaModel, description, beginYearMonth, endYearMonth, fuelType sql.NullString
		var yearFrom, yearTo, linkageTargetID, capacityCC, horsePowerFrom sql.NullInt64
		if err := rows.Scan(
			&nhtsaMake,
			&nhtsaModel,
			&yearFrom,
			&yearTo,
			&linkageTargetID,
			&description,
			&beginYearMonth,
			&endYearMonth,
			&fuelType,
			&capacityCC,
			&horsePowerFrom,
		); err != nil {
			return count, err
		}
		if !yearFrom.Valid || !yearTo.Valid || !linkageTargetID.Valid {
			continue
		}
		if _, err := stmt.ExecContext(ctx,
			emptyToNull(nhtsaMake.String),
			emptyToNull(nhtsaModel.String),
			yearFrom.Int64,
			yearTo.Int64,
			linkageTargetID.Int64,
			emptyToNull(description.String),
			nullStringArg(beginYearMonth),
			nullStringArg(endYearMonth),
			nullStringArg(fuelType),
			nullIntArg(capacityCC),
			nullIntArg(horsePowerFrom),
		); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func tecdocManufacturerID(makeName string) (int, bool) {
	switch strings.ToUpper(strings.TrimSpace(makeName)) {
	case "HYUNDAI":
		return 183, true
	case "KIA":
		return 184, true
	default:
		return 0, false
	}
}

func nullStringArg(v sql.NullString) any {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	return v.String
}

func nullIntArg(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func emptyToNull(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
