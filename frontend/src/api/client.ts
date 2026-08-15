import type {
  VINDecodeResponse,
  PartsResponse,
  OEMSearchResponse,
  ChainResponse,
  SmartSearchResponse,
  CategoriesResponse,
  CategoryTreeResponse,
  AlternativesResponse,
  PlatformResponse,
  PartDetailResponse,
} from '../types';

const BASE = '/api';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${url}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

export async function decodeVIN(vin: string): Promise<VINDecodeResponse> {
  return request('/vin/decode', {
    method: 'POST',
    body: JSON.stringify({ vin }),
  });
}

export async function getPartsForVehicle(
  linkageTargetId: number,
  page = 1,
  limit = 20,
  category = '',
  enrich = false,
): Promise<PartsResponse> {
  const params = new URLSearchParams({
    page: String(page),
    limit: String(limit),
  });
  if (category) params.set('category', category);
  if (enrich) params.set('enrich', 'true');
  return request(`/vehicle/${linkageTargetId}/parts?${params}`);
}

export async function getCategories(
  linkageTargetId: number,
): Promise<CategoriesResponse> {
  return request(`/vehicle/${linkageTargetId}/categories`);
}

export async function searchOEM(
  number: string,
  limit = 20
): Promise<OEMSearchResponse> {
  return request(`/oem/${encodeURIComponent(number)}?limit=${limit}`);
}

export async function getSupersessionChain(
  legacyArticleId: number
): Promise<ChainResponse> {
  return request(`/part/${legacyArticleId}/chain`);
}

export async function getRecalls(
  make: string,
  model: string,
  year: number
) {
  return request(
    `/recalls?make=${encodeURIComponent(make)}&model=${encodeURIComponent(model)}&year=${year}`
  );
}

export async function smartSearch(
  q: string,
  opts: { linkageTargetId?: number; vehicleCC?: number; fuelType?: string; category?: string; page?: number; limit?: number } = {}
): Promise<SmartSearchResponse> {
  const params = new URLSearchParams();
  if (q) params.set('q', q);
  if (opts.linkageTargetId) params.set('linkageTargetId', String(opts.linkageTargetId));
  if (opts.vehicleCC) params.set('vehicleCC', String(opts.vehicleCC));
  if (opts.fuelType) params.set('fuelType', opts.fuelType);
  if (opts.category) params.set('category', opts.category);
  if (opts.page) params.set('page', String(opts.page));
  if (opts.limit) params.set('limit', String(opts.limit));
  return request(`/search?${params}`);
}

export async function getCategoryTree(
  linkageTargetId: number,
): Promise<CategoryTreeResponse> {
  return request(`/vehicle/${linkageTargetId}/tree`);
}

export async function getAlternatives(
  legacyArticleId: number,
  vehicleId?: number,
  limit = 20,
): Promise<AlternativesResponse> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (vehicleId) params.set('vehicleId', String(vehicleId));
  return request(`/part/${legacyArticleId}/alternatives?${params}`);
}

export async function getPartDetail(
  legacyArticleId: number,
  vehicleId?: number,
): Promise<PartDetailResponse> {
  const params = new URLSearchParams();
  if (vehicleId) params.set('vehicleId', String(vehicleId));
  const suffix = params.toString() ? `?${params.toString()}` : '';
  return request(`/part/${legacyArticleId}/detail${suffix}`);
}

export async function getPlatformSiblings(
  linkageTargetId: number,
  make?: string,
  model?: string,
): Promise<PlatformResponse> {
  const params = new URLSearchParams();
  if (make) params.set('make', make);
  if (model) params.set('model', model);
  return request(`/vehicle/${linkageTargetId}/platform?${params}`);
}

export async function getEnrichedParts(
  linkageTargetId: number,
  category: string,
  page = 1,
  limit = 20,
): Promise<PartsResponse> {
  const params = new URLSearchParams({
    page: String(page),
    limit: String(limit),
    category,
    enrich: 'true',
  });
  return request(`/vehicle/${linkageTargetId}/parts?${params}`);
}
