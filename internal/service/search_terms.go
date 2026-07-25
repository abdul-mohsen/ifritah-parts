package service

import (
	"strings"
	"unicode"
)

var textSearchAliases = map[string]string{
	"cabin air filter": "cabin filter",
	"pollen filter":    "cabin filter",
}

func normalizeTextSearchQuery(query string) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if alias, ok := textSearchAliases[normalized]; ok {
		normalized = alias
	}

	terms := strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := make(map[string]struct{}, len(terms))
	result := make([]string, 0, len(terms))
	for _, term := range terms {
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		result = append(result, term)
	}
	return strings.Join(result, " ")
}
