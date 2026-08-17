package service

import (
	"context"
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
)

// supersessionHopRepo returns one hop of a supersession chain at a time.
// The service walks the graph itself so cycle-detection and depth-cap
// enforcement live in Go where they are unit-testable, rather than being
// buried in a MySQL WITH RECURSIVE plan that would require a live DB
// to exercise.
type supersessionHopRepo interface {
	QueryReplacedBy(ctx context.Context, legacyArticleId int) ([]supersessionHop, error)
	QueryReplaces(ctx context.Context, legacyArticleId int) ([]supersessionHop, error)
	QueryCurrent(ctx context.Context, legacyArticleId int) (supersessionHop, error)
}

type supersessionHop struct {
	LegacyArticleId int
	ArticleNumber   string
	Description     string
	BrandName       string
}

// TecDocSupersession walks the replacedbyarticles / replacesarticles graph
// recursively and returns a bounded model.SupersessionChain.
//
// The walk terminates when:
//   - a node has no outgoing edges (natural end)
//   - the walker revisits a node it has already seen (cycle)
//   - depth exceeds model.MaxSupersessionDepth (protection)
//
// When the depth cap is the reason for termination, chain.Truncated is
// set so the caller can honestly warn the user rather than claim a
// complete chain.
type TecDocSupersession struct {
	repo supersessionHopRepo
}

func NewTecDocSupersession(db *sql.DB) *TecDocSupersession {
	if db == nil {
		return &TecDocSupersession{}
	}
	return &TecDocSupersession{repo: &sqlSupersessionRepo{db: db}}
}

// FindSupersession returns the full chain (forward + backward) for the
// given article, up to model.MaxSupersessionDepth hops in each direction.
func (s *TecDocSupersession) FindSupersession(legacyArticleId int) (model.SupersessionChain, error) {
	var out model.SupersessionChain
	if s.repo == nil {
		return out, fmt.Errorf("database not connected")
	}
	if legacyArticleId <= 0 {
		return out, fmt.Errorf("invalid legacyArticleId: %d", legacyArticleId)
	}

	ctx := context.Background()

	cur, err := s.repo.QueryCurrent(ctx, legacyArticleId)
	if err != nil {
		return out, fmt.Errorf("query current article: %w", err)
	}
	out.Current = hopToLink(cur, "current")

	forward, fwdTruncated, err := s.walk(ctx, legacyArticleId, true)
	if err != nil {
		return out, fmt.Errorf("walk forward: %w", err)
	}
	backward, bwdTruncated, err := s.walk(ctx, legacyArticleId, false)
	if err != nil {
		return out, fmt.Errorf("walk backward: %w", err)
	}

	out.ReplacedBy = forward
	out.Replaces = backward
	out.Depth = maxInt(len(forward), len(backward))
	out.Truncated = fwdTruncated || bwdTruncated
	return out, nil
}

// walk performs a bounded, cycle-safe traversal in one direction.
// forward=true walks replaced-by; forward=false walks replaces.
func (s *TecDocSupersession) walk(ctx context.Context, startId int, forward bool) ([]model.SupersessionLink, bool, error) {
	seen := map[int]bool{startId: true}
	frontier := []int{startId}
	var chain []model.SupersessionLink
	truncated := false

	for depth := 0; depth < model.MaxSupersessionDepth; depth++ {
		var next []int
		for _, id := range frontier {
			var hops []supersessionHop
			var err error
			if forward {
				hops, err = s.repo.QueryReplacedBy(ctx, id)
			} else {
				hops, err = s.repo.QueryReplaces(ctx, id)
			}
			if err != nil {
				return nil, false, err
			}
			for _, h := range hops {
				if h.LegacyArticleId != 0 && seen[h.LegacyArticleId] {
					continue
				}
				if h.LegacyArticleId != 0 {
					seen[h.LegacyArticleId] = true
				}
				dir := "replaces"
				if forward {
					dir = "replaced_by"
				}
				chain = append(chain, hopToLink(h, dir))
				if h.LegacyArticleId != 0 {
					next = append(next, h.LegacyArticleId)
				}
			}
		}
		if len(next) == 0 {
			return chain, false, nil
		}
		frontier = next
	}
	// If we exited the loop with a non-empty frontier, there was more graph
	// to walk beyond MaxSupersessionDepth.
	if len(frontier) > 0 {
		truncated = true
	}
	return chain, truncated, nil
}

func hopToLink(h supersessionHop, direction string) model.SupersessionLink {
	return model.SupersessionLink{
		LegacyArticleId: h.LegacyArticleId,
		ArticleNumber:   h.ArticleNumber,
		BrandName:       h.BrandName,
		Description:     h.Description,
		Direction:       direction,
		Confidence:      0.9,
		Source: model.ReplacementSource{
			Kind:   "tecdoc:articles",
			Label:  "TecDoc supersession chain",
			Detail: "Sourced from replacedbyarticles / replacesarticles.",
		},
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// sqlSupersessionRepo is the production repo bound to MySQL.
type sqlSupersessionRepo struct {
	db *sql.DB
}

func (r *sqlSupersessionRepo) QueryCurrent(ctx context.Context, id int) (supersessionHop, error) {
	const q = `
		SELECT
			a.legacyArticleId,
			COALESCE(a.articleNumber, ''),
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ab.brandName, '')
		FROM articles a
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE a.legacyArticleId = ?
		LIMIT 1`
	row := logQueryRow(r.db, "TecDocSupersession.QueryCurrent", q, id)
	var h supersessionHop
	if err := row.Scan(&h.LegacyArticleId, &h.ArticleNumber, &h.Description, &h.BrandName); err != nil {
		if err == sql.ErrNoRows {
			return supersessionHop{LegacyArticleId: id}, nil
		}
		return h, err
	}
	return h, nil
}

func (r *sqlSupersessionRepo) QueryReplacedBy(ctx context.Context, id int) ([]supersessionHop, error) {
	const q = `
		SELECT
			COALESCE(a.legacyArticleId, 0),
			rba.articleNumber,
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ab.brandName, '')
		FROM replacedbyarticles rba
		LEFT JOIN articles a ON UPPER(a.articleNumber) = UPPER(rba.articleNumber) AND a.mfrId = rba.mfrId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE rba.legacyArticleId = ?
		LIMIT 20`
	return r.scanHops(ctx, "TecDocSupersession.QueryReplacedBy", q, id)
}

func (r *sqlSupersessionRepo) QueryReplaces(ctx context.Context, id int) ([]supersessionHop, error) {
	const q = `
		SELECT
			COALESCE(a.legacyArticleId, 0),
			ra.articleNumber,
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ab.brandName, '')
		FROM replacesarticles ra
		LEFT JOIN articles a ON UPPER(a.articleNumber) = UPPER(ra.articleNumber) AND a.mfrId = ra.mfrId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE ra.legacyArticleId = ?
		LIMIT 20`
	return r.scanHops(ctx, "TecDocSupersession.QueryReplaces", q, id)
}

func (r *sqlSupersessionRepo) scanHops(ctx context.Context, label, q string, id int) ([]supersessionHop, error) {
	rows, err := logQueryCtx(r.db, ctx, label, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []supersessionHop
	for rows.Next() {
		var h supersessionHop
		if err := rows.Scan(&h.LegacyArticleId, &h.ArticleNumber, &h.Description, &h.BrandName); err != nil {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}
