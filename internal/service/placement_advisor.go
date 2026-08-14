package service

import (
	"context"
	"database/sql"
	"strings"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

type PlacementAdvisor struct {
	queries *store.Queries
}

func NewPlacementAdvisor(db *sql.DB) *PlacementAdvisor {
	if db == nil {
		return &PlacementAdvisor{}
	}
	return &PlacementAdvisor{queries: store.New(db)}
}

type placementRule struct {
	match        []string
	title        string
	locationArea string
	summary      string
	hints        []string
}

var placementRules = []placementRule{
	{
		match:        []string{"cabin filter", "blower", "hvac", "climate"},
		title:        "Cabin / HVAC intake area",
		locationArea: "Interior ventilation path",
		summary:      "This part usually sits in the cabin air intake or HVAC housing rather than in the exposed engine bay.",
		hints: []string{
			"Check behind the glove box or at the cowl intake before removing engine-bay parts.",
			"Confirm airflow direction and housing orientation before installation.",
		},
	},
	{
		match:        []string{"front brake", "rear brake", "abs", "wheel speed"},
		title:        "Brake and wheel-end area",
		locationArea: "Axle / wheel corner",
		summary:      "This part is typically mounted around the brake assembly or wheel-end hardware for the affected axle.",
		hints: []string{
			"Match front vs rear fitment before disassembly.",
			"Inspect connector and bracket routing if the part includes a sensor lead.",
		},
	},
	{
		match:        []string{"air intake", "engine oil", "cooling system", "fuel system", "timing", "ignition"},
		title:        "Engine bay service area",
		locationArea: "Engine compartment",
		summary:      "This part is typically serviced from the engine bay and depends on engine layout and accessory access.",
		hints: []string{
			"Use the selected vehicle context to confirm engine-side packaging before ordering.",
			"Engine-dependent parts should be checked against displacement and fuel type when available.",
		},
	},
	{
		match:        []string{"headlight", "rear light", "mirror", "glass", "wiper", "body panel"},
		title:        "Exterior body mounting area",
		locationArea: "Body / exterior trim",
		summary:      "This part is mounted on exterior bodywork and usually varies by body style, side, or trim.",
		hints: []string{
			"Verify left vs right and front vs rear before installation.",
			"Trim and market-specific variations are common for body components.",
		},
	},
	{
		match:        []string{"steering", "suspension", "drive shaft", "transmission", "clutch"},
		title:        "Underbody drivetrain area",
		locationArea: "Chassis / driveline underside",
		summary:      "This part is usually accessed from below the vehicle and may vary by drivetrain or suspension layout.",
		hints: []string{
			"Confirm drivetrain layout before treating this as an exact match.",
			"Expect underbody access rather than top-engine access.",
		},
	},
}

func (p *PlacementAdvisor) Build(part *model.Part, vehicle *model.Vehicle, oemNumbers []string) model.PartPlacement {
	if part == nil {
		return unavailablePlacement()
	}

	if placement := p.externalPlacement(part, vehicle, oemNumbers); placement != nil {
		return *placement
	}

	inferred := inferredPlacement(part)
	if inferred != nil {
		return *inferred
	}

	return unavailablePlacement()
}

func (p *PlacementAdvisor) externalPlacement(part *model.Part, vehicle *model.Vehicle, oemNumbers []string) *model.PartPlacement {
	if p.queries == nil {
		return nil
	}

	makeNorm, modelNorm := "", ""
	if vehicle != nil {
		makeNorm = strings.ToLower(strings.TrimSpace(vehicle.Make))
		modelNorm = strings.ToLower(strings.TrimSpace(vehicle.Model))
	}

	candidates := []string{NormalizeOEM(part.ArticleNumber)}
	for _, oem := range oemNumbers {
		norm := NormalizeOEM(oem)
		if norm != "" && norm != candidates[0] {
			candidates = append(candidates, norm)
		}
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		hints, err := p.queries.FindExternalInstallHintsByPart(context.Background(), store.FindExternalInstallHintsByPartParams{
			PartNumberNorm: candidate,
			MakeNorm:       makeNorm,
			ModelNorm:      modelNorm,
		})
		if err != nil || len(hints) == 0 {
			continue
		}

		best := hints[0]
		placement := &model.PartPlacement{
			Kind:          normalizePlacementKind(best.Exactness),
			PlacementType: "text_hint",
			Title:         placementTitleFromHint(best.HintType, part.Category),
			Summary:       best.HintText,
			LocationArea:  strings.ReplaceAll(best.HintType, "_", " "),
			Hints:         []string{best.HintText},
			Confidence:    best.Confidence,
			Source: model.PlacementSource{
				Kind:   "external_source",
				Label:  sourceDisplay(best.SourceKey),
				Detail: "This placement comes from the external-source registry and is labeled with its exactness rather than treated as guaranteed OEM diagram truth.",
			},
		}

		if artifacts, err := p.queries.FindExternalArtifactsBySource(context.Background(), best.SourceKey); err == nil {
			for _, artifact := range artifacts {
				if strings.HasPrefix(artifact.MimeType, "image/") && artifact.MediaUrl != "" {
					placement.ImageURL = artifact.MediaUrl
					placement.ThumbnailURL = artifact.ThumbnailUrl
					placement.PlacementType = "diagram"
					break
				}
			}
		}
		if placement.Kind != "exact" {
			placement.Warnings = append(placement.Warnings, "This placement is not an exact OEM diagram match and should be treated as guided context.")
		}
		return placement
	}

	return nil
}

func inferredPlacement(part *model.Part) *model.PartPlacement {
	text := strings.ToLower(strings.TrimSpace(part.Category + " " + part.Description))
	for _, rule := range placementRules {
		for _, token := range rule.match {
			if strings.Contains(text, token) {
				return &model.PartPlacement{
					Kind:          "inferred",
					PlacementType: "text_hint",
					Title:         rule.title,
					Summary:       rule.summary,
					LocationArea:  rule.locationArea,
					Hints:         rule.hints,
					Warnings: []string{
						"No exact diagram is loaded for this part yet; this location is inferred from the catalog category and description.",
					},
					Confidence: 0.58,
					Source: model.PlacementSource{
						Kind:   "derived_inference",
						Label:  "Catalog-based placement inference",
						Detail: "This placement is inferred from the owned catalog category and part description, not from an exact diagram artifact.",
					},
				}
			}
		}
	}

	if part.Category != "" {
		return &model.PartPlacement{
			Kind:          "catalog_group",
			PlacementType: "catalog_group",
			Title:         "Catalog group placement",
			Summary:       "The part can only be located to its catalog group right now; no exact or visual placement has been loaded.",
			LocationArea:  part.Category,
			Warnings: []string{
				"This is a group-level placement only and should not be treated as an exact installation diagram.",
			},
			Confidence: 0.5,
			Source: model.PlacementSource{
				Kind:   "owned_catalog",
				Label:  "Owned catalog group",
				Detail: "The placement comes from the current catalog grouping only and remains broader than an exploded-view diagram.",
			},
		}
	}

	return nil
}

func unavailablePlacement() model.PartPlacement {
	return model.PartPlacement{
		Kind:          "unavailable",
		PlacementType: "none",
		Title:         "Placement not available",
		Summary:       "No exact or inferred placement could be produced for this part in the current runtime state.",
		Warnings: []string{
			"The system is withholding placement instead of guessing beyond available evidence.",
		},
		Confidence: 0,
		Source: model.PlacementSource{
			Kind:   "owned_catalog",
			Label:  "No placement evidence",
			Detail: "Placement was intentionally omitted because no safe exact or inferred placement was available.",
		},
	}
}

func normalizePlacementKind(exactness string) string {
	switch strings.ToLower(strings.TrimSpace(exactness)) {
	case "exact":
		return "exact"
	case "catalog_group":
		return "catalog_group"
	case "inferred":
		return "inferred"
	default:
		return "inferred"
	}
}

func placementTitleFromHint(hintType, category string) string {
	switch strings.ToLower(strings.TrimSpace(hintType)) {
	case "diagram":
		return "External placement diagram"
	case "install_path":
		return "Installation path hint"
	case "location":
		return "Location hint"
	default:
		if category != "" {
			return category + " placement hint"
		}
		return "Placement hint"
	}
}

func sourceDisplay(sourceKey string) string {
	for _, record := range DefaultExternalSourceCatalog() {
		if record.SourceKey == sourceKey {
			return record.DisplayName
		}
	}
	return sourceKey
}
