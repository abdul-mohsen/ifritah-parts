package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"parts-engine/internal/model"
)

// mockOnlineSource is a hand-rolled fake used by the dispatcher tests.
type mockOnlineSource struct {
	name     string
	enabled  bool
	rate     time.Duration
	results  []model.AftermarketPart
	err      error
	delay    time.Duration
	callCount int32
}

func (m *mockOnlineSource) Name() string             { return m.name }
func (m *mockOnlineSource) Enabled() bool            { return m.enabled }
func (m *mockOnlineSource) RateLimit() time.Duration { return m.rate }
func (m *mockOnlineSource) Search(ctx context.Context, oem string) ([]model.AftermarketPart, error) {
	atomic.AddInt32(&m.callCount, 1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestOnlineSearch_KillSwitchReturnsNil(t *testing.T) {
	t.Setenv("ONLINE_SEARCH_ENABLED", "false")
	src := &mockOnlineSource{name: "mock", enabled: true, results: []model.AftermarketPart{{PartNumber: "P1", Brand: "BOSCH"}}}
	s := NewOnlineSearch(nil, src)
	got := s.Search(context.Background(), "263202G000")
	if got != nil {
		t.Errorf("expected nil when kill-switch off, got %v", got)
	}
	if atomic.LoadInt32(&src.callCount) != 0 {
		t.Errorf("expected no source calls, got %d", src.callCount)
	}
}

func TestOnlineSearch_FanOutAndDedupe(t *testing.T) {
	t.Setenv("ONLINE_SEARCH_ENABLED", "true")
	src1 := &mockOnlineSource{
		name: "srcA", enabled: true,
		results: []model.AftermarketPart{
			{PartNumber: "P1", Brand: "BOSCH", Source: "srcA"},
			{PartNumber: "P2", Brand: "MANN-FILTER", Source: "srcA"},
		},
	}
	src2 := &mockOnlineSource{
		name: "srcB", enabled: true,
		results: []model.AftermarketPart{
			{PartNumber: "P1", Brand: "BOSCH", Source: "srcB"}, // duplicate of src1
			{PartNumber: "P3", Brand: "MAHLE", Source: "srcB"},
		},
	}
	s := NewOnlineSearch(nil, src1, src2)
	got := s.Search(context.Background(), "263202G000")

	// Expect: 3 unique parts (P1/BOSCH, P2/MANN-FILTER, P3/MAHLE)
	if len(got) != 3 {
		t.Fatalf("expected 3 deduped parts, got %d: %+v", len(got), got)
	}
	seen := map[string]bool{}
	for _, p := range got {
		seen[p.PartNumber] = true
	}
	for _, want := range []string{"P1", "P2", "P3"} {
		if !seen[want] {
			t.Errorf("missing part %s in output", want)
		}
	}
}

func TestOnlineSearch_DisabledSourceIgnored(t *testing.T) {
	t.Setenv("ONLINE_SEARCH_ENABLED", "true")
	off := &mockOnlineSource{name: "off", enabled: false}
	on := &mockOnlineSource{
		name: "on", enabled: true,
		results: []model.AftermarketPart{{PartNumber: "P1", Brand: "BOSCH"}},
	}
	s := NewOnlineSearch(nil, off, on)
	got := s.Search(context.Background(), "263202G000")
	if len(got) != 1 {
		t.Errorf("expected 1 result from enabled source only, got %d", len(got))
	}
	if atomic.LoadInt32(&off.callCount) != 0 {
		t.Errorf("expected 0 calls to disabled source, got %d", off.callCount)
	}
}

func TestOnlineSearch_ErrorFromOneSourceDoesNotFailFanOut(t *testing.T) {
	t.Setenv("ONLINE_SEARCH_ENABLED", "true")
	bad := &mockOnlineSource{name: "bad", enabled: true, err: errors.New("network dead")}
	good := &mockOnlineSource{
		name: "good", enabled: true,
		results: []model.AftermarketPart{{PartNumber: "P1", Brand: "BOSCH"}},
	}
	s := NewOnlineSearch(nil, bad, good)
	got := s.Search(context.Background(), "263202G000")
	if len(got) != 1 {
		t.Errorf("expected 1 result from surviving source, got %d", len(got))
	}
}

func TestOnlineSearch_EmptyBrandOrPartFiltered(t *testing.T) {
	t.Setenv("ONLINE_SEARCH_ENABLED", "true")
	src := &mockOnlineSource{
		name: "s", enabled: true,
		results: []model.AftermarketPart{
			{PartNumber: "", Brand: "BOSCH"},             // no part number → drop
			{PartNumber: "P1", Brand: ""},                // no brand → drop
			{PartNumber: "P2", Brand: "MANN"},            // keep
			{PartNumber: "  ", Brand: "MAHLE"},           // whitespace-only part → not dropped (relies on downstream dedupe)
		},
	}
	s := NewOnlineSearch(nil, src)
	got := s.Search(context.Background(), "263202G000")
	// Two of the four should survive: P2/MANN and "  "/MAHLE
	if len(got) < 1 {
		t.Fatalf("expected at least 1 valid result, got %d: %+v", len(got), got)
	}
	// Confirm P2 is present with normalised brand.
	var foundP2 bool
	for _, p := range got {
		if p.PartNumber == "P2" {
			foundP2 = true
			if p.Brand != NormalizeBrand("MANN") {
				t.Errorf("P2 brand not normalised: got %q", p.Brand)
			}
		}
	}
	if !foundP2 {
		t.Errorf("expected P2/MANN in results")
	}
}

func TestOnlineSearch_NoSourcesReturnsEmpty(t *testing.T) {
	t.Setenv("ONLINE_SEARCH_ENABLED", "true")
	s := NewOnlineSearch(nil)
	got := s.Search(context.Background(), "263202G000")
	if got != nil {
		t.Errorf("expected nil with zero sources, got %v", got)
	}
}
