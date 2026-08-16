package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

// SmartSearch provides category-aware parts search with cross-reference expansion.
type SmartSearch struct {
	db               *sql.DB
	parts            *PartsLookup
	crossRef         *CrossRef
	oem              *OEMLookup
	platform         *Platform
	onlineLookup     *PartsOuqService
	dealerLookup     *DealerLookup
	tecdoc           *TecDoc
	tecDocCrossRef   *TecDocCrossRef
	tecDocSpecs      *TecDocSpecifications
	tecDocDocs       *TecDocDocuments
	tecDocSuper      *TecDocSupersession
	tecDocFunctional *TecDocFunctional
	tecDocVehicle    *TecDocVehicle
	dependency       *DependencyClassifier
	offline          bool
	queries          *store.Queries
	// Circuit breaker state — per-instance to avoid cross-test contamination.
	cbFailures sync.Map // string → *atomic.Int64
	cbDisabled sync.Map // string → time.Time
}

func NewSmartSearch(db *sql.DB, parts *PartsLookup, crossRef *CrossRef, oem *OEMLookup, platform *Platform, online *PartsOuqService, offline bool) *SmartSearch {
	ss := &SmartSearch{db: db, parts: parts, crossRef: crossRef, oem: oem, platform: platform, onlineLookup: online, offline: offline}
	if db != nil {
		ss.queries = store.New(db)
	}
	return ss
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

// SetTecDocCrossRef attaches the TecDoc articlecrosses service (S2-T1).
// When set, searchByOEM will supplement Postgres results with articlecrosses
// (30M rows) to increase aftermarket brand coverage.
func (s *SmartSearch) SetTecDocCrossRef(cr *TecDocCrossRef) {
	s.tecDocCrossRef = cr
}

// SetTecDocSpecifications attaches the TecDoc specifications enrichment service (S3).
func (s *SmartSearch) SetTecDocSpecifications(sp *TecDocSpecifications) { s.tecDocSpecs = sp }

// SetTecDocDocuments attaches the TecDoc documents enrichment service (S3).
func (s *SmartSearch) SetTecDocDocuments(d *TecDocDocuments) { s.tecDocDocs = d }

// SetTecDocSupersession attaches the TecDoc supersession enrichment service (S3).
func (s *SmartSearch) SetTecDocSupersession(su *TecDocSupersession) { s.tecDocSuper = su }

// SetTecDocFunctional attaches the TecDoc functional equivalents service (S3).
func (s *SmartSearch) SetTecDocFunctional(f *TecDocFunctional) { s.tecDocFunctional = f }

// SetTecDocVehicle attaches the TecDoc vehicle compatibility service (S3).
func (s *SmartSearch) SetTecDocVehicle(v *TecDocVehicle) { s.tecDocVehicle = v }

// SmartResult is an enhanced part result with confidence and cross-ref data.
type SmartResult struct {
	model.Part
	Confidence              float64                    `json:"confidence"`
	ConfidenceNote          string                     `json:"confidenceNote,omitempty"`
	FitmentDriver           string                     `json:"fitmentDriver"`
	OEMNumbers              []model.OEMReference       `json:"oemNumbers,omitempty"`
	BrandResolved           string                     `json:"brand,omitempty"`
	FitsVehicleCC           int                        `json:"fitsVehicleCC,omitempty"`
	Substitutions           []model.SubstitutionPart   `json:"substitutions,omitempty"`
	AftermarketAlternatives []model.AftermarketPart    `json:"aftermarketAlternatives,omitempty"`
	Compatibility           []string                   `json:"compatibility,omitempty"`
	// S3: enrichment fields
	Specifications      []model.Specification    `json:"specifications,omitempty"`
	Documents           []model.Document         `json:"documents,omitempty"`
	Supersession        *model.SupersessionChain `json:"supersession,omitempty"`
	FunctionalEquivalents []model.OEMReference   `json:"functionalEquivalents,omitempty"`
	CompatibleVehicles  []model.CompatibleVehicle `json:"compatibleVehicles,omitempty"`
	SourceStrategy      string                   `json:"sourceStrategy,omitempty"`
}

// SmartSearchResponse is the full smart search response.
type SmartSearchResponse struct {
	Query          string         `json:"query"`
	Vehicle        *model.Vehicle `json:"vehicle,omitempty"`
	Results        []SmartResult  `json:"results"`
	Total          int            `json:"total"`
	Categories     []string       `json:"categories,omitempty"`
	SearchStrategy string         `json:"searchStrategy"`
	Mode           string         `json:"mode,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// Search runs the smart search engine.
// It detects query type (OEM number, part number, category text, VIN),
// then applies category-aware filtering with cross-reference expansion.
//
// mode selects a specific search strategy ("exact_oem", "cross_reference",
// "vehicle_fitment", "supersession", "cross_brand", "spec_match",
// "assembly_context", "vin_assembly", "combined"). Empty string runs the
// legacy cascade (backward compat).
//
// enrichmentLevel controls post-search enrichment:
//   "none"  — no TecDoc enrichment (fastest)
//   "basic" — specifications + compatible vehicles only
//   "full"  — all: specs + docs + supersession + functional equivalents + compatible vehicles
// Empty string defaults to "basic".
func (s *SmartSearch) Search(query string, linkageTargetId int, vehicleCC int, fuelType string, category string, page, limit int) (*SmartSearchResponse, error) {
	return s.SearchWithOptions(query, linkageTargetId, vehicleCC, fuelType, category, page, limit, "", "basic")
}

// SearchWithOptions is the full entry point used by the handler (S3/S4).
func (s *SmartSearch) SearchWithOptions(query string, linkageTargetId, vehicleCC int, fuelType, category string, page, limit int, mode, enrichmentLevel string) (*SmartSearchResponse, error) {
	var resp *SmartSearchResponse
	var err error

	if mode != "" && mode != "combined" {
		resp, err = s.searchByMode(query, linkageTargetId, vehicleCC, fuelType, category, page, limit, mode)
	} else if mode == "combined" {
		resp, err = s.searchCombined(query, linkageTargetId, vehicleCC, fuelType, category, page, limit)
	} else {
		resp, err = s.searchDispatch(query, linkageTargetId, vehicleCC, fuelType, category, page, limit)
	}

	if resp != nil {
		before := len(resp.Results)
		resp.Results = dedupeByArticleId(resp.Results)
		if len(resp.Results) != before {
			log.Printf("[SmartSearch.Search] dedupeByArticleId removed %d duplicate(s)", before-len(resp.Results))
		}
		resp.Total = len(resp.Results)
		resp.Mode = mode

		if enrichmentLevel != "none" && len(resp.Results) > 0 {
			resp.Results = s.enrichResults(resp.Results, enrichmentLevel)
		}
	}
	return resp, err
}

// searchDispatch holds the actual strategy-selection cascade. It is the pre-S0-T2
// body of Search; kept intact so downstream returns can continue to build a
// response with whatever total count they compute — dedupeByArticleId in Search()
// will normalize resp.Total against the final unique-result slice.
func (s *SmartSearch) searchDispatch(query string, linkageTargetId int, vehicleCC int, fuelType string, category string, page, limit int) (*SmartSearchResponse, error) {
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
		// OEM search found nothing — try article lookup (owned catalog).
		// S0-T1: DO NOT fall through to searchByText for OEM queries. That path
		// invokes SearchByKeyword (tecdoc_keyword strategy) which returns
		// wrong-category garbage for OEM misses (root cause of BUG-1, BUG-5,
		// BUG-9, BUG-10, BUG-11). searchByArticle is safe because it hits the
		// owned catalog by exact article number. If both miss, return the OEM
		// response with a warning so the caller sees the miss instead of a
		// misleading full-text hit.
		artResp, err := s.searchByArticle(query, linkageTargetId, vehicleCC, limit)
		if err == nil && artResp.Total > 0 {
			return artResp, nil
		}
		resp.Warnings = append(resp.Warnings,
			fmt.Sprintf("OEM %q not found; text-search fallback disabled to prevent wrong-category matches (S0-T1)", query))
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
		// S0-T3: always run the vehicle search first. If it returns results, return them.
		// If it returns zero results AND the query is non-empty text (BUG-3), merge in a
		// text-search pass so the user still gets results instead of nil.
		vResp, vErr := s.searchByVehicle(query, linkageTargetId, vehicleCC, fuelType, category, page, limit)
		if vErr != nil {
			return vResp, vErr
		}
		if vResp.Total > 0 || strings.TrimSpace(query) == "" {
			return vResp, nil
		}
		// Vehicle search missed with a text query — fall back to text search and tag results.
		textResp, textErr := s.searchByText(query, vehicleCC, fuelType, page, limit)
		if textErr != nil || textResp.Total == 0 {
			// Return the empty vehicle response (preserves vehicle context + categories).
			return vResp, nil
		}
		textResp.Warnings = append(textResp.Warnings,
			"No vehicle-linked parts found; showing text-search results — vehicle fitment not confirmed")
		return textResp, nil
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

	// ─── HK scope gate ────────────────────────────────────────────────
	// Before running any online / dealer / supersession fallback, verify
	// this even looks like a Hyundai/Kia OEM. If not, we short-circuit
	// with an honest "not in scope" message — the app is documented as
	// HK-only (README.md, ARCHITECTURE.md §data-source-boundaries).
	//
	// Owned-catalog and cross-ref lookups still run (they only ever hold
	// HK-scoped data anyway), so a valid HK OEM disguised in a weird
	// format still gets a chance via the seeded corpus below.
	scope := IsHKOEM(oemNum)
	if !scope.IsHK {
		log.Printf("[SmartSearch.searchByOEM] REJECTED by HK-scope gate: %s", scope.Reason)
		resp.SearchStrategy = "hk_scope_rejected"
		resp.Warnings = append(resp.Warnings, scope.Reason)
		if scope.SuggestedMake != "" {
			resp.Warnings = append(resp.Warnings,
				"Try the parts distributor for "+scope.SuggestedMake+" instead.")
		}
		resp.Total = 0
		return resp, nil
	}

	// Track merged OEM references across all sources. Declared here so the
	// MySQL/TecDoc STEP 0 short-circuit can populate it before the local
	// CrossRef step runs.
	var refs []model.OEMReference
	var err error
	var oemResult *model.OEMSearchResult // hoisted past `goto buildResults`

	// Step 0: MySQL/TecDoc — the source of truth for HK parts.
	// The 21.5M-row oem_number table + articlesvehicletrees join is the
	// authoritative catalog. Local Postgres + SQLite are enrichment caches,
	// not the authority. When MySQL is reachable we consult it FIRST and
	// merge its refs into the result set; local sources still contribute
	// but never override a MySQL hit for the same article.
	if s.tecdoc != nil {
		log.Printf("[SmartSearch.searchByOEM] STEP 0: TecDoc.SearchByOEM (MySQL — source of truth)")
		tdRefs, tdErr := s.tecdoc.SearchByOEM(oemNum, limit)
		if tdErr != nil {
			log.Printf("[SmartSearch.searchByOEM] STEP 0 ERROR (non-fatal, falling back to local): %v", tdErr)
		} else if len(tdRefs) > 0 {
			log.Printf("[SmartSearch.searchByOEM] STEP 0 HIT: TecDoc returned %d refs (elapsed=%v)", len(tdRefs), time.Since(start))
			resp.SearchStrategy = "tecdoc_oem"
			// Return TecDoc results immediately — local cache queries below
			// would only duplicate them. Aftermarket cross-refs still get
			// added via enrichAftermarket() after the buildResults section.
			refs = tdRefs
			goto buildResults
		}
	}

	// Step 1: CrossRef service (oem_search_index — indexed)
	log.Printf("[SmartSearch.searchByOEM] STEP 1: CrossRef.FindByOEM")
	refs, err = s.crossRef.FindByOEM(oemNum, limit)
	if err != nil {
		log.Printf("[SmartSearch.searchByOEM] STEP 1 ERROR: %v (elapsed=%v)", err, time.Since(start))
		return nil, err
	}
	log.Printf("[SmartSearch.searchByOEM] STEP 1 DONE: %d refs (elapsed=%v)", len(refs), time.Since(start))

	// Step 2: Also check oem_search_index
	log.Printf("[SmartSearch.searchByOEM] STEP 2: OEMLookup.Search")
	oemResult, _ = s.oem.Search(oemNum, limit)
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

	// An exact owned-catalog part-number record is stronger evidence than a
	// cross-reference. Do not return it for a selected vehicle unless its
	// catalog fitment link confirms compatibility.
	if s.parts != nil {
		directParts, directErr := s.parts.FindByArticleNumber(oemNum, linkageTargetId, limit)
		if directErr != nil {
			return nil, fmt.Errorf("exact catalog OEM match: %w", directErr)
		}
		for _, part := range directParts {
			duplicate := false
			for _, existing := range refs {
				if existing.LegacyArticleId == part.LegacyArticleId {
					duplicate = true
					break
				}
			}
			if !duplicate {
				refs = append(refs, model.OEMReference{
					RawNumber:       oemNum,
					Normalized:      NormalizeOEM(oemNum),
					LegacyArticleId: part.LegacyArticleId,
					Manufacturer:    "HYUNDAI/KIA",
					BrandName:       part.BrandName,
					ArticleNumber:   part.ArticleNumber,
					Description:     part.Description,
				})
			}
		}
	}
	sortOEMReferences(refs, oemNum)

	log.Printf("[SmartSearch.searchByOEM] MERGED refs=%d (elapsed=%v)", len(refs), time.Since(start))

	// S2-T1: TecDoc articlecrosses (30M rows) — runs regardless of existing refs.
	// This is the primary aftermarket brand coverage source. The Postgres
	// oem_search_index (~1,700 rows) is orders of magnitude smaller; articlecrosses
	// is the authoritative OEM→aftermarket cross-reference table.
	// Runs unconditionally so that even when Postgres already has hits we still
	// surface additional aftermarket brands from TecDoc (brand coverage 7.6% → ~50%).
	if s.tecDocCrossRef != nil {
		log.Printf("[SmartSearch.searchByOEM] STEP 2b: TecDoc.SearchCrossReferences")
		tdCrossRefs, tdCrossErr := s.tecDocCrossRef.SearchCrossReferences(oemNum, limit)
		if tdCrossErr == nil && len(tdCrossRefs) > 0 {
			log.Printf("[SmartSearch.searchByOEM] STEP 2b HIT: %d cross-refs (elapsed=%v)", len(tdCrossRefs), time.Since(start))
			for _, ref := range tdCrossRefs {
				dup := false
				for _, existing := range refs {
					if existing.LegacyArticleId == ref.LegacyArticleId {
						dup = true
						break
					}
				}
				if !dup {
					refs = append(refs, ref)
				}
			}
			if resp.SearchStrategy == "oem_crossref" {
				resp.SearchStrategy = "tecdoc_crossref"
			}
		} else if tdCrossErr != nil {
			log.Printf("[SmartSearch.searchByOEM] STEP 2b ERROR: %v (elapsed=%v)", tdCrossErr, time.Since(start))
		}
	}

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
					// Reject scraped UI chrome ("Sign up with", "Login",
					// "Cookie Preferences", etc.) that would otherwise be
					// surfaced as a 0.75-confidence part.
					if IsJunkDescription(onlineResult.Description) {
						log.Printf("[SmartSearch.searchByOEM] online result rejected as junk description: %q", onlineResult.Description)
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
						if IsJunkDescription(onlineResult.Description) {
							log.Printf("[SmartSearch.searchByOEM] online (stripped) result rejected as junk description: %q", onlineResult.Description)
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
			if dealerResult != nil && dealerResult.Description != "" && !IsJunkDescription(dealerResult.Description) {
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
			if dealerResult != nil && IsJunkDescription(dealerResult.Description) {
				log.Printf("[SmartSearch.searchByOEM] dealer result rejected as junk description: %q", dealerResult.Description)
			}
		}

		// Strategy 7: Reverse supersession — check if any cached part lists this as a substitution
		log.Printf("[SmartSearch.searchByOEM] STEP 9: reverse supersession")
		if s.onlineLookup != nil && s.onlineLookup.GetCache() != nil {
			if superseded := s.onlineLookup.GetCache().FindBySubstitution(NormalizeOEM(oemNum)); superseded != nil && !IsJunkDescription(superseded.Description) {
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
		if NormalizeOEM(ref.ArticleNumber) == NormalizeOEM(oemNum) {
			conf = 0.96
			note = "Exact part-number match in the owned catalog"
		}

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

// dedupeByArticleId removes duplicate SmartResult entries whose
// Part.LegacyArticleId matches an earlier entry. First occurrence wins.
//
// Entries with LegacyArticleId == 0 (online lookups, dealer scrapes,
// aftermarket_crossref_only responses that carry no TecDoc article id) are kept
// as-is because they cannot be reliably deduplicated by id — de-duplication of
// those would require string-normalized brand+articleNumber compare, out of
// scope for S0-T2.
//
// This is the single global dedup hook for S0-T2 (fixes BUG-2, BUG-7, BUG-8,
// BUG-12: same legacyArticleId appearing twice in results).
func dedupeByArticleId(results []SmartResult) []SmartResult {
	if len(results) < 2 {
		return results
	}
	seen := make(map[int]bool, len(results))
	out := make([]SmartResult, 0, len(results))
	for _, r := range results {
		if r.LegacyArticleId == 0 {
			// No reliable id to key on — pass through.
			out = append(out, r)
			continue
		}
		if seen[r.LegacyArticleId] {
			continue
		}
		seen[r.LegacyArticleId] = true
		out = append(out, r)
	}
	return out
}

func sortOEMReferences(refs []model.OEMReference, query string) {
	normalizedQuery := NormalizeOEM(query)
	sort.SliceStable(refs, func(i, j int) bool {
		return oemReferenceRank(refs[i], normalizedQuery) > oemReferenceRank(refs[j], normalizedQuery)
	})
}

func oemReferenceRank(ref model.OEMReference, normalizedQuery string) int {
	rank := 0
	if NormalizeOEM(ref.ArticleNumber) == normalizedQuery {
		rank += 1000
	}
	if NormalizeOEM(ref.RawNumber) == normalizedQuery {
		rank += 100
	}
	brand := strings.ToUpper(ref.BrandName + " " + ref.Manufacturer)
	if strings.Contains(brand, "HYUNDAI") || strings.Contains(brand, "KIA") {
		rank += 10
	}
	return rank
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

	// Step 0: MySQL/TecDoc — the oem_number index is authoritative for
	// article-number lookups too (an aftermarket article-number often
	// exists in the same 21.5M-row index under source_table=articles).
	if s.tecdoc != nil {
		tdRefs, tdErr := s.tecdoc.SearchByOEM(artNum, limit)
		if tdErr == nil && len(tdRefs) > 0 {
			log.Printf("[SmartSearch.searchByArticle] STEP 0 HIT: TecDoc returned %d refs", len(tdRefs))
			resp.SearchStrategy = "tecdoc_article"
			for _, ref := range tdRefs {
				rule := ClassifyCategory(ref.Description)
				resp.Results = append(resp.Results, SmartResult{
					Part: model.Part{
						LegacyArticleId: ref.LegacyArticleId,
						ArticleNumber:   ref.ArticleNumber,
						Description:     ref.Description,
						BrandName:       ref.BrandName,
					},
					Confidence:     0.85,
					ConfidenceNote: "TecDoc article lookup (MySQL source of truth)",
					FitmentDriver:  driverName(rule.Driver),
					BrandResolved:  ref.BrandName,
					OEMNumbers:     []model.OEMReference{ref},
				})
			}
			resp.Total = len(resp.Results)
			s.enrichAftermarket(resp)
			return resp, nil
		}
		if tdErr != nil {
			log.Printf("[SmartSearch.searchByArticle] STEP 0 ERROR (falling back to local): %v", tdErr)
		}
	}

	normalized := strings.ToUpper(strings.TrimSpace(artNum))

	rows, err := s.queries.SearchByArticleNumber(context.Background(), store.SearchByArticleNumberParams{
		Upper: normalized,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("article search: %w", err)
	}

	for _, row := range rows {
		p := model.Part{
			LegacyArticleId: int(row.LegacyArticleID),
			ArticleNumber:   row.ArticleNumber.String,
			Description:     row.GenericArticleDesc.String,
			BrandName:       row.BrandName.String,
			Category:        row.CategoryName.String,
			AssemblyGroupId: int(row.AssemblyGroupNodeID),
		}

		rule := ClassifyCategory(p.Description)
		var fitsCC int
		if row.CapacityCc.Valid {
			fitsCC = int(row.CapacityCc.Int32)
		}

		conf, note := s.computeConfidence(rule, vehicleCC, fitsCC, "", "", p.LegacyArticleId, linkageTargetId)

		result := SmartResult{
			Part:           p,
			Confidence:     conf,
			ConfidenceNote: note,
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  row.BrandName.String,
			FitsVehicleCC:  fitsCC,
		}

		// Fetch OEM cross-refs
		oems, _ := s.crossRef.FindOEMNumbers(p.LegacyArticleId)
		if len(oems) > 0 {
			result.OEMNumbers = oems
		}

		resp.Results = append(resp.Results, result)
	}

	// Fallback: if article lookup found nothing and query looks like it could be a dashless OEM number,
	// try OEM search with common dash positions (Hyundai/Kia OEM format: XXXXX-XXXXX)
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
	if s.queries == nil || len(normalizedPrefix) < 8 {
		return nil
	}
	rows, err := s.queries.SearchOEMPrefix(context.Background(), store.SearchOEMPrefixParams{
		Normalized: normalizedPrefix + "%",
		Limit:      int32(limit),
	})
	if err != nil {
		return nil
	}

	var refs []model.OEMReference
	seen := make(map[int]bool)
	for _, row := range rows {
		ref := model.OEMReference{
			RawNumber:       row.RawNumber,
			Normalized:      row.Normalized,
			LegacyArticleId: int(row.LegacyArticleID),
			Manufacturer:    row.MfrName.String,
			BrandName:       row.BrandName.String,
			ArticleNumber:   row.ArticleNumber.String,
			Description:     row.Description.String,
		}
		if seen[ref.LegacyArticleId] {
			continue
		}
		seen[ref.LegacyArticleId] = true
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

	// Step 0: MySQL/TecDoc parts-for-vehicle — source of truth. When TecDoc
	// is wired and the vehicle is known, its articlesvehicletrees join is
	// authoritative. We consult it FIRST; if empty or unavailable, we fall
	// through to the local Postgres store below.
	if s.tecdoc != nil && linkageTargetId > 0 {
		tdParts, tdTotal, tdErr := s.tecdoc.PartsForVehicle(linkageTargetId, category, page, limit)
		if tdErr == nil && len(tdParts) > 0 {
			log.Printf("[SmartSearch.searchByVehicle] STEP 0 HIT: TecDoc returned %d/%d parts for vehicle=%d",
				len(tdParts), tdTotal, linkageTargetId)
			resp.SearchStrategy = "tecdoc_vehicle"
			resp.Results = tdParts
			resp.Total = tdTotal
			s.enrichAftermarket(resp)
			return resp, nil
		}
		if tdErr != nil {
			log.Printf("[SmartSearch.searchByVehicle] STEP 0 ERROR (falling back to local): %v", tdErr)
		}
	}

	normalizedFilter := normalizeTextSearchQuery(textFilter)

	offset := (page - 1) * limit

	total, err := s.queries.CountVehicleSearchParts(context.Background(), store.CountVehicleSearchPartsParams{
		LinkingTargetID: int32(linkageTargetId),
		Column2:         category,
		Column3:         normalizedFilter,
	})
	if err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}
	resp.Total = int(total)

	catRows, err := s.queries.ListVehicleSearchCategories(context.Background(), int32(linkageTargetId))
	if err == nil {
		for _, c := range catRows {
			if c.Valid && c.String != "" {
				resp.Categories = append(resp.Categories, c.String)
			}
		}
	}

	rows, err := s.queries.ListVehicleSearchParts(context.Background(), store.ListVehicleSearchPartsParams{
		LinkingTargetID: int32(linkageTargetId),
		Column2:         category,
		Column3:         normalizedFilter,
		Limit:           int32(limit),
		Offset:          int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("vehicle search: %w", err)
	}

	for _, row := range rows {
		p := model.Part{
			LegacyArticleId: int(row.LegacyArticleID),
			ArticleNumber:   row.ArticleNumber.String,
			Description:     row.GenericArticleDesc.String,
			BrandName:       row.BrandName.String,
			Category:        row.CategoryName.String,
			AssemblyGroupId: int(row.AssemblyGroupNodeID),
		}

		rule := ClassifyCategory(p.Description)
		var fitsCC int
		if row.CapacityCc.Valid {
			fitsCC = int(row.CapacityCc.Int32)
		}

		conf, note := s.computeConfidenceForVehicle(rule, vehicleCC, fitsCC, fuelType, row.FuelType.String)

		result := SmartResult{
			Part:           p,
			Confidence:     conf,
			ConfidenceNote: note,
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  row.BrandName.String,
			FitsVehicleCC:  fitsCC,
		}

		resp.Results = append(resp.Results, result)
	}

	// S2-T4: enrich each result with articlecrosses OEM numbers.
	// This surfaces aftermarket alternatives for parts found via vehicle search
	// that have no cross-reference entry in the Postgres oem_search_index.
	// Batched: collect all legacyArticleIds, one cross-ref lookup per article.
	if s.tecDocCrossRef != nil && len(resp.Results) > 0 {
		for i, result := range resp.Results {
			if result.LegacyArticleId <= 0 {
				continue
			}
			// Use the article ID as a proxy OEM key — TecDoc cross-ref can resolve it
			// to OEM numbers via the articlecrosses table.
			oemNums, oemErr := s.oem.OEMNumbersForArticle(result.LegacyArticleId)
			if oemErr != nil || len(oemNums) == 0 {
				continue
			}
			// Take the first OEM number and look up cross-references for richer brand data.
			if refs, refErr := s.tecDocCrossRef.SearchCrossReferences(oemNums[0], 20); refErr == nil {
				var oemRefs []model.OEMReference
				for _, ref := range refs {
					oemRefs = append(oemRefs, ref)
				}
				resp.Results[i].OEMNumbers = oemRefs
			}
		}
	}

	return resp, nil
}

// searchByText does a text search across parts descriptions and article numbers.
func (s *SmartSearch) searchByText(text string, vehicleCC int, fuelType string, page, limit int) (*SmartSearchResponse, error) {
	resp := &SmartSearchResponse{
		Query:          text,
		SearchStrategy: "text_search",
	}

	// Step 0: MySQL/TecDoc keyword search — source of truth. Consult FIRST.
	// Falls through to the local Postgres text index if TecDoc is not wired
	// or returns nothing.
	if s.tecdoc != nil {
		tdRefs, tdErr := s.tecdoc.SearchByKeyword(text, limit)
		if tdErr == nil && len(tdRefs) > 0 {
			log.Printf("[SmartSearch.searchByText] STEP 0 HIT: TecDoc keyword returned %d refs", len(tdRefs))
			resp.SearchStrategy = "tecdoc_keyword"
			for _, ref := range tdRefs {
				rule := ClassifyCategory(ref.Description)
				resp.Results = append(resp.Results, SmartResult{
					Part: model.Part{
						LegacyArticleId: ref.LegacyArticleId,
						ArticleNumber:   ref.ArticleNumber,
						Description:     ref.Description,
						BrandName:       ref.BrandName,
					},
					Confidence:     0.65,
					ConfidenceNote: "TecDoc keyword search (MySQL source of truth)",
					FitmentDriver:  driverName(rule.Driver),
					BrandResolved:  ref.BrandName,
					OEMNumbers:     []model.OEMReference{ref},
				})
			}
			resp.Total = len(resp.Results)
			s.enrichAftermarket(resp)
			return resp, nil
		}
		if tdErr != nil {
			log.Printf("[SmartSearch.searchByText] STEP 0 ERROR (falling back to local): %v", tdErr)
		}
	}

	offset := (page - 1) * limit
	normalizedText := normalizeTextSearchQuery(text)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.queries.SearchByText(ctx, store.SearchByTextParams{
		RegexpSplitToArray: normalizedText,
		Limit:              int32(limit),
		Offset:             int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("text search: %w", err)
	}
	for _, row := range rows {
		p := model.Part{
			LegacyArticleId: int(row.LegacyArticleID),
			ArticleNumber:   row.ArticleNumber.String,
			Description:     row.GenericArticleDesc.String,
			BrandName:       row.BrandName.String,
			Category:        row.CategoryName.String,
			AssemblyGroupId: int(row.AssemblyGroupNodeID),
		}

		rule := ClassifyCategory(p.Description)
		result := SmartResult{
			Part:           p,
			Confidence:     0.6, // lower — no vehicle context
			ConfidenceNote: "Text search without vehicle context",
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  row.BrandName.String,
		}
		if row.CapacityCc.Valid {
			result.FitsVehicleCC = int(row.CapacityCc.Int32)
		}

		resp.Results = append(resp.Results, result)
	}

	resp.Total = len(resp.Results)

	// TecDoc fulltext search fallback (searchindex 5.8M rows).
	// S0-T4: gate results by category token. Classify the query text to get the
	// expected category; reject any returned article whose description does not
	// match that category. This prevents BUG-1 (e.g. "oil filter" returning
	// fuel filters or engine mounts via tecdoc_keyword).
	if len(resp.Results) == 0 && s.tecdoc != nil {
		tdRefs, tdErr := s.tecdoc.SearchByKeyword(text, limit)
		if tdErr == nil && len(tdRefs) > 0 {
			queryRule := ClassifyCategory(text)
			resp.SearchStrategy = "tecdoc_fulltext"
			for _, ref := range tdRefs {
				artRule := ClassifyCategory(ref.Description)
				// Hard gate: reject the result when the query has a clear category
				// (not FitUniversal) and the article maps to a different category.
				if queryRule.Driver != FitUniversal && artRule.Driver != queryRule.Driver {
					log.Printf("[searchByText] tecdoc_fulltext gated out %q (query_driver=%v article_driver=%v)",
						ref.Description, queryRule.Driver, artRule.Driver)
					continue
				}
				result := SmartResult{
					Part: model.Part{
						LegacyArticleId: ref.LegacyArticleId,
						ArticleNumber:   ref.ArticleNumber,
						Description:     ref.Description,
						BrandName:       ref.BrandName,
					},
					Confidence:     0.55,
					ConfidenceNote: "TecDoc fulltext search (no vehicle context)",
					FitmentDriver:  driverName(artRule.Driver),
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
	if linkageTargetId > 0 && s.queries != nil {
		exists, err := s.queries.CheckPartFitsVehicle(context.Background(), store.CheckPartFitsVehicleParams{
			LinkingTargetID: int32(linkageTargetId),
			LegacyArticleID: int32(legacyArticleId),
		})
		if err == nil && exists {
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
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}

	rows, err := s.queries.ListCategoryInfos(context.Background(), int32(linkageTargetId))
	if err != nil {
		return nil, fmt.Errorf("categories: %w", err)
	}

	var cats []model.CategoryInfo
	for _, row := range rows {
		c := model.CategoryInfo{
			Name:      row.GenericArticleDesc.String,
			PartCount: int(row.PartCount),
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
	// Must start with a digit.
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
	// S2-T3 (BUG-6): all-digit strings of 5+ chars starting with a digit are
	// OEM number stems (e.g. "97133"). Route them as OEM first; if the OEM
	// lookup misses, the cascade falls through to searchByArticle automatically.
	if digits == len(q) && digits >= 5 {
		return true
	}
	// Classic HK OEM pattern: high digit ratio with at least one dash/space.
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
	// S2-T3 (BUG-6): purely numeric strings are now handled by looksLikeOEMNumber
	// (all-digit 5+ char rule). Do not claim them here to avoid the wrong routing.
	if !hasLetter {
		return false
	}
	// Mix of letters and digits = article number pattern
	if hasLetter && hasDigit && !strings.ContainsAny(q, "- ") {
		return true
	}
	if hasLetter && hasDigit {
		_, err := strconv.Atoi(q)
		return err != nil && !strings.ContainsRune(q, '-')
	}
	return false
}
