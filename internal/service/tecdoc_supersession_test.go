package service

import (
	"context"
	"errors"
	"testing"

	"parts-engine/internal/model"
)

// stubSupersessionRepo lets tests drive the walker with an in-memory graph.
// forward[id] returns id's direct replaced-by children; backward[id] returns
// direct replaces parents. Both may be nil (natural end).
type stubSupersessionRepo struct {
	current  map[int]supersessionHop
	forward  map[int][]supersessionHop
	backward map[int][]supersessionHop
	err      error
}

func (s *stubSupersessionRepo) QueryCurrent(_ context.Context, id int) (supersessionHop, error) {
	if s.err != nil {
		return supersessionHop{}, s.err
	}
	if h, ok := s.current[id]; ok {
		return h, nil
	}
	return supersessionHop{LegacyArticleId: id}, nil
}

func (s *stubSupersessionRepo) QueryReplacedBy(_ context.Context, id int) ([]supersessionHop, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.forward[id], nil
}

func (s *stubSupersessionRepo) QueryReplaces(_ context.Context, id int) ([]supersessionHop, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.backward[id], nil
}

func TestTecDocSupersessionFindSupersessionForwardChain(t *testing.T) {
	repo := &stubSupersessionRepo{
		current: map[int]supersessionHop{
			1: {LegacyArticleId: 1, ArticleNumber: "26300-A"},
		},
		forward: map[int][]supersessionHop{
			1: {{LegacyArticleId: 2, ArticleNumber: "26300-B"}},
			2: {{LegacyArticleId: 3, ArticleNumber: "26300-C"}},
			3: nil,
		},
		backward: map[int][]supersessionHop{},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Current.ArticleNumber != "26300-A" {
		t.Fatalf("current not populated: %+v", chain.Current)
	}
	if len(chain.ReplacedBy) != 2 {
		t.Fatalf("expected 2 forward hops, got %d", len(chain.ReplacedBy))
	}
	if chain.ReplacedBy[0].Direction != "replaced_by" {
		t.Fatalf("direction not stamped: %q", chain.ReplacedBy[0].Direction)
	}
	if chain.Truncated {
		t.Fatalf("chain should not be truncated for depth 2")
	}
	if chain.Depth != 2 {
		t.Fatalf("expected depth=2, got %d", chain.Depth)
	}
}

func TestTecDocSupersessionFindSupersessionCycleSafe(t *testing.T) {
	// 1 -> 2 -> 1  (cycle back)
	repo := &stubSupersessionRepo{
		forward: map[int][]supersessionHop{
			1: {{LegacyArticleId: 2, ArticleNumber: "B"}},
			2: {{LegacyArticleId: 1, ArticleNumber: "A"}},
		},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chain.ReplacedBy) != 1 {
		t.Fatalf("cycle not detected — expected exactly 1 hop, got %d", len(chain.ReplacedBy))
	}
}

func TestTecDocSupersessionFindSupersessionDepthCap(t *testing.T) {
	// Build a straight chain longer than MaxSupersessionDepth (10).
	forward := map[int][]supersessionHop{}
	for i := 1; i < 20; i++ {
		forward[i] = []supersessionHop{{LegacyArticleId: i + 1, ArticleNumber: "P"}}
	}
	repo := &stubSupersessionRepo{forward: forward}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !chain.Truncated {
		t.Fatalf("expected Truncated=true when depth exceeds cap")
	}
	if len(chain.ReplacedBy) != model.MaxSupersessionDepth {
		t.Fatalf("expected %d hops (the cap), got %d", model.MaxSupersessionDepth, len(chain.ReplacedBy))
	}
}

func TestTecDocSupersessionFindSupersessionBackwardChain(t *testing.T) {
	repo := &stubSupersessionRepo{
		backward: map[int][]supersessionHop{
			5: {{LegacyArticleId: 4, ArticleNumber: "OLD-1"}},
			4: {{LegacyArticleId: 3, ArticleNumber: "OLD-2"}},
		},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chain.Replaces) != 2 {
		t.Fatalf("expected 2 backward hops, got %d", len(chain.Replaces))
	}
	if chain.Replaces[0].Direction != "replaces" {
		t.Fatalf("expected direction=replaces, got %q", chain.Replaces[0].Direction)
	}
	if len(chain.ReplacedBy) != 0 {
		t.Fatalf("forward chain must remain empty")
	}
}

func TestTecDocSupersessionInvalidId(t *testing.T) {
	svc := &TecDocSupersession{repo: &stubSupersessionRepo{}}
	if _, err := svc.FindSupersession(0); err == nil {
		t.Fatalf("expected error for zero id")
	}
	if _, err := svc.FindSupersession(-1); err == nil {
		t.Fatalf("expected error for negative id")
	}
}

func TestTecDocSupersessionNilRepo(t *testing.T) {
	svc := &TecDocSupersession{}
	if _, err := svc.FindSupersession(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocSupersessionNilDBConstructor(t *testing.T) {
	svc := NewTecDocSupersession(nil)
	if svc == nil {
		t.Fatalf("NewTecDocSupersession(nil) must not return nil")
	}
	if _, err := svc.FindSupersession(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocSupersessionRepoErrorForward(t *testing.T) {
	svc := &TecDocSupersession{repo: &stubSupersessionRepo{err: errors.New("boom")}}
	if _, err := svc.FindSupersession(1); err == nil {
		t.Fatalf("expected repo error to surface")
	}
}

func TestTecDocSupersessionSourceStamped(t *testing.T) {
	repo := &stubSupersessionRepo{
		forward: map[int][]supersessionHop{
			1: {{LegacyArticleId: 2, ArticleNumber: "X"}},
		},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, _ := svc.FindSupersession(1)
	if chain.ReplacedBy[0].Source.Kind != "tecdoc:articles" {
		t.Fatalf("expected source.Kind=tecdoc:articles, got %q", chain.ReplacedBy[0].Source.Kind)
	}
	if chain.ReplacedBy[0].Confidence == 0 {
		t.Fatalf("confidence not set")
	}
}
