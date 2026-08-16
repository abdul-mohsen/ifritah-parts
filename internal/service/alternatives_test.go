package service

import "testing"

// TestNewAlternatives_NilDBReturnsNonNilStruct verifies that constructing an
// Alternatives service with a nil *sql.DB does not panic and returns a usable
// (if limited) struct — the nil db is a valid "offline" mode.
func TestNewAlternatives_NilDBReturnsNonNilStruct(t *testing.T) {
	a := NewAlternatives(nil, false)
	if a == nil {
		t.Fatal("NewAlternatives(nil): expected non-nil struct")
	}
	if a.queries != nil {
		t.Error("NewAlternatives(nil): expected queries to be nil when db is nil")
	}
}

// TestAlternatives_FindForArticle_NilQueriesReturnsError verifies that calling
// FindForArticle on an Alternatives struct without an initialised queries field
// returns an error rather than panicking.
func TestAlternatives_FindForArticle_NilQueriesReturnsError(t *testing.T) {
	a := &Alternatives{} // zero-value: queries == nil
	results, err := a.FindForArticle(1, 0, 20)
	if err == nil {
		t.Error("expected error when queries is nil, got nil")
	}
	if results != nil {
		t.Errorf("expected nil results on error, got %v", results)
	}
}

// TestAlternatives_FindForArticle_LimitClamping verifies the limit guard:
// values ≤0 or >50 must be clamped to 20.  We can only test the limit
// normalisation path from the outside without a real DB by checking that the
// nil-queries guard fires AFTER the clamping — i.e. the function doesn't
// panic due to a zero limit before the guard.
func TestAlternatives_FindForArticle_LimitClamping(t *testing.T) {
	a := &Alternatives{} // nil queries — always returns error

	limits := []int{-10, 0, 51, 100, 1000}
	for _, lim := range limits {
		_, err := a.FindForArticle(1, 0, lim)
		// We expect an error from nil queries regardless of input limit.
		// The important thing is no panic — if we reach here the limit
		// clamping path didn't cause an out-of-range crash.
		if err == nil {
			t.Errorf("FindForArticle(limit=%d): expected error for nil queries, got nil", lim)
		}
	}
}
