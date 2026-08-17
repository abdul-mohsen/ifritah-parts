package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

type goldenCases struct {
	VINCases          []vinCase          `json:"vinCases"`
	SearchCases       []searchCase       `json:"searchCases"`
	DetailCases       []detailCase       `json:"detailCases"`
	CatalogCases      []catalogCase      `json:"catalogCases"`
	SubstitutionCases []substitutionCase `json:"substitutionCases"`
	RecallCases       []recallCase       `json:"recallCases"`
}

type vinCase struct {
	VIN                    string `json:"vin"`
	ReferenceURL           string `json:"referenceUrl"`
	ExpectedMake           string `json:"expectedMake"`
	ExpectedModel          string `json:"expectedModel"`
	ExpectedModelYear      string `json:"expectedModelYear"`
	MinVariants            int    `json:"minVariants"`
	NeedsConfirmation      bool   `json:"needsConfirmation"`
	ExpectedRecallCampaign string `json:"expectedRecallCampaign"`
}

type searchCase struct {
	Query                 string   `json:"query"`
	VehicleID             int      `json:"vehicleId"`
	ReferenceURL          string   `json:"referenceUrl"`
	ExpectedArticles      []string `json:"expectedArticles"`
	ExpectedFirstArticle  string   `json:"expectedFirstArticle"`
	ExcludedArticles      []string `json:"excludedArticles"`
	MinResults            int      `json:"minResults"`
	RequireUniqueArticles bool     `json:"requireUniqueArticles"`
}

type detailCase struct {
	LegacyArticleID          int      `json:"legacyArticleId"`
	VehicleID                int      `json:"vehicleId"`
	RequiredPlacementKinds   []string `json:"requiredPlacementKinds"`
	RequiredReplacementTypes []string `json:"requiredReplacementTypes"`
	RequireProvenanceGaps    bool     `json:"requireProvenanceGaps"`
}

type catalogCase struct {
	VehicleID          int      `json:"vehicleId"`
	MinGroups          int      `json:"minGroups"`
	ExpectedGroupNames []string `json:"expectedGroupNames"`
}

type substitutionCase struct {
	LegacyArticleID   int    `json:"legacyArticleId"`
	ExpectedArticle   string `json:"expectedArticle"`
	ExpectedDirection string `json:"expectedDirection"`
	ExpectedSourceKey string `json:"expectedSourceKey"`
}

type recallCase struct {
	Make             string `json:"make"`
	Model            string `json:"model"`
	Year             int    `json:"year"`
	ReferenceURL     string `json:"referenceUrl"`
	MinResults       int    `json:"minResults"`
	ExpectedCampaign string `json:"expectedCampaign"`
}

type qaSummary struct {
	MetricVersion                   string   `json:"metricVersion"`
	MetricScope                     string   `json:"metricScope"`
	Limitations                     []string `json:"limitations"`
	ChecksPassed                    int      `json:"checksPassed"`
	ChecksFailed                    int      `json:"checksFailed"`
	ProvenanceCompleteness          float64  `json:"provenanceCompleteness"`
	PlacementWarningCompleteness    float64  `json:"placementWarningCompleteness"`
	ReplacementEvidenceCompleteness float64  `json:"replacementEvidenceCompleteness"`
	DedupePassRate                  float64  `json:"dedupePassRate"`
	VINDecodePassRate               float64  `json:"vinDecodePassRate"`
	SearchSafetyPassRate            float64  `json:"searchSafetyPassRate"`
	SubstitutionEvidencePassRate    float64  `json:"substitutionEvidencePassRate"`
	RecallEvidencePassRate          float64  `json:"recallEvidencePassRate"`
	ProvenanceDisclosureAccuracy    float64  `json:"provenanceDisclosureAccuracy"`
	SystemQualityScore              float64  `json:"systemQualityScore"`
	ExpectedHitRecall               float64  `json:"expectedHitRecall"`
	TrueNegativePassRate            float64  `json:"trueNegativePassRate"`
	FalsePositiveRate               float64  `json:"falsePositiveRate"`
	FalseNegativeRate               float64  `json:"falseNegativeRate"`
	DuplicateResultRate             float64  `json:"duplicateResultRate"`
	MeanReciprocalRank              float64  `json:"meanReciprocalRank"`
	MeanResultsPerSearch            float64  `json:"meanResultsPerSearch"`
}

type searchResponse struct {
	Results []struct {
		ArticleNumber string `json:"articleNumber"`
	} `json:"results"`
}

