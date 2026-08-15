package service

import (
	"fmt"
	"sort"
	"strings"

	"parts-engine/internal/model"
)

type replacementOEMFinder interface {
	FindByOEM(oemNumber string, limit int) ([]model.OEMReference, error)
}

type replacementPartFinder interface {
	FindByArticle(legacyArticleId int, linkageTargetId int) (*model.Part, error)
}

type replacementAlternativesFinder interface {
	FindForArticle(legacyArticleId int, linkageTargetId int, limit int) ([]AlternativePart, error)
}

type ReplacementAdvisor struct {
	crossRef     replacementOEMFinder
	parts        replacementPartFinder
	alternatives replacementAlternativesFinder
}

func NewReplacementAdvisor(crossRef *CrossRef, parts *PartsLookup, alternatives *Alternatives) *ReplacementAdvisor {
	return &ReplacementAdvisor{
		crossRef:     crossRef,
		parts:        parts,
		alternatives: alternatives,
	}
}

func (r *ReplacementAdvisor) Build(part *model.Part, linkageTargetID int, oemNumbers []string, limit int) ([]model.ReplacementCandidate, []string, error) {
	if part == nil {
		return nil, nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}

	candidates := make([]model.ReplacementCandidate, 0, limit)
	warnings := []string{}
	seen := map[int]bool{part.LegacyArticleId: true}

	if r.crossRef != nil {
		for _, rawOEM := range uniqueReplacementStrings(oemNumbers) {
			refs, err := r.crossRef.FindByOEM(rawOEM, limit*2)
			if err != nil {
				return nil, warnings, fmt.Errorf("shared OEM candidates: %w", err)
			}
			for _, ref := range refs {
				if seen[ref.LegacyArticleId] || ref.LegacyArticleId <= 0 {
					continue
				}
				candidatePart := r.loadPart(ref.LegacyArticleId, linkageTargetID)
				if candidatePart == nil {
					continue
				}
				seen[ref.LegacyArticleId] = true
				candidates = append(candidates, model.ReplacementCandidate{
					Part:          *candidatePart,
					CandidateType: "shared_oem_reference",
					Explanation:   fmt.Sprintf("Shares OEM reference %s with the selected part in the owned catalog cross-reference index.", rawOEM),
					OEMReference:  rawOEM,
					Confidence:    0.86,
					Source: model.ReplacementSource{
						Kind:   "oem_crossref",
						Label:  "Shared OEM reference",
						Detail: "This candidate is suggested because both parts map to the same OEM reference in the owned cross-reference index. It is a stronger signal than a text-only similarity match, but still needs human confirmation before being treated as an exact replacement.",
					},
					Warnings: []string{
						"Shared OEM references are strong evidence, but they are not being presented as a guaranteed supersession chain.",
					},
				})
				if len(candidates) >= limit {
					sortReplacementCandidates(candidates)
					return candidates[:limit], warnings, nil
				}
			}
		}
	}

	if r.alternatives != nil {
		alts, err := r.alternatives.FindForArticle(part.LegacyArticleId, linkageTargetID, limit)
		if err != nil {
			return nil, warnings, fmt.Errorf("catalog compatible candidates: %w", err)
		}
		for _, alt := range alts {
			if seen[alt.LegacyArticleId] || alt.LegacyArticleId <= 0 {
				continue
			}
			seen[alt.LegacyArticleId] = true
			candidates = append(candidates, model.ReplacementCandidate{
				Part:          alt.Part,
				CandidateType: "catalog_compatible",
				Explanation:   "Matches the same owned-catalog generic description and compatibility bucket for this vehicle context.",
				Confidence:    0.64,
				Source: model.ReplacementSource{
					Kind:   "owned_catalog",
					Label:  "Catalog compatibility",
					Detail: "This suggestion comes from the owned catalog's same-description compatibility grouping. It is useful for discovery, but weaker than a shared OEM reference and should not be treated as a guaranteed replacement.",
				},
				Warnings: []string{
					"Catalog-compatible suggestions are intentionally shown as lower-confidence options, not exact substitutions.",
				},
			})
			if len(candidates) >= limit {
				break
			}
		}
		if len(alts) > 0 {
			warnings = append(warnings, "Some suggestions below are catalog-compatible alternatives rather than proven OEM replacement chains.")
		}
	}

	sortReplacementCandidates(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates, warnings, nil
}

func (r *ReplacementAdvisor) loadPart(legacyArticleID int, linkageTargetID int) *model.Part {
	if r.parts == nil {
		return nil
	}
	part, err := r.parts.FindByArticle(legacyArticleID, linkageTargetID)
	if err == nil && part != nil {
		return part
	}
	if linkageTargetID > 0 {
		part, err = r.parts.FindByArticle(legacyArticleID, 0)
		if err == nil && part != nil {
			return part
		}
	}
	return nil
}

func sortReplacementCandidates(candidates []model.ReplacementCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Confidence == candidates[j].Confidence {
			return strings.ToUpper(candidates[i].ArticleNumber) < strings.ToUpper(candidates[j].ArticleNumber)
		}
		return candidates[i].Confidence > candidates[j].Confidence
	})
}

func uniqueReplacementStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
