package service

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"parts-engine/internal/model"
)

// SmartSearch provides category-aware parts search with cross-reference expansion.
type SmartSearch struct {
	db           *sql.DB
	parts        *PartsLookup
	crossRef     *CrossRef
	oem          *OEMLookup
	platform     *Platform
	onlineLookup *PartsOuqService
	dealerLookup *DealerLookup
	tecdoc       *TecDoc
	dependency   *DependencyClassifier
	offline      bool
}

func NewSmartSearch(db *sql.DB, parts *PartsLookup, crossRef *CrossRef, oem *OEMLookup, platform *Platform, online *PartsOuqService, offline bool) *SmartSearch {
	return &SmartSearch{db: db, parts: parts, crossRef: crossRef, oem: oem, platform: platform, onlineLookup: online, offline: offline}
}

// SetDependencyClassifier attaches the data-driven dependency classifier.
func (s *SmartSearch) SetDependencyClassifier(dc *DependencyClassifier) {
	s.dependency = dc
}

// SetDealerLookup attaches the dealer dictionary fallback service.
func (s *SmartSearch) SetDealerLookup(dl *DealerLookup) {
	s.dealerLookup = dl
}

// SetTecDoc attaches the full TecDoc MySQL service (only available when MySQL is connected).
func (s *SmartSearch) SetTecDoc(td *TecDoc) {
	s.tecdoc = td
}

// SmartResult is an enhanced part result with confidence and cross-ref data.
type SmartResult struct {
	model.Part
	Confidence              float64                  `json:"confidence"`
	ConfidenceNote          string                   `json:"confidenceNote,omitempty"`
	FitmentDriver           string                   `json:"fitmentDriver"`
	OEMNumbers              []model.OEMReference     `json:"oemNumbers,omitempty"`
	BrandResolved           string                   `json:"brand,omitempty"`
	FitsVehicleCC           int                      `json:"fitsVehicleCC,omitempty"`
	Substitutions           []model.SubstitutionPart `json:"substitutions,omitempty"`
	AftermarketAlternatives []model.AftermarketPart  `json:"aftermarketAlternatives,omitempty"`
	Compatibility           []string                 `json:"compatibility,omitempty"`
}

