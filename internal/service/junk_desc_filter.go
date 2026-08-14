package service

import (
	"strings"
)

// junkDescriptionPhrases is a curated deny-list of substrings that indicate a
// scrape result has captured page chrome (login buttons, error messages,
// captcha copy) instead of a real part description. Any online / dealer /
// supersession result whose description contains one of these must be
// rejected ΓÇö surfacing it as a 0.75-confidence "part" is worse than surfacing
// nothing.
//
// Comparison is case-insensitive substring match. Keep the deny-list narrow
// enough that legitimate descriptions ("brake pad set with signal") never
// contain any of these fragments.
var junkDescriptionPhrases = []string{
	"sign up with",
	"sign up",
	"sign in",
	"log in",
	"login",
	"create an account",
	"forgot password",
	"captcha",
	"cloudflare",
	"access denied",
	"403 forbidden",
	"404 not found",
	"501 not implemented",
	"502 bad gateway",
	"503 service unavailable",
	"504 gateway timeout",
	"not available",
	"unavailable",
	"page not found",
	"no results",
	"life-time-filter",   // Seen in production as a placeholder brand string.
	"life time filter",
	"click here",
	"read more",
	"learn more",
	"cookie preferences",
	"terms of service",
	"privacy policy",
}

// IsJunkDescription returns true when the given description looks like it
// was scraped from page chrome rather than a genuine part-catalog entry.
//
// The rule is "trust nothing that even smells like a UI string." False
// negatives (real description missed) cost the user nothing beyond a
// slightly less rich result; false positives (junk shown as part) cost the
// user confidence in the whole product.
func IsJunkDescription(description string) bool {
	if strings.TrimSpace(description) == "" {
		return true // Empty description is also junk ΓÇö never sell an unnamed part.
	}
	lower := strings.ToLower(description)
	for _, phrase := range junkDescriptionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// FilterJunkOEMReferences returns a copy of `refs` with any entries whose
// Description looks like scraped page chrome removed. Preserves order.
func FilterJunkOEMReferences(refs []struct {
	Description string
}) int {
	// This helper is unused at the moment; kept as a design placeholder for
	// callers that want to reject before they build the SmartResult.
	kept := 0
	for _, ref := range refs {
		if IsJunkDescription(ref.Description) {
			continue
		}
		kept++
	}
	return kept
}
