// API types matching Go backend models

export interface Vehicle {
  linkageTargetId: number;
  make: string;
  model: string;
  modelYear?: number;
  description?: string;
  fuelType?: string;
  capacityCC?: number;
  horsePower?: number;
  beginYearMonth?: number;
  endYearMonth?: number;
}

export interface Part {
  legacyArticleId: number;
  articleNumber: string;
  description: string;
  brandName?: string;
  category?: string;
  assemblyGroupId?: number;
}

export interface OEMReference {
  rawNumber: string;
  normalized?: string;
  manufacturer?: string;
  brandName?: string;
  articleNumber?: string;
  description?: string;
  legacyArticleId: number;
}

export interface CrossBrandHit {
  siblingMake: string;
  siblingModel: string;
  platform: string;
  sharedParts?: number;
}

export interface Recall {
  nhtsaCampaignNumber: string;
  component: string;
  summary: string;
  consequence?: string;
  remedy?: string;
  reportDate?: string;
  sourceLabel: string;
  sourceUrl: string;
  warning?: string;
}

export interface SupersessionLink {
  legacyArticleId: number;
  articleNumber: string;
  brandName?: string;
  description?: string;
  direction: 'replaced_by' | 'replaces' | 'reported_replacement' | 'reported_predecessor' | 'reported_related';
  confidence: number;
  source: {
    kind: string;
    label: string;
    detail: string;
  };
  warnings?: string[];
}

export interface NHTSAVehicle {
  make: string;
  model: string;
  modelYear: string;
  bodyClass?: string;
  driveType?: string;
  fuelType?: string;
  engineDisplacementCC?: string;
  engineNumberOfCylinders?: string;
}

export interface VINDecodeResponse {
  vin: string;
  nhtsaRaw?: NHTSAVehicle;
  vehicle?: Vehicle;
  parts?: Part[];
  totalParts?: number;
  crossBrand?: CrossBrandHit[];
  recalls?: Recall[];
  allVariants?: Vehicle[];
  needsConfirmation?: boolean;
}

export interface CategoryInfo {
  name: string;
  partCount: number;
  fitmentDriver: string;
}

export interface CategoriesResponse {
  linkageTargetId: number;
  categories: CategoryInfo[];
  total: number;
}

// Hierarchical category tree
export interface CategoryLeaf {
  name: string;
  assemblyGroupId?: number;
  partCount: number;
}

export interface CategoryGroup {
  name: string;
  icon?: string;
  categories: CategoryLeaf[];
  totalParts: number;
}

export interface CategoryTreeResponse {
  linkageTargetId: number;
  tree: CategoryGroup[];
  totalGroups: number;
  totalParts: number;
}

// Alternatives
export interface AlternativePart extends Part {
  sharedVehicles?: number;
}

export interface AlternativesResponse {
  legacyArticleId: number;
  alternatives: AlternativePart[];
  total: number;
  label: string;
}

export interface PartWithOEM extends Part {
  oemNumbers?: string[];
}

export interface PartsResponse {
  linkageTargetId: number;
  page: number;
  limit: number;
  total: number;
  parts: Part[];
}

export interface OEMSearchResponse {
  query: string;
  normalized: string;
  results: OEMReference[];
  total: number;
  fitsVehicles?: VehicleFit[];
  oemCategory?: OEMCategory;
}

export interface VehicleFit {
  make: string;
  model: string;
  modelYear?: number;
  description?: string;
  linkageTargetId?: number;
}

export interface OEMCategory {
  system: string;
  subsystem: string;
  prefix: string;
}

export interface ChainResponse {
  legacyArticleId: number;
  chain: SupersessionLink[];
  total: number;
}

// SmartSearch types

export interface SubstitutionPart {
  partNumber: string;
  description: string;
  make?: string;
}

export interface AftermarketPart {
  partNumber: string;
  description: string;
  brand: string;
}

export interface SmartResult {
  legacyArticleId: number;
  articleNumber: string;
  description: string;
  brandName?: string;
  category?: string;
  assemblyGroupId?: number;
  confidence: number;
  confidenceNote?: string;
  fitmentDriver: string;
  oemNumbers?: OEMReference[];
  brand?: string;
  fitsVehicleCC?: number;
  substitutions?: SubstitutionPart[];
  aftermarketAlternatives?: AftermarketPart[];
  compatibility?: string[];
}