// SmartSearchResponse is the full smart search response.
type SmartSearchResponse struct {
	Query          string         `json:"query"`
	Vehicle        *model.Vehicle `json:"vehicle,omitempty"`
	Results        []SmartResult  `json:"results"`
	Total          int            `json:"total"`
	Categories     []string       `json:"categories,omitempty"`
	SearchStrategy string         `json:"searchStrategy"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// Search runs the smart search engine.
// It detects query type (OEM number, part number, category text, VIN),
// then applies category-aware filtering with cross-reference expansion.
func (s *SmartSearch) Search(query string, linkageTargetId int, vehicleCC int, fuelType string, category string, page, limit int) (*SmartSearchResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if page < 1 {
		page = 1
	}

	query = strings.TrimSpace(query)

	// Detect search type with cascade fallback when no results found
	switch {
	case looksLikeOEMNumber(query):
		resp, err := s.searchByOEM(query, linkageTargetId, vehicleCC, fuelType, limit)
		if err != nil {
			return resp, err
		}
		if resp.Total > 0 {
			// If only online results, also check local article DB (may have richer data)
			if resp.SearchStrategy == "online_partsouq" {
				artResp, artErr := s.searchByArticle(query, linkageTargetId, vehicleCC, limit)
				if artErr == nil && artResp.Total > 0 {
					return artResp, nil
				}
			}
			return resp, nil
		}
		// OEM search found nothing — try article lookup then text search
		artResp, err := s.searchByArticle(query, linkageTargetId, vehicleCC, limit)
		if err == nil && artResp.Total > 0 {
			return artResp, nil
		}
		textResp, err := s.searchByText(query, vehicleCC, fuelType, page, limit)
		if err == nil && textResp.Total > 0 {
			return textResp, nil
		}
		return resp, nil // return original OEM response with warnings
	case looksLikeArticleNumber(query):
		resp, err := s.searchByArticle(query, linkageTargetId, vehicleCC, limit)
		if err != nil {
			return resp, err
		}
		if resp.Total > 0 {
			return resp, nil
		}
		// Article lookup found nothing — try text search
		textResp, err := s.searchByText(query, vehicleCC, fuelType, page, limit)
		if err == nil && textResp.Total > 0 {
			return textResp, nil
		}
		return resp, nil
	case linkageTargetId > 0:
		return s.searchByVehicle(query, linkageTargetId, vehicleCC, fuelType, category, page, limit)
	default:
		return s.searchByText(query, vehicleCC, fuelType, page, limit)
	}
}

// searchByOEM finds aftermarket parts for an OEM number, then filters by vehicle fit.
func (s *SmartSearch) searchByOEM(oemNum string, linkageTargetId, vehicleCC int, fuelType string, limit int) (*SmartSearchResponse, error) {
	start := time.Now()
	log.Printf("[SmartSearch.searchByOEM] ========== START oem=%q vehicle=%d cc=%d fuel=%q limit=%d ==========", oemNum, linkageTargetId, vehicleCC, fuelType, limit)

	resp := &SmartSearchResponse{
		Query:          oemNum,
		SearchStrategy: "oem_crossref",
	}

	// Step 1: CrossRef service (oem_search_index — indexed)
	log.Printf("[SmartSearch.searchByOEM] STEP 1: CrossRef.FindByOEM")
	refs, err := s.crossRef.FindByOEM(oemNum, limit)
	if err != nil {
		log.Printf("[SmartSearch.searchByOEM] STEP 1 ERROR: %v (elapsed=%v)", err, time.Since(start))
		return nil, err
	}
	log.Printf("[SmartSearch.searchByOEM] STEP 1 DONE: %d refs (elapsed=%v)", len(refs), time.Since(start))

	// Step 2: Also check oem_search_index
	log.Printf("[SmartSearch.searchByOEM] STEP 2: OEMLookup.Search")
	oemResult, _ := s.oem.Search(oemNum, limit)
	if oemResult != nil {
		log.Printf("[SmartSearch.searchByOEM] STEP 2 DONE: %d results (elapsed=%v)", len(oemResult.Results), time.Since(start))
		for _, r := range oemResult.Results {
			// Deduplicate by legacyArticleId
			dup := false
			for _, existing := range refs {
				if existing.LegacyArticleId == r.LegacyArticleId {
					dup = true
					break
				}
			}
			if !dup {
				refs = append(refs, r)
			}
		}
	}

	log.Printf("[SmartSearch.searchByOEM] MERGED refs=%d (elapsed=%v)", len(refs), time.Since(start))

	if len(refs) == 0 {
		// Strategy 0: TecDoc full database (oem_number 21.5M + oem_search_index via MySQL)
		log.Printf("[SmartSearch.searchByOEM] STEP 3: No refs yet — trying TecDoc")
		if s.tecdoc != nil {
			tdRefs, tdErr := s.tecdoc.SearchByOEM(oemNum, limit)
			if tdErr == nil && len(tdRefs) > 0 {
				log.Printf("[SmartSearch.searchByOEM] STEP 3 HIT: TecDoc returned %d refs (elapsed=%v)", len(tdRefs), time.Since(start))
				refs = tdRefs
				resp.SearchStrategy = "tecdoc_oem"
				goto buildResults
			}
		}

		// Strategy 1: Try stripping color/trim suffixes and re-search locally
		log.Printf("[SmartSearch.searchByOEM] STEP 4: suffix-strip fallback")
		if stripped, ok := stripColorSuffix(oemNum); ok && stripped != oemNum {
			log.Printf("[SmartSearch.searchByOEM] STEP 4: trying stripped=%q", stripped)
			strippedRefs, _ := s.crossRef.FindByOEM(stripped, limit)
			strippedOEM, _ := s.oem.Search(stripped, limit)
			if strippedOEM != nil {
				for _, r := range strippedOEM.Results {
					dup := false
					for _, e := range strippedRefs {
						if e.LegacyArticleId == r.LegacyArticleId {
							dup = true
							break
						}
					}
					if !dup {
						strippedRefs = append(strippedRefs, r)
					}
				}
			}
			if len(strippedRefs) > 0 {
				refs = strippedRefs
				resp.Warnings = append(resp.Warnings,
					fmt.Sprintf("Matched by base number %s (color/trim suffix removed)", stripped))
				goto buildResults
			}
		}

		// Strategy 2: Prefix fuzzy match — try first 8 chars of normalized OEM
		log.Printf("[SmartSearch.searchByOEM] STEP 5: prefix match fallback")
		{
			norm8 := NormalizeOEM(oemNum)
			if len(norm8) >= 10 {
				norm8 = norm8[:len(norm8)-2] // drop last 2 chars
			} else if len(norm8) >= 8 {
				norm8 = norm8[:8]
			}
			if len(norm8) >= 8 {
				prefixRefs := s.prefixOEMSearch(norm8, limit)
				if len(prefixRefs) > 0 {
					refs = prefixRefs
					resp.SearchStrategy = "oem_prefix_match"
					resp.Warnings = append(resp.Warnings,
						fmt.Sprintf("Exact OEM not found; matched by prefix %s...", norm8[:8]))
					goto buildResults
				}
			}
		}

		// Strategy 3: Try online lookup
		log.Printf("[SmartSearch.searchByOEM] STEP 6: online lookup (partsouq)")
		if s.onlineLookup != nil {
			onlineResults, err := s.onlineLookup.LookupPart(oemNum)
			if err == nil && len(onlineResults) > 0 {
				log.Printf("online lookup found %d parts for: %s", len(onlineResults), oemNum)
				resp.SearchStrategy = "online_partsouq"
				for _, onlineResult := range onlineResults {
					if onlineResult.Description == "" {
						continue
					}
					result := SmartResult{
						Part: model.Part{
							LegacyArticleId: 0,
							ArticleNumber:   onlineResult.PartNumber,
							Description:     onlineResult.Description,
							BrandName:       onlineResult.Make,
							Category:        onlineResult.Category,
						},
						Confidence:              0.75,
						ConfidenceNote:          "Online lookup from PartsOuq",
						FitmentDriver:           "online",
						BrandResolved:           onlineResult.Make,
						Substitutions:           onlineResult.Substitutions,
						AftermarketAlternatives: onlineResult.Aftermarket,
						Compatibility:           onlineResult.Compatibility,
					}
					resp.Results = append(resp.Results, result)
				}
				resp.Total = len(resp.Results)
				if len(resp.Results) > 0 && onlineResults[0].Category != "" {
					resp.Categories = []string{onlineResults[0].Category}
				}
				s.enrichAftermarket(resp)
				return resp, nil
			}
			if err != nil {
				log.Printf("online lookup error for %s: %v", oemNum, err)
			}

			// Strategy 4: Online lookup with suffix stripped
			if stripped, ok := stripColorSuffix(oemNum); ok && stripped != oemNum {
				onlineResults2, err2 := s.onlineLookup.LookupPart(stripped)
				if err2 == nil && len(onlineResults2) > 0 {
					log.Printf("online lookup (suffix-stripped) found %d parts for: %s → %s", len(onlineResults2), oemNum, stripped)
					resp.SearchStrategy = "online_partsouq_stripped"
					for _, onlineResult := range onlineResults2 {
						if onlineResult.Description == "" {
							continue
						}
						result := SmartResult{
							Part: model.Part{
								LegacyArticleId: 0,
								ArticleNumber:   onlineResult.PartNumber,
								Description:     onlineResult.Description,
								BrandName:       onlineResult.Make,
								Category:        onlineResult.Category,
							},
							Confidence:              0.70,
							ConfidenceNote:          "Online lookup (color suffix removed)",
							FitmentDriver:           "online",
							BrandResolved:           onlineResult.Make,
							Substitutions:           onlineResult.Substitutions,
							AftermarketAlternatives: onlineResult.Aftermarket,
							Compatibility:           onlineResult.Compatibility,
						}
						resp.Results = append(resp.Results, result)
					}
					resp.Total = len(resp.Results)
					s.enrichAftermarket(resp)
					return resp, nil
				}
			}
		}

		// Strategy 5: ECU base number matching (391xxx series)
		log.Printf("[SmartSearch.searchByOEM] STEP 7: ECU base match")
		{
			norm := NormalizeOEM(oemNum)
			if len(norm) >= 10 && (strings.HasPrefix(norm, "39110") || strings.HasPrefix(norm, "39121") ||
				strings.HasPrefix(norm, "39133") || strings.HasPrefix(norm, "39171") ||
				strings.HasPrefix(norm, "39108") || strings.HasPrefix(norm, "39100") ||
				strings.HasPrefix(norm, "39113") || strings.HasPrefix(norm, "39128") ||
				strings.HasPrefix(norm, "39160") || strings.HasPrefix(norm, "39210")) {
				ecuRefs := s.prefixOEMSearch(norm[:8], limit)
				if len(ecuRefs) > 0 {
					refs = ecuRefs
					resp.SearchStrategy = "ecu_base_match"
					resp.Warnings = append(resp.Warnings,
						fmt.Sprintf("ECU variant match by base number %s (exact variant %s not cataloged)", norm[:8], oemNum))
					goto buildResults
				}
			}
		}

		// Strategy 6: Dealer site direct lookup (hyundaipartsdeal.com / kiapartsnow.com)
		log.Printf("[SmartSearch.searchByOEM] STEP 8: dealer lookup")
		if s.dealerLookup != nil {
			dealerResult := s.dealerLookup.LookupPart(oemNum)
			if dealerResult != nil && dealerResult.Description != "" {
				log.Printf("dealer lookup found: %s → %s", oemNum, dealerResult.Description)
				resp.SearchStrategy = "dealer_lookup"
				result := SmartResult{
					Part: model.Part{
						ArticleNumber: dealerResult.PartNumber,
						Description:   dealerResult.Description,
						BrandName:     dealerResult.Make,
						Category:      dealerResult.Category,
					},
					Confidence:     0.70,
					ConfidenceNote: "Found via dealer catalog (" + dealerResult.Source + ")",
					FitmentDriver:  "online",
					BrandResolved:  dealerResult.Make,
				}
				if decoded := DecodeOEMPrefix(oemNum); decoded != nil {
					result.Part.Category = decoded.System + " / " + decoded.Category
				}
				resp.Results = append(resp.Results, result)
				resp.Total = 1
				s.enrichAftermarket(resp)
				return resp, nil
			}
		}

		// Strategy 7: Reverse supersession — check if any cached part lists this as a substitution
		log.Printf("[SmartSearch.searchByOEM] STEP 9: reverse supersession")
		if s.onlineLookup != nil && s.onlineLookup.GetCache() != nil {
			if superseded := s.onlineLookup.GetCache().FindBySubstitution(NormalizeOEM(oemNum)); superseded != nil {
				log.Printf("supersession found: %s → via %s", oemNum, superseded.PartNumber)
				resp.SearchStrategy = "supersession_reverse"
				result := SmartResult{
					Part: model.Part{
						ArticleNumber: superseded.PartNumber,
						Description:   superseded.Description,
						BrandName:     superseded.Make,
						Category:      superseded.Category,
					},
					Confidence:     0.65,
					ConfidenceNote: fmt.Sprintf("Part %s is superseded; found via replacement %s", oemNum, superseded.PartNumber),
					FitmentDriver:  "online",
					BrandResolved:  superseded.Make,
				}
				resp.Results = append(resp.Results, result)
				resp.Total = 1
				resp.Warnings = append(resp.Warnings,
					fmt.Sprintf("Part %s appears to be superseded. Replacement: %s", oemNum, superseded.PartNumber))
				s.enrichAftermarket(resp)
				return resp, nil
			}
		}

		log.Printf("[SmartSearch.searchByOEM] STEP 10: aftermarket crossref fallback")
		resp.Warnings = append(resp.Warnings, "No cross-references found for OEM number: "+oemNum)
		// Decode OEM prefix to give user at least a category hint
		if decoded := DecodeOEMPrefix(oemNum); decoded != nil {
			resp.Warnings = append(resp.Warnings,
				fmt.Sprintf("OEM prefix %s indicates: %s / %s", decoded.Prefix, decoded.System, decoded.Category))
		}

		// Strategy 8: Even if OEM part not found, check aftermarket cross-ref DB
		if s.crossRef != nil {
			amParts, _ := s.crossRef.FindAftermarketByOEM(oemNum)
			if len(amParts) > 0 {
				resp.SearchStrategy = "aftermarket_crossref_only"
				desc := "OEM Part"
				cat := ""
				if decoded := DecodeOEMPrefix(oemNum); decoded != nil {
					desc = decoded.Category
					cat = decoded.System + " / " + decoded.Category
				}
				result := SmartResult{
					Part: model.Part{
						ArticleNumber: oemNum,
						Description:   desc,
						BrandName:     "HYUNDAI/KIA",
						Category:      cat,
					},
					Confidence:              0.50,
					ConfidenceNote:          "OEM part not found in catalog, but aftermarket alternatives available",
					FitmentDriver:           "crossref",
					BrandResolved:           "HYUNDAI/KIA",
					AftermarketAlternatives: amParts,
				}
				resp.Results = append(resp.Results, result)
				resp.Total = 1
				resp.Warnings = append(resp.Warnings, "Part details unavailable but aftermarket cross-references found")
				return resp, nil
			}
		}

		return resp, nil
	}

buildResults:

	log.Printf("[SmartSearch.searchByOEM] BUILD RESULTS: %d refs → dedup + confidence (elapsed=%v)", len(refs), time.Since(start))

	seen := make(map[int]bool)
	for _, ref := range refs {
		if seen[ref.LegacyArticleId] {
			continue
		}
		seen[ref.LegacyArticleId] = true

		rule := ClassifyCategory(ref.Description)
		conf, note := s.computeConfidence(rule, vehicleCC, 0, fuelType, "", ref.LegacyArticleId, linkageTargetId)

		result := SmartResult{
			Part: model.Part{
				LegacyArticleId: ref.LegacyArticleId,
				ArticleNumber:   ref.ArticleNumber,
				Description:     ref.Description,
				BrandName:       ref.BrandName,
			},
			Confidence:     conf,
			ConfidenceNote: note,
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  ref.BrandName,
			OEMNumbers:     []model.OEMReference{ref},
		}
		resp.Results = append(resp.Results, result)
	}

	// Enrich all results with aftermarket cross-references from curated DB
	s.enrichAftermarket(resp)

	resp.Total = len(resp.Results)
	return resp, nil
}

// enrichAftermarket adds aftermarket alternatives from the aftermarket_crossref table
// to all results that don't already have them. Also checks the original query OEM number.
func (s *SmartSearch) enrichAftermarket(resp *SmartSearchResponse) {
	if s.crossRef == nil {
		return
	}
	queryOEM := resp.Query // original user query

	for i := range resp.Results {
		r := &resp.Results[i]
		existing := make(map[string]bool)
		for _, p := range r.AftermarketAlternatives {
			existing[strings.ToUpper(p.Brand+"|"+p.PartNumber)] = true
		}

		addParts := func(oem string) {
			if oem == "" {
				return
			}
			amParts, err := s.crossRef.FindAftermarketByOEM(oem)
			if err != nil || len(amParts) == 0 {
				return
			}
			for _, p := range amParts {
				key := strings.ToUpper(p.Brand + "|" + p.PartNumber)
				if !existing[key] {
					r.AftermarketAlternatives = append(r.AftermarketAlternatives, p)
					existing[key] = true
				}
			}
		}

		// Check by returned article number
		addParts(r.Part.ArticleNumber)
		// Also check by original query (handles prefix match / supersession cases)
		// Only for the first result to avoid duplicating AM across sub-results
		if i == 0 && NormalizeOEM(queryOEM) != NormalizeOEM(r.Part.ArticleNumber) {
			addParts(queryOEM)
		}

		// TecDoc full DB enrichment: oem_number → aftermarket articles
		if s.tecdoc != nil && len(r.AftermarketAlternatives) == 0 {
			oems := []string{r.Part.ArticleNumber}
			if queryOEM != "" && NormalizeOEM(queryOEM) != NormalizeOEM(r.Part.ArticleNumber) {
				oems = append(oems, queryOEM)
			}
			for _, ref := range r.OEMNumbers {
				if ref.RawNumber != "" {
					oems = append(oems, ref.RawNumber)
				}
			}
			for _, oem := range oems {
				tdParts, _ := s.tecdoc.FindAftermarketForOEM(oem)
				for _, p := range tdParts {
					key := strings.ToUpper(p.Brand + "|" + p.PartNumber)
					if !existing[key] {
						r.AftermarketAlternatives = append(r.AftermarketAlternatives, p)
						existing[key] = true
					}
				}
				if len(r.AftermarketAlternatives) > 0 {
					break
				}
			}
		}
	}
}

// searchByArticle finds a specific aftermarket part number and its cross-references.
func (s *SmartSearch) searchByArticle(artNum string, linkageTargetId, vehicleCC, limit int) (*SmartSearchResponse, error) {
	resp := &SmartSearchResponse{
		Query:          artNum,
		SearchStrategy: "article_lookup",
	}

	normalized := strings.ToUpper(strings.TrimSpace(artNum))

	// Find in hk_parts_cache first (fast)
	var query string
	if s.offline {
		// SQLite: no articles/ambrand tables
		query = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       hk.brandName, hk.categoryName, hk.assemblyGroupNodeId,
			       hk.capacityCC
			FROM hk_parts_cache hk
			WHERE UPPER(hk.articleNumber) = ?
			LIMIT ?`
	} else {
		query = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       COALESCE(ab.brandName, hk.brandName) AS brand, hk.categoryName, hk.assemblyGroupNodeId,
			       hk.capacityCC
			FROM hk_parts_cache hk
			LEFT JOIN articles a ON a.legacyArticleId = hk.legacyArticleId
			LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
			WHERE UPPER(hk.articleNumber) = ?
			LIMIT ?`
	}

	rows, err := logQuery(s.db, "SmartSearch.searchByArticle.hk", query, normalized, limit)
	if err != nil {
		return nil, fmt.Errorf("article search: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p model.Part
		var desc, brand, cat sql.NullString
		var partCC sql.NullInt32
		if err := rows.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand, &cat, &p.AssemblyGroupId, &partCC); err != nil {
			return nil, fmt.Errorf("scan article: %w", err)
		}
		p.Description = desc.String
		p.BrandName = brand.String
		p.Category = cat.String

		rule := ClassifyCategory(p.Description)
		var fitsCC int
		if partCC.Valid {
			fitsCC = int(partCC.Int32)
		}

		conf, note := s.computeConfidence(rule, vehicleCC, fitsCC, "", "", p.LegacyArticleId, linkageTargetId)

		result := SmartResult{
			Part:           p,
			Confidence:     conf,
			ConfidenceNote: note,
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  brand.String,
			FitsVehicleCC:  fitsCC,
		}

		// Fetch OEM cross-refs
		oems, _ := s.crossRef.FindOEMNumbers(p.LegacyArticleId)
		if len(oems) > 0 {
			result.OEMNumbers = oems
		}

		resp.Results = append(resp.Results, result)
	}

	if len(resp.Results) == 0 && !s.offline {
		// Fallback: search articles table directly (MySQL only)
		fallback := `
			SELECT a.legacyArticleId, a.articleNumber, a.genericArticleDescription,
			       ab.brandName
			FROM articles a
			LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
			WHERE UPPER(a.articleNumber) = ?
			LIMIT ?`

		rows2, err := logQuery(s.db, "SmartSearch.searchByArticle.fallback", fallback, normalized, limit)
		if err != nil {
			return nil, fmt.Errorf("article fallback: %w", err)
		}
		defer rows2.Close()

		for rows2.Next() {
			var p model.Part
			var desc, brand sql.NullString
			if err := rows2.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand); err != nil {
				return nil, fmt.Errorf("scan fallback: %w", err)
			}
			p.Description = desc.String
			p.BrandName = brand.String

			result := SmartResult{
				Part:           p,
				Confidence:     0.5,
				ConfidenceNote: "Found in articles table (not in HK cache — may not fit your vehicle)",
				FitmentDriver:  driverName(ClassifyCategory(p.Description).Driver),
				BrandResolved:  brand.String,
			}

			oems, _ := s.crossRef.FindOEMNumbers(p.LegacyArticleId)
			if len(oems) > 0 {
				result.OEMNumbers = oems
			}

			resp.Results = append(resp.Results, result)
		}
	}

	// Fallback: if article lookup found nothing and query looks like it could be a dashless OEM number,
	// try OEM search with common dash positions (Hyundai/Kia OEM format: XXXXX-XXXXX)
	// Close all open rows first to release the single SQLite connection.
	rows.Close()
	if len(resp.Results) == 0 && len(normalized) >= 9 {
		oemCandidates := generateOEMCandidates(normalized)
		for _, candidate := range oemCandidates {
			oemResp, err := s.searchByOEM(candidate, linkageTargetId, vehicleCC, "", limit)
			if err == nil && len(oemResp.Results) > 0 {
				oemResp.Query = artNum
				oemResp.SearchStrategy = "article_to_oem_fallback"
				oemResp.Warnings = append(oemResp.Warnings,
					fmt.Sprintf("Interpreted as OEM number: %s", candidate))
				return oemResp, nil
			}
		}
	}

	resp.Total = len(resp.Results)
	return resp, nil
}

// generateOEMCandidates inserts dashes at likely positions in a dashless part number.
// Hyundai/Kia OEM format is typically XXXXX-XXXXX (5-5) or XXXXX-XXXXXXX.
func generateOEMCandidates(s string) []string {
	var candidates []string
	// Try 5-digit prefix (most common HK OEM pattern: 26300-35503)
	if len(s) >= 9 {
		candidates = append(candidates, s[:5]+"-"+s[5:])
	}
	// Try 4-digit prefix (some older parts)
	if len(s) >= 8 {
		candidates = append(candidates, s[:4]+"-"+s[4:])
	}
	// Try 2-char prefix for old KIA format (0K prefix: 0K011-34160A)
	if len(s) >= 10 && (strings.HasPrefix(s, "0K") || strings.HasPrefix(s, "0k")) {
		candidates = append(candidates, s[:5]+"-"+s[5:])
		candidates = append(candidates, s[:4]+"-"+s[4:])
	}
	return candidates
}

// knownColorSuffixes are color/region/trim codes appended to HK OEM numbers.
// These do not affect part compatibility — the base number is the actual part.
var knownColorSuffixes = []string{
	"MZH", "EB", "4X", "WK", "Y8S", "SWP", "IM", "M9Y", "4SS", "UU5", "MBS",
	"V8S", "S2C", "MST", "RY", "SAE", "WLC", "PDW", "UM", "YDA", "MBA",
	"AAH", "ABT", "ABS", "ACS", "ABP", "AB", "GB", "HU", "HUB",
}

// stripColorSuffix attempts to remove a known color/trim/region suffix from the
// end of a Hyundai/KIA OEM part number. Returns the stripped number with dash
// re-appended if appropriate, plus a bool indicating if stripping occurred.
func stripColorSuffix(oem string) (string, bool) {
	upper := strings.ToUpper(strings.TrimSpace(oem))
	// Remove dashes/spaces for suffix check
	normalized := normalizeForSuffix(upper)
	for _, sfx := range knownColorSuffixes {
		if strings.HasSuffix(normalized, sfx) && len(normalized) > len(sfx)+5 {
			base := normalized[:len(normalized)-len(sfx)]
			// Re-insert dash at position 5 if it looks like an OEM number
			if len(base) >= 9 && base[0] >= '0' && base[0] <= '9' {
				return base[:5] + "-" + base[5:], true
			}
			return base, true
		}
	}
	return oem, false
}

func normalizeForSuffix(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// prefixOEMSearch does a LIKE prefix search on the oem_search_index for near-miss lookups.
func (s *SmartSearch) prefixOEMSearch(normalizedPrefix string, limit int) []model.OEMReference {
	if s.db == nil || len(normalizedPrefix) < 8 {
		return nil
	}
	query := `SELECT raw_number, normalized, legacyArticleId, source_table,
	                 mfr_name, brand_name, article_number, description
	          FROM oem_search_index
	          WHERE normalized LIKE ?
	          LIMIT ?`
	rows, err := logQuery(s.db, "SmartSearch.prefixOEM", query, normalizedPrefix+"%", limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var refs []model.OEMReference
	seen := make(map[int]bool)
	for rows.Next() {
		var ref model.OEMReference
		var norm, src, mfr, brand, artNum, desc sql.NullString
		if err := rows.Scan(&ref.RawNumber, &norm, &ref.LegacyArticleId, &src,
			&mfr, &brand, &artNum, &desc); err != nil {
			continue
		}
		if seen[ref.LegacyArticleId] {
			continue
		}
		seen[ref.LegacyArticleId] = true
		ref.Normalized = norm.String
		ref.Manufacturer = mfr.String
		ref.BrandName = brand.String
		ref.ArticleNumber = artNum.String
		ref.Description = desc.String
		refs = append(refs, ref)
	}
	return refs
}

// searchByVehicle searches parts for a specific vehicle, using category-aware filtering.
func (s *SmartSearch) searchByVehicle(textFilter string, linkageTargetId, vehicleCC int, fuelType, category string, page, limit int) (*SmartSearchResponse, error) {
	resp := &SmartSearchResponse{
		Query:          textFilter,
		SearchStrategy: "vehicle_smart",
	}

	offset := (page - 1) * limit

	// Build the query with optional text and category filters
	where := "hk.linkingTargetId = ?"
	args := []any{linkageTargetId}

	if category != "" {
		where += " AND (hk.genericArticleDesc LIKE ? OR hk.categoryName LIKE ?)"
		args = append(args, "%"+category+"%", "%"+category+"%")
	}
	if textFilter != "" {
		where += " AND (hk.genericArticleDesc LIKE ? OR hk.articleNumber LIKE ? OR hk.categoryName LIKE ?)"
		args = append(args, "%"+textFilter+"%", "%"+textFilter+"%", "%"+textFilter+"%")
	}

	// Count
	countSQL := "SELECT COUNT(DISTINCT hk.legacyArticleId) FROM hk_parts_cache hk WHERE " + where
	var total int
	if err := logQueryRow(s.db, "SmartSearch.searchByVehicle.count", countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}
	resp.Total = total

	// Get distinct categories available
	catSQL := `SELECT DISTINCT hk.genericArticleDesc FROM hk_parts_cache hk WHERE hk.linkingTargetId = ? AND hk.genericArticleDesc IS NOT NULL AND hk.genericArticleDesc != '' ORDER BY hk.genericArticleDesc`
	catRows, err := logQuery(s.db, "SmartSearch.searchByVehicle.categories", catSQL, linkageTargetId)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var c string
			if catRows.Scan(&c) == nil && c != "" {
				resp.Categories = append(resp.Categories, c)
			}
		}
	}

	// Get parts with brand resolved
	var dataSQL string
	if s.offline {
		dataSQL = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       hk.brandName, hk.categoryName,
			       hk.assemblyGroupNodeId, hk.capacityCC, hk.fuelType
			FROM hk_parts_cache hk
			WHERE ` + where + `
			ORDER BY hk.genericArticleDesc, hk.brandName
			LIMIT ? OFFSET ?`
	} else {
		dataSQL = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       COALESCE(ab.brandName, hk.brandName) AS brand, hk.categoryName,
			       hk.assemblyGroupNodeId, hk.capacityCC, hk.fuelType
			FROM hk_parts_cache hk
			LEFT JOIN articles a ON a.legacyArticleId = hk.legacyArticleId
			LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
			WHERE ` + where + `
			ORDER BY hk.genericArticleDesc, brand
			LIMIT ? OFFSET ?`
	}

	dataArgs := append(args, limit, offset)
	rows, err := logQuery(s.db, "SmartSearch.searchByVehicle.data", dataSQL, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("vehicle search: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p model.Part
		var desc, brand, cat, partFuel sql.NullString
		var partCC sql.NullInt32
		if err := rows.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand, &cat, &p.AssemblyGroupId, &partCC, &partFuel); err != nil {
			return nil, fmt.Errorf("scan smart: %w", err)
		}
		p.Description = desc.String
		p.BrandName = brand.String
		p.Category = cat.String

		rule := ClassifyCategory(p.Description)
		var fitsCC int
		if partCC.Valid {
			fitsCC = int(partCC.Int32)
		}

		conf, note := s.computeConfidenceForVehicle(rule, vehicleCC, fitsCC, fuelType, partFuel.String)

		result := SmartResult{
			Part:           p,
			Confidence:     conf,
			ConfidenceNote: note,
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  brand.String,
			FitsVehicleCC:  fitsCC,
		}

		resp.Results = append(resp.Results, result)
	}

	return resp, nil
}

// searchByText does a text search across parts descriptions and article numbers.
func (s *SmartSearch) searchByText(text string, vehicleCC int, fuelType string, page, limit int) (*SmartSearchResponse, error) {
	resp := &SmartSearchResponse{
		Query:          text,
		SearchStrategy: "text_search",
	}
	offset := (page - 1) * limit

	// Search hk_parts_cache by text match on description, article number, category
	var dataSQL string
	if s.offline {
		dataSQL = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       hk.brandName, hk.categoryName,
			       hk.assemblyGroupNodeId, hk.capacityCC
			FROM hk_parts_cache hk
			WHERE (hk.genericArticleDesc LIKE ? OR hk.articleNumber LIKE ? OR hk.categoryName LIKE ?)
			ORDER BY hk.genericArticleDesc, hk.brandName
			LIMIT ? OFFSET ?`
	} else {
		dataSQL = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       COALESCE(ab.brandName, hk.brandName) AS brand, hk.categoryName,
			       hk.assemblyGroupNodeId, hk.capacityCC
			FROM hk_parts_cache hk
			LEFT JOIN articles a ON a.legacyArticleId = hk.legacyArticleId
			LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
			WHERE (hk.genericArticleDesc LIKE ? OR hk.articleNumber LIKE ? OR hk.categoryName LIKE ?)
			ORDER BY hk.genericArticleDesc, brand
			LIMIT ? OFFSET ?`
	}

	pattern := "%" + text + "%"
	rows, err := logQuery(s.db, "SmartSearch.searchByText", dataSQL, pattern, pattern, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("text search: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p model.Part
		var desc, brand, cat sql.NullString
		var partCC sql.NullInt32
		if err := rows.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand, &cat, &p.AssemblyGroupId, &partCC); err != nil {
			return nil, fmt.Errorf("scan text: %w", err)
		}
		p.Description = desc.String
		p.BrandName = brand.String
		p.Category = cat.String

		rule := ClassifyCategory(p.Description)
		result := SmartResult{
			Part:           p,
			Confidence:     0.6, // lower — no vehicle context
			ConfidenceNote: "Text search without vehicle context",
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  brand.String,
		}
		if partCC.Valid {
			result.FitsVehicleCC = int(partCC.Int32)
		}

		resp.Results = append(resp.Results, result)
	}

	resp.Total = len(resp.Results)

	// TecDoc fulltext search fallback (searchindex 5.8M rows)
	if len(resp.Results) == 0 && s.tecdoc != nil {
		tdRefs, tdErr := s.tecdoc.SearchByKeyword(text, limit)
		if tdErr == nil && len(tdRefs) > 0 {
			resp.SearchStrategy = "tecdoc_fulltext"
			for _, ref := range tdRefs {
				rule := ClassifyCategory(ref.Description)
				result := SmartResult{
					Part: model.Part{
						LegacyArticleId: ref.LegacyArticleId,
						ArticleNumber:   ref.ArticleNumber,
						Description:     ref.Description,
						BrandName:       ref.BrandName,
					},
					Confidence:     0.55,
					ConfidenceNote: "TecDoc fulltext search (no vehicle context)",
					FitmentDriver:  driverName(rule.Driver),
					BrandResolved:  ref.BrandName,
				}
				resp.Results = append(resp.Results, result)
			}
			resp.Total = len(resp.Results)
		}
	}

	if len(resp.Results) == 0 {
		resp.Warnings = append(resp.Warnings, "No parts found for text: "+text+". Try an OEM number or browse by vehicle.")
	}

	return resp, nil
}

// computeConfidence calculates a confidence score based on category rules.
// Returns (0.0-1.0 confidence, human-readable note).
func (s *SmartSearch) computeConfidence(rule CategoryRule, vehicleCC, partCC int, vehicleFuel, partFuel string, legacyArticleId, linkageTargetId int) (float64, string) {
	// If we have a linkageTargetId, check if this part is actually in hk_parts_cache for it
	if linkageTargetId > 0 {
		var exists int
		err := logQueryRow(s.db, "SmartSearch.computeConfidence.fitCheck",
			"SELECT 1 FROM hk_parts_cache WHERE linkingTargetId = ? AND legacyArticleId = ? LIMIT 1",
			linkageTargetId, legacyArticleId).Scan(&exists)
		if err == nil {
			return 0.95, "Direct fitment confirmed in parts catalog"
		}
	}

	return s.computeConfidenceForVehicle(rule, vehicleCC, partCC, vehicleFuel, partFuel)
}

// computeConfidenceForVehicle calculates confidence without checking specific linkage.
func (s *SmartSearch) computeConfidenceForVehicle(rule CategoryRule, vehicleCC, partCC int, vehicleFuel, partFuel string) (float64, string) {
	switch rule.Driver {
	case FitEngine:
		if vehicleCC == 0 {
			return 0.7, "Engine-dependent part — provide VIN for accurate matching"
		}
		if partCC == 0 {
			return 0.7, "Engine-dependent part — vehicle variant CC unknown"
		}
		diff := vehicleCC - partCC
		if diff < 0 {
			diff = -diff
		}
		margin := rule.CCMargin
		if margin == 0 {
			margin = 500
		}
		if diff == 0 {
			return 0.95, "Engine CC exact match"
		}
		if diff <= margin {
			return 0.85, fmt.Sprintf("Engine CC within ±%dcc (diff: %dcc)", margin, diff)
		}
		if diff <= margin*2 {
			return 0.5, fmt.Sprintf("Engine CC marginal: %dcc difference (limit: ±%dcc)", diff, margin)
		}
		return 0.2, fmt.Sprintf("Engine CC mismatch: %dcc difference — likely wrong part", diff)

	case FitBody:
		return 0.85, "Body-dependent part — matched by model/generation"

	case FitDrivetrain:
		return 0.80, "Drivetrain-dependent part — verify drive type (FWD/AWD/RWD)"

	case FitBrake:
		if vehicleCC > 0 && partCC > 0 {
			diff := vehicleCC - partCC
			if diff < 0 {
				diff = -diff
			}
			if diff <= 1000 {
				return 0.85, "Brake part — CC match within tolerance"
			}
			return 0.6, "Brake part — CC differs, may vary by trim/sport package"
		}
		return 0.75, "Brake part — may vary by trim level"

	case FitUniversal:
		return 0.90, "Universal fitment — fits by physical dimensions"
	}

	return 0.7, "Unknown category"
}

// GetCategories returns all distinct part categories for a vehicle.
func (s *SmartSearch) GetCategories(linkageTargetId int) ([]model.CategoryInfo, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	query := `
		SELECT genericArticleDesc, COUNT(DISTINCT legacyArticleId) AS partCount
		FROM hk_parts_cache
		WHERE linkingTargetId = ?
		  AND genericArticleDesc IS NOT NULL AND genericArticleDesc != ''
		GROUP BY genericArticleDesc
		ORDER BY partCount DESC`

	rows, err := s.db.Query(query, linkageTargetId)
	if err != nil {
		return nil, fmt.Errorf("categories: %w", err)
	}
	defer rows.Close()

	var cats []model.CategoryInfo
	for rows.Next() {
		var c model.CategoryInfo
		if err := rows.Scan(&c.Name, &c.PartCount); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		rule := ClassifyCategory(c.Name)
		c.FitmentDriver = driverName(rule.Driver)
		cats = append(cats, c)
	}
	return cats, nil
}

// helpers

// GetOEMNumbers delegates to CrossRef service.
func (s *SmartSearch) GetOEMNumbers(legacyArticleId int) ([]model.OEMReference, error) {
	return s.crossRef.FindOEMNumbers(legacyArticleId)
}

// GetVehiclesForArticle delegates to CrossRef service.
func (s *SmartSearch) GetVehiclesForArticle(legacyArticleId, vehicleCC int, category string, limit int) ([]model.Vehicle, error) {
	return s.crossRef.FindVehiclesForArticle(legacyArticleId, vehicleCC, category, limit)
}

func driverName(d FitmentDriver) string {
	switch d {
	case FitEngine:
		return "engine"
	case FitBody:
		return "body"
	case FitDrivetrain:
		return "drivetrain"
	case FitBrake:
		return "brake"
	case FitUniversal:
		return "universal"
	}
	return "unknown"
}

func looksLikeOEMNumber(q string) bool {
	if len(q) < 5 {
		return false
	}
	// HK OEM numbers always start with a digit (e.g. 26300-35505, 97133-D3000)
	// Aftermarket articles often start with letters (W 811/80, OC 205, C 26 013)
	if q[0] < '0' || q[0] > '9' {
		return false
	}
	digits := 0
	dashes := 0
	for _, c := range q {
		if c >= '0' && c <= '9' {
			digits++
		}
		if c == '-' || c == ' ' {
			dashes++
		}
	}
	// High digit ratio with dashes = likely OEM
	return digits >= 4 && dashes >= 1
}

func looksLikeArticleNumber(q string) bool {
	if len(q) < 3 {
		return false
	}
	// Aftermarket article numbers are typically alphanumeric codes
	// e.g. DRA1919, J1320561, 0986035731
	upper := strings.ToUpper(q)
	hasLetter := false
	hasDigit := false
	for _, c := range upper {
		if c >= 'A' && c <= 'Z' {
			hasLetter = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	// If it's purely numeric with 5+ digits, likely an article number
	if !hasLetter && hasDigit && len(q) >= 5 {
		return true
	}
	// Mix of letters and digits, no spaces, no hyphens = article number
	if hasLetter && hasDigit && !strings.ContainsAny(q, "- ") {
		return true
	}
	// If first char is a letter followed by digits = article number pattern
	if hasLetter && hasDigit {
		_, err := strconv.Atoi(q)
		return err != nil && !strings.ContainsRune(q, '-')
	}
	return false
}