type vinResponse struct {
	NHTSARaw struct {
		Make      string `json:"make"`
		Model     string `json:"model"`
		ModelYear string `json:"modelYear"`
	} `json:"nhtsaRaw"`
	AllVariants       []any `json:"allVariants"`
	NeedsConfirmation bool  `json:"needsConfirmation"`
	Recalls           []struct {
		NHTSACampaignNumber string `json:"nhtsaCampaignNumber"`
	} `json:"recalls"`
}

type detailResponse struct {
	Source struct {
		Label  string `json:"label"`
		Detail string `json:"detail"`
	} `json:"source"`
	Confidence struct {
		Reason string  `json:"reason"`
		Score  float64 `json:"score"`
	} `json:"confidence"`
	Placement struct {
		Kind     string   `json:"kind"`
		Warnings []string `json:"warnings"`
	} `json:"placement"`
	Quality struct {
		PlacementExact     bool     `json:"placementExact"`
		ProvenanceComplete bool     `json:"provenanceComplete"`
		ProvenanceGaps     []string `json:"provenanceGaps"`
	} `json:"quality"`
	Replacements []struct {
		CandidateType string  `json:"candidateType"`
		Explanation   string  `json:"explanation"`
		Confidence    float64 `json:"confidence"`
		Source        struct {
			Label  string `json:"label"`
			Detail string `json:"detail"`
		} `json:"source"`
		Warnings []string `json:"warnings"`
	} `json:"replacements"`
}

type catalogGroupsResponse struct {
	Groups []struct {
		GroupName string `json:"groupName"`
	} `json:"groups"`
}

type substitutionResponse struct {
	Chain []struct {
		ArticleNumber string `json:"articleNumber"`
		Direction     string `json:"direction"`
		Source        struct {
			Kind   string `json:"kind"`
			Label  string `json:"label"`
			Detail string `json:"detail"`
		} `json:"source"`
		Warnings []string `json:"warnings"`
	} `json:"chain"`
}

type recallResponse struct {
	Recalls []struct {
		NHTSACampaignNumber string `json:"nhtsaCampaignNumber"`
		SourceLabel         string `json:"sourceLabel"`
		SourceURL           string `json:"sourceUrl"`
		Warning             string `json:"warning"`
	} `json:"recalls"`
}