export interface SmartSearchResponse {
  query: string;
  vehicle?: Vehicle;
  results: SmartResult[];
  total: number;
  categories?: string[];
  searchStrategy: string;
  warnings?: string[];
}

// Platform compatibility
export interface PlatformInfo {
  siblingMake: string;
  siblingModel: string;
  platform: string;
  sharedParts: number;
  totalParts: number;
}

export interface PlatformResponse {
  linkageTargetId: number;
  make: string;
  model: string;
  siblings: PlatformInfo[];
}

// Parts criteria (TecDoc attributes)
export interface PartCriterion {
  name: string;
  value: string;
  unit?: string;
}

export interface EnrichedPart extends Part {
  oemNumbers?: string[];
  criteria?: PartCriterion[];
  alternatives?: number;
}

export type PartSourceKind =
  | 'owned_catalog'
  | 'smart_search'
  | 'oem_crossref'
  | 'derived_inference'
  | 'external_source';

export type PartConfidenceBand = 'high' | 'medium' | 'low';

export interface PartDetailSource {
  kind: PartSourceKind;
  label: string;
  detail: string;
}

export interface PartDetailQuality {
  provenanceComplete: boolean;
  provenanceGaps?: string[];
  hasOEMNumbers: boolean;
  hasCriteria: boolean;
  hasVehicleContext?: boolean;
  hasFitmentEvidence?: boolean;
  hasPlacement?: boolean;
  placementExact?: boolean;
  hasReplacementCandidates?: boolean;
}

export type PartPlacementKind = 'exact' | 'catalog_group' | 'inferred' | 'unavailable';
export type PartPlacementType = 'diagram' | 'text_hint' | 'catalog_group' | 'none';

export interface PartPlacementSource {
  kind: PartSourceKind;
  label: string;
  detail: string;
}

export interface PartPlacement {
  kind: PartPlacementKind;
  placementType: PartPlacementType;
  title: string;
  summary: string;
  locationArea?: string;
  imageUrl?: string;
  thumbnailUrl?: string;
  hints?: string[];
  warnings?: string[];
  confidence: number;
  source: PartPlacementSource;
}

export interface PartDetailAlternative extends Part {
  sharedVehicles?: number;
}

export type PartReplacementCandidateType = 'shared_oem_reference' | 'catalog_compatible' | 'aftermarket_alternative' | 'substitution';

export interface PartReplacementCandidate {
  legacyArticleId: number;
  articleNumber: string;
  description: string;
  brandName?: string;
  category?: string;
  assemblyGroupId?: number;
  candidateType: PartReplacementCandidateType;
  explanation: string;
  oemReference?: string;
  confidence: number;
  source: PartDetailSource;
  warnings?: string[];
}

export interface PartDetailViewModel {
  legacyArticleId: number;
  articleNumber: string;
  description: string;
  brandName?: string;
  category?: string;
  oemNumbers: string[];
  criteria?: Record<string, string>;
  fitVehicles?: Vehicle[];
  replacements?: PartReplacementCandidate[];
  alternatives?: PartDetailAlternative[];
  placement?: PartPlacement;
  confidence: number;
  confidenceBand: PartConfidenceBand;
  confidenceReason: string;
  fitmentDriver?: string;
  source: PartDetailSource;
  warnings?: string[];
  quality: PartDetailQuality;
}

export interface PartDetailResponse {
  legacyArticleId: number;
  vehicleId?: number;
  articleNumber: string;
  description: string;
  brandName?: string;
  category?: string;
  assemblyGroupId?: number;
  oemNumbers?: string[];
  criteria?: Record<string, string>;
  fitVehicles?: Vehicle[];
  replacements?: PartReplacementCandidate[];
  alternatives?: PartDetailAlternative[];
  placement?: PartPlacement;
  source: PartDetailSource;
  confidence: {
    score: number;
    reason: string;
  };
  quality: PartDetailQuality;
  warnings?: string[];
}
