package service

import (
	"strings"
)

// CategoryTokensForOEM returns the lower-case tokens a valid description
// for the given OEM SHOULD contain, extracted from the prefix's
// OEMCategory.Category label with common stopwords stripped.
//
// Used by M1.S3.T2's post-hoc category-consistency validation in
// searchCombined: any result whose description contains ZERO of these
// tokens is dropped as a category mismatch.
//
// Returns nil when the OEM does not decode via DecodeOEMPrefix.
// Never returns an empty non-nil slice — nil is the signal "no gate
// possible, let the result through".
func CategoryTokensForOEM(oem string) []string {
	cat := DecodeOEMPrefix(oem)
	if cat == nil {
		return nil
	}
	return tokenizeCategory(cat.Category)
}

// categoryTokenStopwords are words removed from tokenizeCategory output
// because they carry no signal for category classification.
var categoryTokenStopwords = map[string]bool{
	"a":   true,
	"an":  true,
	"the": true,
	"of":  true,
	"and": true,
	"or":  true,
	"for": true,
	"to":  true,
	"in":  true,
	"on":  true,
	"by":  true,
}

// tokenizeCategory extracts significant lower-case tokens from a category
// label. "Front Brake Pad / Disc" -> ["front", "brake", "pad", "disc"].
// Splits on whitespace + `/` + `-` + `&` + `(` + `)`. Filters tokens < 2
// chars and stopwords.
func tokenizeCategory(category string) []string {
	if category == "" {
		return nil
	}
	text := strings.ToLower(category)
	// Turn structural separators into spaces so Fields() splits cleanly.
	text = strings.NewReplacer(
		"/", " ",
		"-", " ",
		"&", " ",
		"(", " ",
		")", " ",
		",", " ",
	).Replace(text)
	toks := strings.Fields(text)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		if len(t) < 2 {
			continue
		}
		if categoryTokenStopwords[t] {
			continue
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hasCategoryOverlap returns true when the description contains at least
// one token from the expected list (case-insensitive). Used by the M1.S3
// validation to decide whether to keep or drop a result.
func hasCategoryOverlap(description string, expectedTokens []string) bool {
	if len(expectedTokens) == 0 {
		return true
	}
	descLower := strings.ToLower(description)
	for _, t := range expectedTokens {
		if strings.Contains(descLower, t) {
			return true
		}
	}
	return false
}