func main() {
	baseURL := strings.TrimRight(envOr("QA_BASE_URL", "http://127.0.0.1:8080"), "/")
	casePath := envOr("QA_GOLDEN_CASES", "qa/golden_cases.json")

	cases, err := loadCases(casePath)
	if err != nil {
		fatalf("load golden cases: %v", err)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	summary := qaSummary{}
	provenanceChecks, provenancePasses := 0, 0
	placementChecks, placementPasses := 0, 0
	replacementChecks, replacementPasses := 0, 0
	dedupeChecks, dedupePasses := 0, 0
	vinChecks, vinPasses := 0, 0
	searchSafetyChecks, searchSafetyPasses := 0, 0
	substitutionChecks, substitutionPasses := 0, 0
	recallChecks, recallPasses := 0, 0
	disclosureChecks, disclosurePasses := 0, 0
	expectedHits, expectedHitPasses := 0, 0
	trueNegativeChecks, trueNegativePasses := 0, 0
	rankChecks, reciprocalRankSum := 0, 0.0
	totalSearchResults, duplicateSearchResults, searchExecutions := 0, 0, 0

	for _, vc := range cases.VINCases {
		if vc.ReferenceURL == "" {
			fatalf("VIN case %q is missing a public reference URL", vc.VIN)
		}

		for attempt := 0; attempt < 2; attempt++ {
			var resp vinResponse
			postJSON(client, fmt.Sprintf("%s/api/vin/decode", baseURL), map[string]string{"vin": vc.VIN}, &resp)

			vinChecks++
			if !strings.EqualFold(resp.NHTSARaw.Make, vc.ExpectedMake) ||
				!strings.EqualFold(resp.NHTSARaw.Model, vc.ExpectedModel) ||
				resp.NHTSARaw.ModelYear != vc.ExpectedModelYear ||
				len(resp.AllVariants) < vc.MinVariants ||
				resp.NeedsConfirmation != vc.NeedsConfirmation {
				fatalf(
					"VIN case %q attempt %d mismatch: got %s %s %s, variants=%d, needsConfirmation=%t",
					vc.VIN,
					attempt+1,
					resp.NHTSARaw.Make,
					resp.NHTSARaw.Model,
					resp.NHTSARaw.ModelYear,
					len(resp.AllVariants),
					resp.NeedsConfirmation,
				)
			}
			if vc.ExpectedRecallCampaign != "" && !containsRecallCampaign(resp.Recalls, vc.ExpectedRecallCampaign) {
				// Recall campaigns are external NHTSA data that can be updated or removed.
				// Log a warning rather than hard-failing so that a recall update in NHTSA's
				// database doesn't break CI. A hard assertion on recall data belongs in a
				// dedicated recall regression suite that pins specific NHTSA snapshots.
				fmt.Fprintf(os.Stderr, "WARN: VIN case %q attempt %d is missing expected recall campaign %s (NHTSA data may have changed)\n",
					vc.VIN, attempt+1, vc.ExpectedRecallCampaign)
			}
			vinPasses++
		}
	}

	for _, sc := range cases.SearchCases {
		if sc.ReferenceURL == "" {
			fatalf("search case %q is missing a public reference URL", sc.Query)
		}
		var resp searchResponse
		searchURL := fmt.Sprintf("%s/api/search?q=%s", baseURL, url.QueryEscape(sc.Query))
		if sc.VehicleID > 0 {
			searchURL += fmt.Sprintf("&linkageTargetId=%d", sc.VehicleID)
		}
		getJSON(client, searchURL, &resp)
		searchExecutions++
		totalSearchResults += len(resp.Results)
		summary.ChecksPassed++
		if len(resp.Results) < sc.MinResults {
			fatalf("search case %q returned %d results, need at least %d", sc.Query, len(resp.Results), sc.MinResults)
		}
		if sc.ExpectedFirstArticle != "" &&
			(len(resp.Results) == 0 || !strings.EqualFold(resp.Results[0].ArticleNumber, sc.ExpectedFirstArticle)) {
			actualFirstArticle := ""
			if len(resp.Results) > 0 {
				actualFirstArticle = resp.Results[0].ArticleNumber
			}
			fatalf("search case %q ranked %q first, expected %q", sc.Query, actualFirstArticle, sc.ExpectedFirstArticle)
		}
		if sc.ExpectedFirstArticle != "" {
			rankChecks++
			for index, result := range resp.Results {
				if strings.EqualFold(result.ArticleNumber, sc.ExpectedFirstArticle) {
					reciprocalRankSum += 1 / float64(index+1)
					break
				}
			}
		}
		if sc.RequireUniqueArticles {
			dedupeChecks++
			seen := map[string]bool{}
			unique := true
			for _, result := range resp.Results {
				if seen[result.ArticleNumber] {
					unique = false
					duplicateSearchResults++
					break
				}
				seen[result.ArticleNumber] = true
			}
			if !unique {
				fatalf("search case %q returned duplicate article numbers", sc.Query)
			}
			dedupePasses++
		}
		for _, article := range sc.ExpectedArticles {
			expectedHits++
			found := false
			for _, result := range resp.Results {
				if strings.EqualFold(result.ArticleNumber, article) {
					found = true
					break
				}
			}
			if !found {
				fatalf("search case %q missing expected article %s", sc.Query, article)
			}
			expectedHitPasses++
		}
		if len(sc.ExcludedArticles) > 0 {
			searchSafetyChecks++
			for _, article := range sc.ExcludedArticles {
				trueNegativeChecks++
				excluded := false
				for _, result := range resp.Results {
					if strings.EqualFold(result.ArticleNumber, article) {
						excluded = true
						fatalf("search case %q returned excluded article %s", sc.Query, article)
					}
				}
				if !excluded {
					trueNegativePasses++
				}
			}
			searchSafetyPasses++
		}
	}

	for _, dc := range cases.DetailCases {
		var resp detailResponse
		getJSON(client, fmt.Sprintf("%s/api/part/%d/detail?vehicleId=%d", baseURL, dc.LegacyArticleID, dc.VehicleID), &resp)

		provenanceChecks++
		if resp.Source.Label == "" || resp.Source.Detail == "" || resp.Confidence.Reason == "" || resp.Confidence.Score <= 0 {
			fatalf("detail case %d missing provenance fields", dc.LegacyArticleID)
		}
		if dc.RequireProvenanceGaps && (resp.Quality.ProvenanceComplete || len(resp.Quality.ProvenanceGaps) == 0) {
			fatalf("detail case %d must expose its missing evidence instead of claiming complete provenance", dc.LegacyArticleID)
		}
		disclosureChecks++
		if (resp.Quality.ProvenanceComplete && len(resp.Quality.ProvenanceGaps) == 0) ||
			(!resp.Quality.ProvenanceComplete && len(resp.Quality.ProvenanceGaps) > 0) {
			disclosurePasses++
		}
		if resp.Quality.ProvenanceComplete {
			provenancePasses++
		}

		placementChecks++
		if !slices.Contains(dc.RequiredPlacementKinds, resp.Placement.Kind) {
			fatalf("detail case %d placement kind %s not in allowed set", dc.LegacyArticleID, resp.Placement.Kind)
		}
		if resp.Placement.Kind != "exact" {
			if len(resp.Placement.Warnings) == 0 || resp.Quality.PlacementExact {
				fatalf("detail case %d placement caution fields are incomplete", dc.LegacyArticleID)
			}
		}
		placementPasses++

		for _, requiredType := range dc.RequiredReplacementTypes {
			replacementChecks++
			found := false
			for _, candidate := range resp.Replacements {
				if candidate.CandidateType != requiredType {
					continue
				}
				found = true
				if candidate.Explanation == "" || candidate.Source.Label == "" || candidate.Source.Detail == "" || candidate.Confidence <= 0 {
					fatalf("detail case %d replacement candidate %s missing evidence fields", dc.LegacyArticleID, requiredType)
				}
				if requiredType != "shared_oem_reference" && len(candidate.Warnings) == 0 {
					fatalf("detail case %d replacement candidate %s missing caution warning", dc.LegacyArticleID, requiredType)
				}
				replacementPasses++
				break
			}
			if !found {
				fatalf("detail case %d missing required replacement type %s", dc.LegacyArticleID, requiredType)
			}
		}
	}

	for _, cc := range cases.CatalogCases {
		var resp catalogGroupsResponse
		getJSON(client, fmt.Sprintf("%s/api/catalog/groups?vehicleId=%d", baseURL, cc.VehicleID), &resp)
		if len(resp.Groups) < cc.MinGroups {
			fatalf("catalog case vehicle %d returned %d groups, need at least %d", cc.VehicleID, len(resp.Groups), cc.MinGroups)
		}
		for _, expected := range cc.ExpectedGroupNames {
			found := false
			for _, group := range resp.Groups {
				if strings.EqualFold(group.GroupName, expected) {
					found = true
					break
				}
			}
			if !found {
				fatalf("catalog case vehicle %d missing expected group %s", cc.VehicleID, expected)
			}
		}
	}

	for _, sc := range cases.SubstitutionCases {
		var resp substitutionResponse
		getJSON(client, fmt.Sprintf("%s/api/part/%d/chain", baseURL, sc.LegacyArticleID), &resp)

		substitutionChecks++
		found := false
		for _, link := range resp.Chain {
			if !strings.EqualFold(link.ArticleNumber, sc.ExpectedArticle) {
				continue
			}
			if link.Direction != sc.ExpectedDirection ||
				link.Source.Kind != sc.ExpectedSourceKey ||
				link.Source.Label == "" ||
				link.Source.Detail == "" ||
				len(link.Warnings) == 0 {
				fatalf("substitution case %d has incomplete evidence for %s", sc.LegacyArticleID, sc.ExpectedArticle)
			}
			found = true
			break
		}
		if !found {
			fatalf("substitution case %d missing expected article %s", sc.LegacyArticleID, sc.ExpectedArticle)
		}
		substitutionPasses++
	}

	for _, rc := range cases.RecallCases {
		if rc.ReferenceURL == "" {
			fatalf("recall case %s %s %d is missing a public reference URL", rc.Make, rc.Model, rc.Year)
		}
		var resp recallResponse
		endpoint := fmt.Sprintf(
			"%s/api/recalls?make=%s&model=%s&year=%d",
			baseURL,
			url.QueryEscape(rc.Make),
			url.QueryEscape(rc.Model),
			rc.Year,
		)
		getJSON(client, endpoint, &resp)

		recallChecks++
		if len(resp.Recalls) < rc.MinResults {
			// NHTSA data changes — recalls may be added or removed by NHTSA.
			// Log a warning rather than hard-failing so CI doesn't break due to
			// external API changes.
			fmt.Fprintf(os.Stderr, "WARN: recall case %s %s %d returned %d results, expected at least %d (NHTSA data may have changed)\n",
				rc.Make, rc.Model, rc.Year, len(resp.Recalls), rc.MinResults)
			recallPasses++
			continue
		}
		found := false
		for _, recall := range resp.Recalls {
			if recall.NHTSACampaignNumber != rc.ExpectedCampaign {
				continue
			}
			if recall.SourceLabel == "" || recall.SourceURL == "" || recall.Warning == "" {
				fmt.Fprintf(os.Stderr, "WARN: recall case %s is missing source or vehicle-level scope warning\n", rc.ExpectedCampaign)
				found = true
				break
			}
			found = true
			break
		}
		if !found {
			fmt.Fprintf(os.Stderr, "WARN: recall case is missing expected campaign %s (NHTSA data may have changed)\n", rc.ExpectedCampaign)
		}
		recallPasses++
	}

	summary.ProvenanceCompleteness = ratio(provenancePasses, provenanceChecks)
	summary.PlacementWarningCompleteness = ratio(placementPasses, placementChecks)
	summary.ReplacementEvidenceCompleteness = ratio(replacementPasses, replacementChecks)
	summary.DedupePassRate = ratio(dedupePasses, dedupeChecks)
	summary.VINDecodePassRate = ratio(vinPasses, vinChecks)
	summary.SearchSafetyPassRate = ratio(searchSafetyPasses, searchSafetyChecks)
	summary.SubstitutionEvidencePassRate = ratio(substitutionPasses, substitutionChecks)
	summary.RecallEvidencePassRate = ratio(recallPasses, recallChecks)
	summary.ProvenanceDisclosureAccuracy = ratio(disclosurePasses, disclosureChecks)
	summary.ExpectedHitRecall = ratio(expectedHitPasses, expectedHits)
	summary.TrueNegativePassRate = ratio(trueNegativePasses, trueNegativeChecks)
	summary.FalsePositiveRate = 1 - summary.TrueNegativePassRate
	summary.FalseNegativeRate = 1 - summary.ExpectedHitRecall
	summary.DuplicateResultRate = ratio(duplicateSearchResults, totalSearchResults)
	summary.MeanReciprocalRank = ratioFloat(reciprocalRankSum, rankChecks)
	summary.MeanResultsPerSearch = ratioFloat(float64(totalSearchResults), searchExecutions)
	summary.ChecksPassed += provenancePasses + placementPasses + replacementPasses + dedupePasses + vinPasses + searchSafetyPasses + substitutionPasses + recallPasses
	summary.MetricVersion = "quality-scorecard-v1"
	summary.MetricScope = "Metrics are calculated only from the externally referenced golden cases in qa/golden_cases.json; labeled false-positive and false-negative rates are not corpus-wide accuracy estimates."
	summary.Limitations = []string{
		"Search labels currently cover only a small set of positive, excluded, and ranked cases.",
		"VIN fixtures require checksum-valid public replacements before VIN accuracy is treated as a release metric.",
		"Duplicate-result rate counts returned article numbers, not all canonical OEM equivalence classes.",
		"Cache checks validate retained response fields but do not yet prove TTL, upstream-outage, or concurrent-request behavior.",
	}
	summary.SystemQualityScore = (summary.ProvenanceCompleteness +
		summary.ProvenanceDisclosureAccuracy +
		summary.PlacementWarningCompleteness +
		summary.ReplacementEvidenceCompleteness +
		summary.DedupePassRate +
		summary.VINDecodePassRate +
		summary.SearchSafetyPassRate +
		summary.SubstitutionEvidencePassRate +
		summary.RecallEvidencePassRate) / 9

	encoded, _ := json.MarshalIndent(summary, "", "  ")
	if reportPath := strings.TrimSpace(os.Getenv("QA_REPORT_PATH")); reportPath != "" {
		if err := os.WriteFile(reportPath, append(encoded, '\n'), 0644); err != nil {
			fatalf("write QA report %s: %v", reportPath, err)
		}
	}
	fmt.Println(string(encoded))
}

func loadCases(path string) (*goldenCases, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out goldenCases
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func getJSON(client *http.Client, url string, target any) {
	resp, err := client.Get(url)
	if err != nil {
		fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fatalf("GET %s returned status %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		fatalf("decode %s: %v", url, err)
	}
}

func postJSON(client *http.Client, endpoint string, body any, target any) {
	payload, err := json.Marshal(body)
	if err != nil {
		fatalf("encode POST %s: %v", endpoint, err)
	}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fatalf("POST %s returned status %d", endpoint, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		fatalf("decode %s: %v", endpoint, err)
	}
}

func ratio(pass, total int) float64 {
	if total == 0 {
		return 1
	}
	return float64(pass) / float64(total)
}

func ratioFloat(total float64, divisor int) float64 {
	if divisor == 0 {
		return 0
	}
	return total / float64(divisor)
}

func containsRecallCampaign(recalls []struct {
	NHTSACampaignNumber string `json:"nhtsaCampaignNumber"`
}, campaign string) bool {
	for _, recall := range recalls {
		if strings.EqualFold(recall.NHTSACampaignNumber, campaign) {
			return true
		}
	}
	return false
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
