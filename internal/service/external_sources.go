package service

import (
	"context"
	"database/sql"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

type ExternalSourceStore struct {
	db      *sql.DB
	queries *store.Queries
}

func NewExternalSourceStore(db *sql.DB) *ExternalSourceStore {
	if db == nil {
		return nil
	}
	return &ExternalSourceStore{db: db, queries: store.New(db)}
}

func (s *ExternalSourceStore) SeedCatalog(records []model.ExternalSourceRecord, assessments []model.ExternalSourceAssessment) error {
	if s == nil || s.db == nil || s.queries == nil {
		return nil
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	ctx := context.Background()
	for _, record := range records {
		if err := qtx.UpsertExternalSource(ctx, store.UpsertExternalSourceParams{
			SourceKey:            record.SourceKey,
			DisplayName:          record.DisplayName,
			WebsiteUrl:           record.WebsiteURL,
			DataType:             record.DataType,
			AccessMethod:         record.AccessMethod,
			LicenseRisk:          record.LicenseRisk,
			HyundaiKiaUsefulness: record.HyundaiKiaUsefulness,
			MultiMakeUsefulness:  record.MultiMakeUsefulness,
			FalsePositiveRisk:    record.FalsePositiveRisk,
			Recommendation:       string(record.Recommendation),
			UserFacingEligible:   record.UserFacingEligible,
			FreshnessNotes:       record.FreshnessNotes,
			RateLimitNotes:       record.RateLimitNotes,
			Notes:                record.Notes,
		}); err != nil {
			return err
		}
	}

	for _, assessment := range assessments {
		if err := qtx.UpsertExternalSourceAssessment(ctx, store.UpsertExternalSourceAssessmentParams{
			SourceKey:           assessment.SourceKey,
			SampleScope:         assessment.SampleScope,
			EvidenceScore:       int32(assessment.EvidenceScore),
			PrecisionScore:      int32(assessment.PrecisionScore),
			DuplicateNoiseScore: int32(assessment.DuplicateNoiseScore),
			FalsePositiveScore:  int32(assessment.FalsePositiveScore),
			QaDecision:          assessment.QADecision,
			Rationale:           assessment.Rationale,
		}); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ExternalSourceStore) CountSourcesByRecommendation() (map[string]int, error) {
	counts := map[string]int{}
	if s == nil || s.queries == nil {
		return counts, nil
	}

	rows, err := s.queries.CountSourcesByRecommendation(context.Background())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.Recommendation] = int(row.SourceCount)
	}
	return counts, nil
}

func (s *ExternalSourceStore) ListUserFacingEligible() ([]string, error) {
	if s == nil || s.queries == nil {
		return nil, nil
	}
	return s.queries.ListUserFacingEligibleSources(context.Background(), string(model.ExternalSourceBackendEnrichment))
}

func DefaultExternalSourceCatalog() []model.ExternalSourceRecord {
	return []model.ExternalSourceRecord{
		{SourceKey: "nhtsa_vpic_api", DisplayName: "NHTSA vPIC API", WebsiteURL: "https://vpic.nhtsa.dot.gov/api/", DataType: "vin_decode_vehicle_identity", AccessMethod: "rest_api", LicenseRisk: "very_low", HyundaiKiaUsefulness: "high", MultiMakeUsefulness: "high", FalsePositiveRisk: "very_low", Recommendation: model.ExternalSourceBackendEnrichment, FreshnessNotes: "Live API backed by NHTSA data.", RateLimitNotes: "Safe for app use, but bulk decode should prefer local mirror.", Notes: "Good for vehicle identity and linkage context, not part selection alone."},
		{SourceKey: "nhtsa_vpic_db", DisplayName: "NHTSA vPIC Downloadable DB", WebsiteURL: "https://vpic.nhtsa.dot.gov/downloads/", DataType: "vin_decode_vehicle_identity", AccessMethod: "downloadable_database", LicenseRisk: "very_low", HyundaiKiaUsefulness: "high", MultiMakeUsefulness: "high", FalsePositiveRisk: "very_low", Recommendation: model.ExternalSourceBackendEnrichment, FreshnessNotes: "Monthly downloadable snapshot.", RateLimitNotes: "No runtime rate-limit once mirrored locally.", Notes: "Best free VIN-resolution side source; safer than repeated live API calls."},
		{SourceKey: "nhtsa_recalls_api", DisplayName: "NHTSA Recalls API", WebsiteURL: "https://api.nhtsa.gov/recalls/", DataType: "recall_component_context", AccessMethod: "rest_api", LicenseRisk: "very_low", HyundaiKiaUsefulness: "high", MultiMakeUsefulness: "high", FalsePositiveRisk: "medium", Recommendation: model.ExternalSourceBackendEnrichment, FreshnessNotes: "Live recall data.", RateLimitNotes: "Reasonable for lookup traffic.", Notes: "Useful as a safety-context badge, not as a direct part-number selector."},
		{SourceKey: "nhtsa_complaints_api", DisplayName: "NHTSA Complaints API", WebsiteURL: "https://api.nhtsa.gov/complaints/", DataType: "owner_reported_issue_text", AccessMethod: "rest_api", LicenseRisk: "very_low", HyundaiKiaUsefulness: "medium", MultiMakeUsefulness: "high", FalsePositiveRisk: "high", Recommendation: model.ExternalSourceResearchOnly, FreshnessNotes: "Live complaint feed.", RateLimitNotes: "Reasonable for analyst use.", Notes: "Too unstructured for automatic fitment or replacement claims."},
		{SourceKey: "tecdoc", DisplayName: "TecDoc", WebsiteURL: "https://www.tecalliance.net/", DataType: "fitment_crossref_replacement", AccessMethod: "licensed_database", LicenseRisk: "low", HyundaiKiaUsefulness: "very_high", MultiMakeUsefulness: "very_high", FalsePositiveRisk: "low", Recommendation: model.ExternalSourceBackendEnrichment, FreshnessNotes: "Existing licensed dataset in current stack.", RateLimitNotes: "Local DB access once loaded.", Notes: "Primary precision source for fitment and replacement evidence."},
		{SourceKey: "haynespro", DisplayName: "HaynesPro", WebsiteURL: "https://www.infopro-digital-automotive.com/solutions/data/haynespro-workshopdata/", DataType: "repair_guidance_diagrams", AccessMethod: "commercial_api_or_subscription", LicenseRisk: "low", HyundaiKiaUsefulness: "high", MultiMakeUsefulness: "high", FalsePositiveRisk: "low", Recommendation: model.ExternalSourceBackendEnrichment, FreshnessNotes: "Vendor-managed commercial feed.", RateLimitNotes: "Contract-dependent.", Notes: "Best clean path found for diagrams and install guidance."},
		{SourceKey: "mitchell1_prodemand", DisplayName: "Mitchell1 ProDemand", WebsiteURL: "https://www.mitchell1.com/prodemand/", DataType: "repair_guidance_diagrams", AccessMethod: "subscription_api", LicenseRisk: "low", HyundaiKiaUsefulness: "high", MultiMakeUsefulness: "high", FalsePositiveRisk: "low", Recommendation: model.ExternalSourceBackendEnrichment, FreshnessNotes: "Vendor-managed subscription data.", RateLimitNotes: "Seat/subscription controlled.", Notes: "Viable shop-facing integration path with documented token flow."},
		{SourceKey: "epa_fueleconomy", DisplayName: "EPA FuelEconomy", WebsiteURL: "https://www.fueleconomy.gov/feg/ws/", DataType: "vehicle_engine_variant_metadata", AccessMethod: "rest_api_and_csv", LicenseRisk: "very_low", HyundaiKiaUsefulness: "medium", MultiMakeUsefulness: "high", FalsePositiveRisk: "low", Recommendation: model.ExternalSourceBackendEnrichment, FreshnessNotes: "Public API and yearly downloadable files.", RateLimitNotes: "API observed unreliable; prefer mirrored CSV if adopted.", Notes: "Helpful for engine disambiguation only, not diagrams or replacements."},
		{SourceKey: "wikidata", DisplayName: "Wikidata", WebsiteURL: "https://query.wikidata.org/", DataType: "platform_generation_metadata", AccessMethod: "sparql_endpoint", LicenseRisk: "very_low", HyundaiKiaUsefulness: "medium", MultiMakeUsefulness: "high", FalsePositiveRisk: "medium", Recommendation: model.ExternalSourceResearchOnly, FreshnessNotes: "Community-maintained live knowledge graph.", RateLimitNotes: "Public endpoint etiquette required.", Notes: "Good for seeding platform relationships, not for direct user-facing claims."},
		{SourceKey: "wikimedia_commons", DisplayName: "Wikimedia Commons", WebsiteURL: "https://commons.wikimedia.org/", DataType: "licensed_generic_component_illustrations", AccessMethod: "manual_review_allowlist", LicenseRisk: "medium", HyundaiKiaUsefulness: "low", MultiMakeUsefulness: "medium", FalsePositiveRisk: "high", Recommendation: model.ExternalSourceResearchOnly, FreshnessNotes: "Per-file license and attribution must be revalidated at review time.", RateLimitNotes: "No automated ingestion; approved files enter only through the internal review queue.", Notes: "Generic illustrations only. Never use as OEM part identity, dimensions, or fitment proof."},
		{SourceKey: "hyundai_tech_info", DisplayName: "Hyundai Tech Info", WebsiteURL: "https://www.hyundaitechinfo.com/", DataType: "oem_service_manuals", AccessMethod: "subscription_portal", LicenseRisk: "high", HyundaiKiaUsefulness: "very_high", MultiMakeUsefulness: "low", FalsePositiveRisk: "very_low", Recommendation: model.ExternalSourceResearchOnly, FreshnessNotes: "OEM portal with strong ground truth.", RateLimitNotes: "Subscription and portal restrictions apply.", Notes: "Manual QA reference only unless a direct license is negotiated."},
		{SourceKey: "kia_tech_info", DisplayName: "Kia Tech Info", WebsiteURL: "https://kiatechinfo.snapon.com/", DataType: "oem_service_manuals", AccessMethod: "subscription_portal", LicenseRisk: "high", HyundaiKiaUsefulness: "very_high", MultiMakeUsefulness: "low", FalsePositiveRisk: "very_low", Recommendation: model.ExternalSourceResearchOnly, FreshnessNotes: "OEM portal with strong ground truth.", RateLimitNotes: "Subscription and portal restrictions apply.", Notes: "Manual QA reference only unless a direct license is negotiated."},
		{SourceKey: "oem_retail_diagram_sites", DisplayName: "OEM Retail Diagram Sites", WebsiteURL: "https://www.kiapartsnow.com/", DataType: "retail_catalog_and_diagrams", AccessMethod: "scrape_only", LicenseRisk: "very_high", HyundaiKiaUsefulness: "very_high", MultiMakeUsefulness: "low", FalsePositiveRisk: "low", Recommendation: model.ExternalSourceRejected, FreshnessNotes: "Consumer retail pages can be current but are not safe to ingest.", RateLimitNotes: "Robots and portal restrictions block safe automation.", Notes: "Do not automate against blocked dealer or OEM retail sites."},
		{SourceKey: "rockauto", DisplayName: "RockAuto", WebsiteURL: "https://www.rockauto.com/", DataType: "aftermarket_fitment_catalog", AccessMethod: "scrape_only", LicenseRisk: "very_high", HyundaiKiaUsefulness: "high", MultiMakeUsefulness: "high", FalsePositiveRisk: "medium", Recommendation: model.ExternalSourceRejected, FreshnessNotes: "Broad coverage but not safely licensable from the public site.", RateLimitNotes: "Robots/terms prohibit automated ingestion.", Notes: "Rejected for legal and operational reasons despite coverage."},
	}
}

func DefaultExternalSourceAssessments() []model.ExternalSourceAssessment {
	return []model.ExternalSourceAssessment{
		{SourceKey: "nhtsa_vpic_api", SampleScope: "Hyundai Tucson, Santa Fe, Kia Sportage, Kia Sorento VIN and YMM validation", EvidenceScore: 92, PrecisionScore: 95, DuplicateNoiseScore: 12, FalsePositiveScore: 6, QADecision: "approved_for_backend_enrichment", Rationale: "Strong vehicle identity signal with low legal risk; safe to use as context only."},
		{SourceKey: "nhtsa_vpic_db", SampleScope: "Local VIN decode mirror readiness for Hyundai/Kia-focused flows", EvidenceScore: 95, PrecisionScore: 96, DuplicateNoiseScore: 8, FalsePositiveScore: 5, QADecision: "approved_for_backend_enrichment", Rationale: "Best free low-risk source for local decode enrichment and reproducible QA."},
		{SourceKey: "nhtsa_recalls_api", SampleScope: "Component-level recall context for Hyundai/Kia samples", EvidenceScore: 84, PrecisionScore: 74, DuplicateNoiseScore: 18, FalsePositiveScore: 32, QADecision: "approved_for_backend_context_only", Rationale: "Useful for warning badges and context; component strings are too coarse for part-number decisions."},
		{SourceKey: "nhtsa_complaints_api", SampleScope: "Owner complaint text review against Hyundai/Kia issue clusters", EvidenceScore: 58, PrecisionScore: 42, DuplicateNoiseScore: 57, FalsePositiveScore: 71, QADecision: "research_only", Rationale: "Freeform text is too noisy for cautious automated suggestions."},
		{SourceKey: "tecdoc", SampleScope: "Existing Hyundai/Kia fitment, OEM cross-ref, and replacement flows", EvidenceScore: 94, PrecisionScore: 91, DuplicateNoiseScore: 15, FalsePositiveScore: 14, QADecision: "approved_for_backend_enrichment", Rationale: "Primary fitment and replacement evidence base already present in the stack."},
		{SourceKey: "haynespro", SampleScope: "Vendor capability review for Hyundai/Kia diagrams and install guidance", EvidenceScore: 82, PrecisionScore: 88, DuplicateNoiseScore: 14, FalsePositiveScore: 11, QADecision: "approved_pending_contract", Rationale: "Most promising safe diagram source, but still requires licensed access and golden-set validation."},
		{SourceKey: "mitchell1_prodemand", SampleScope: "Vendor capability review for repair guidance and diagrams", EvidenceScore: 78, PrecisionScore: 85, DuplicateNoiseScore: 16, FalsePositiveScore: 12, QADecision: "approved_pending_subscription", Rationale: "Likely safe and useful, but access model is shop-subscription dependent."},
		{SourceKey: "epa_fueleconomy", SampleScope: "Engine and drivetrain metadata relevance for Hyundai/Kia trim disambiguation", EvidenceScore: 72, PrecisionScore: 83, DuplicateNoiseScore: 10, FalsePositiveScore: 17, QADecision: "approved_for_backend_context_only", Rationale: "Helpful for engine variant context, but not directly tied to parts truth."},
		{SourceKey: "wikidata", SampleScope: "Platform-sharing seed research for Hyundai/Kia sibling vehicles", EvidenceScore: 61, PrecisionScore: 64, DuplicateNoiseScore: 22, FalsePositiveScore: 39, QADecision: "research_only", Rationale: "Useful as a starting hint, but not strong enough for direct claims without TecDoc confirmation."},
		{SourceKey: "wikimedia_commons", SampleScope: "Per-file licensing review for generic component illustrations", EvidenceScore: 45, PrecisionScore: 35, DuplicateNoiseScore: 28, FalsePositiveScore: 68, QADecision: "manual_review_only", Rationale: "Potentially reusable only after file-level license, attribution, and generic-identity review; never a fitment source."},
		{SourceKey: "hyundai_tech_info", SampleScope: "Manual OEM validation source for Hyundai service data", EvidenceScore: 93, PrecisionScore: 97, DuplicateNoiseScore: 6, FalsePositiveScore: 4, QADecision: "manual_reference_only", Rationale: "Excellent truth source, but not acceptable for ingestion without a direct license."},
		{SourceKey: "kia_tech_info", SampleScope: "Manual OEM validation source for Kia service data", EvidenceScore: 93, PrecisionScore: 97, DuplicateNoiseScore: 6, FalsePositiveScore: 4, QADecision: "manual_reference_only", Rationale: "Excellent truth source, but not acceptable for ingestion without a direct license."},
		{SourceKey: "oem_retail_diagram_sites", SampleScope: "Consumer dealer retail diagram pages", EvidenceScore: 49, PrecisionScore: 85, DuplicateNoiseScore: 19, FalsePositiveScore: 28, QADecision: "rejected", Rationale: "Coverage is tempting, but legal and robots restrictions make it unsafe to automate."},
		{SourceKey: "rockauto", SampleScope: "Consumer aftermarket retail site coverage", EvidenceScore: 44, PrecisionScore: 69, DuplicateNoiseScore: 33, FalsePositiveScore: 41, QADecision: "rejected", Rationale: "Rejected because automation is blocked and quality cannot justify that risk."},
	}
}
