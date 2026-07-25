import type { PartDetailResponse, PartDetailSource, PartDetailViewModel } from '../types';

interface PartDetailInput {
  legacyArticleId: number;
  articleNumber: string;
  description: string;
  brandName?: string;
  category?: string;
  oemNumbers?: string[];
  criteria?: Record<string, string>;
  fitVehicles?: PartDetailViewModel['fitVehicles'];
  replacements?: PartDetailViewModel['replacements'];
  alternatives?: PartDetailViewModel['alternatives'];
  placement?: PartDetailViewModel['placement'];
  confidence: number;
  confidenceReason: string;
  fitmentDriver?: string;
  source: PartDetailSource;
  warnings?: string[];
  quality?: PartDetailViewModel['quality'];
}

function toConfidenceBand(confidence: number): PartDetailViewModel['confidenceBand'] {
  if (confidence >= 0.85) {
    return 'high';
  }
  if (confidence >= 0.65) {
    return 'medium';
  }
  return 'low';
}

export function createPartDetailViewModel(input: PartDetailInput): PartDetailViewModel {
  const warnings = [...(input.warnings ?? [])];
  const oemNumbers = input.oemNumbers ?? [];

  if (oemNumbers.length === 0) {
    warnings.push('No OEM cross-reference is loaded for this part in the current view.');
  }
  if (!input.criteria || Object.keys(input.criteria).length === 0) {
    warnings.push('No technical specifications are loaded for this part in the current view.');
  }

  return {
    legacyArticleId: input.legacyArticleId,
    articleNumber: input.articleNumber,
    description: input.description,
    brandName: input.brandName,
    category: input.category,
    oemNumbers,
    criteria: input.criteria,
    fitVehicles: input.fitVehicles,
    replacements: input.replacements,
    alternatives: input.alternatives,
    placement: input.placement,
    confidence: input.confidence,
    confidenceBand: toConfidenceBand(input.confidence),
    confidenceReason: input.confidenceReason,
    fitmentDriver: input.fitmentDriver,
    source: input.source,
    warnings,
    quality: input.quality ?? {
      provenanceComplete: Boolean(
        input.source.label &&
        input.source.detail &&
        input.confidenceReason &&
        oemNumbers.length > 0 &&
        input.criteria &&
        Object.keys(input.criteria).length > 0 &&
        input.fitVehicles &&
        input.fitVehicles.length > 0 &&
        input.placement &&
        input.placement.kind !== 'unavailable',
      ),
      provenanceGaps: [
        ...(oemNumbers.length === 0 ? ['OEM reference evidence'] : []),
        ...(!input.criteria || Object.keys(input.criteria).length === 0 ? ['technical specification evidence'] : []),
        ...(!input.fitVehicles || input.fitVehicles.length === 0 ? ['expanded vehicle fitment evidence'] : []),
        ...(!input.placement || input.placement.kind === 'unavailable' ? ['placement evidence'] : []),
      ],
      hasOEMNumbers: oemNumbers.length > 0,
      hasCriteria: Boolean(input.criteria && Object.keys(input.criteria).length > 0),
      hasFitmentEvidence: Boolean(input.fitVehicles && input.fitVehicles.length > 0),
      hasPlacement: Boolean(input.placement && input.placement.kind !== 'unavailable'),
      placementExact: input.placement?.kind === 'exact',
      hasReplacementCandidates: Boolean(input.replacements && input.replacements.length > 0),
    },
  };
}

export function createPartDetailViewModelFromResponse(response: PartDetailResponse): PartDetailViewModel {
  return createPartDetailViewModel({
    legacyArticleId: response.legacyArticleId,
    articleNumber: response.articleNumber,
    description: response.description,
    brandName: response.brandName,
    category: response.category,
    oemNumbers: response.oemNumbers,
    criteria: response.criteria,
    fitVehicles: response.fitVehicles,
    replacements: response.replacements,
    alternatives: response.alternatives,
    placement: response.placement,
    confidence: response.confidence.score,
    confidenceReason: response.confidence.reason,
    source: response.source,
    warnings: response.warnings,
    quality: response.quality,
  });
}
