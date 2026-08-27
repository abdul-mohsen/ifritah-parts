// Unit tests for the pure helpers in enrich_corpus_linkages. The
// DB-touching paths (scanInts) need a live TecDoc MySQL corpus and
// are exercised manually per the README. These tests just pin the
// side of the tool that CI can validate cheaply.

package main

import (
	"reflect"
	"testing"
)

// TestNormalizeOEM asserts the runtime rule the tool relies on to
// match TecDoc's oem_number.clean_number storage: uppercase and
// alphanumeric-only. It MUST agree with internal/service/tecdoc.go's
// NormalizeOEM helper — if they drift, the enrichment tool silently
// returns zero rows even for OEMs that do exist in the corpus.
func TestNormalizeOEM(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"strip dash", "26350-2J001", "263502J001"},
		{"strip space", " 26350 2J001 ", "263502J001"},
		{"upper", "97133-d3000", "97133D3000"},
		{"strip punctuation", "97133.D-3000", "97133D3000"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"HK style unchanged", "58101-3XA00", "581013XA00"},
		{"mixed alnum only", "Kia581013X", "KIA581013X"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeOEM(tc.in); got != tc.want {
				t.Errorf("normalizeOEM(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIndexOf(t *testing.T) {
	header := []string{"OEM", "Slice", "ExpectedCategory"}
	tests := []struct {
		name string
		q    string
		want int
	}{
		{"present at 0", "OEM", 0},
		{"present at end", "ExpectedCategory", 2},
		{"present mid", "Slice", 1},
		{"absent", "LinkageTargetIds", -1},
		{"case sensitive", "oem", -1},
		{"empty query", "", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := indexOf(header, tc.q); got != tc.want {
				t.Errorf("indexOf(header, %q) = %d, want %d", tc.q, got, tc.want)
			}
		})
	}
}

func TestJoinInts(t *testing.T) {
	tests := []struct {
		name string
		xs   []int
		sep  string
		want string
	}{
		{"empty", nil, "|", ""},
		{"single", []int{42}, "|", "42"},
		{"multiple pipe", []int{1, 2, 3}, "|", "1|2|3"},
		{"multiple comma", []int{10, 20}, ",", "10,20"},
		{"preserves order", []int{3, 1, 2}, "-", "3-1-2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinInts(tc.xs, tc.sep); got != tc.want {
				t.Errorf("joinInts(%v, %q) = %q, want %q", tc.xs, tc.sep, got, tc.want)
			}
		})
	}
}

// TestNormalizeOEM_MatchesRuntime documents the coupling. If the
// runtime NormalizeOEM changes (adds Unicode fold, adds hyphen
// preservation, etc.), THIS test needs to be updated in lockstep or
// the enrichment tool silently under-hits. Kept as a table with
// notable HK OEM shapes.
func TestNormalizeOEM_HKCorpus(t *testing.T) {
	// A cross-section of shapes that occur in the audit corpus.
	corpus := []struct {
		raw, clean string
	}{
		{"26350-2J001", "263502J001"}, // Hyundai V6 oil filter
		{"58101-3XA00", "581013XA00"}, // Hyundai front brake pads
		{"82460-2T010", "824602T010"}, // Kia window motor
		{"97133-2S000", "971332S000"}, // cabin filter
		{"55700-3S000", "557003S000"}, // Sonata rear axle beam
		{"86391-3S000", "863913S000"}, // Sonata mirror (M1 known collision)
	}
	// Also verify the return shape is a plain string with no lingering
	// separators — a regression here would break the SQL parameter
	// binding.
	want := make([]string, 0, len(corpus))
	got := make([]string, 0, len(corpus))
	for _, c := range corpus {
		want = append(want, c.clean)
		got = append(got, normalizeOEM(c.raw))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("HK corpus normalisation drift:\n  got  %v\n  want %v", got, want)
	}
}
