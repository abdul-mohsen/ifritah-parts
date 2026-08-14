package service

import "testing"

func TestNormalizeTextSearchQueryUsesKnownAliasesAndTokens(t *testing.T) {
	tests := map[string]string{
		"Cabin Air Filter": "cabin filter",
		"pollen filter":    "cabin filter",
		"Oil-Filter":       "oil filter",
		"  Brake brake  ":  "brake",
	}

	for input, want := range tests {
		if got := normalizeTextSearchQuery(input); got != want {
			t.Errorf("normalizeTextSearchQuery(%q) = %q, want %q", input, got, want)
		}
	}
}
