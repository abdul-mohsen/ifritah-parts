import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import type { SmartResult, OEMSearchResponse, PartDetailViewModel, OEMReference } from '../types';
import { getPartDetail, searchOEM } from '../api/client';
import { useSearchStream } from '../hooks/useSearchStream';
import PartDetailModal from './PartDetailModal';
import SupersessionChain from './SupersessionChain';
import { SearchModeSelector, StrategyBadge, StrategiesSummaryBar } from './SearchModeSelector';
import { SpecificationTable } from './SpecificationTable';
import { CompatibilityChips } from './CompatibilityChips';
import { DocumentsList } from './DocumentsList';
import { SearchProgress } from './SearchProgress';
import { createPartDetailViewModel, createPartDetailViewModelFromResponse } from '../utils/partDetail';

const driverColors: Record<string, string> = {
  engine: 'bg-orange-100 text-orange-800 border-orange-200',
  brake: 'bg-red-100 text-red-800 border-red-200',
  body: 'bg-blue-100 text-blue-800 border-blue-200',
  drivetrain: 'bg-purple-100 text-purple-800 border-purple-200',
  universal: 'bg-gray-100 text-gray-700 border-gray-200',
  online: 'bg-emerald-100 text-emerald-800 border-emerald-200',
};

// Detect Hyundai/Kia OEM pattern: 5 digits, or 5-digits + letter suffix, or dash-separated
const OEM_PATTERN = /^[0-9]{2}[0-9A-Z]{3}[-\s]?[A-Z0-9]{0,6}$/i;

function isLikelyOEM(q: string): boolean {
  const cleaned = q.replace(/[-\s.]/g, '');
  return OEM_PATTERN.test(q) || (cleaned.length >= 5 && /^\d{5,}/.test(cleaned));
}

function ConfidenceBadge({ value }: { value: number }) {
  const pct = Math.round(value * 100);
  const color =
    pct >= 90 ? 'text-green-700 bg-green-50' :
    pct >= 70 ? 'text-yellow-700 bg-yellow-50' :
    'text-red-700 bg-red-50';
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs font-semibold ${color}`}>
      {pct}%
    </span>
  );
}

function FitmentBadge({ driver }: { driver: string }) {
  const cls = driverColors[driver] || driverColors.universal;
  return (
    <span className={`inline-block px-2 py-0.5 rounded border text-xs font-medium ${cls}`}>
      {driver}
    </span>
  );
}

function dedupeOEMResults(results: OEMReference[]): OEMReference[] {
  const seen = new Map<string, OEMReference>();
  for (const result of results) {
    const key = result.legacyArticleId > 0
      ? `id:${result.legacyArticleId}`
      : `${result.articleNumber || result.rawNumber}|${result.brandName || result.manufacturer || ''}`;
    if (!seen.has(key)) {
      seen.set(key, result);
    }
  }
  return Array.from(seen.values());
}

function deriveModelName(model: string | undefined, description: string | undefined): string {
  if (model && model.trim()) {
    return model.trim().toUpperCase();
  }
  if (!description) {
    return '';
  }
  return description.split(' ')[0]?.trim().toUpperCase() ?? '';
}

export default function OemSearch() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const vehicleId = Number(searchParams.get('vehicleId') || '0');
  const vehicleCC = Number(searchParams.get('vehicleCC') || '0');
  const fuelType = searchParams.get('fuelType') || '';
  const vehicleMake = searchParams.get('make') || '';
  const vehicleModel = searchParams.get('model') || '';
  const sourceType = searchParams.get('sourceType') || '';
  const sourceQuery = searchParams.get('sourceQuery') || '';
  const hasVehicleContext = vehicleId > 0;
  const [query, setQuery] = useState(() => searchParams.get('q') || '');
  // Mode persistence: URL `?mode=` wins > localStorage > empty.
  // On change, update both so the user's choice deep-links and survives reloads.
  const [searchMode, setSearchModeState] = useState(() => {
    const urlMode = searchParams.get('mode') || '';
    if (urlMode) return urlMode;
    try {
      return localStorage.getItem('ifritah.searchMode') || '';
    } catch {
      return '';
    }
  });
  const setSearchMode = (mode: string) => {
    setSearchModeState(mode);
    try {
      if (mode) {
        localStorage.setItem('ifritah.searchMode', mode);
      } else {
        localStorage.removeItem('ifritah.searchMode');
      }
    } catch {
      // localStorage may be unavailable (private-mode browsers, SSR). Silent fallback.
    }
    // Reflect selection in URL so the choice can be shared / bookmarked.
    const next = new URLSearchParams(searchParams);
    if (mode) {
      next.set('mode', mode);
    } else {
      next.delete('mode');
    }
    setSearchParams(next, { replace: true });
  };

  // Search stream hook — replaces the previous useState(loading) + smartSearch() calls.
  const { loading, steps, result, error: streamError, search: startSearch } = useSearchStream();

  const [error, setError] = useState('');
  const [oemResult, setOemResult] = useState<OEMSearchResponse | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [selectedDetail, setSelectedDetail] = useState<PartDetailViewModel | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');

  async function openSearchDetail(entry: SmartResult, strategy: string) {
    const fallback = createPartDetailViewModel({
      legacyArticleId: entry.legacyArticleId,
      articleNumber: entry.articleNumber,
      description: entry.description,
      brandName: entry.brand || entry.brandName,
      category: entry.category,
      oemNumbers: entry.oemNumbers?.map((item) => item.rawNumber),
      confidence: entry.confidence,
      confidenceReason: entry.confidenceNote || `This result was ranked by the ${strategy} strategy in the current smart search flow.`,
      fitmentDriver: entry.fitmentDriver,
      source: {
        kind: strategy === 'online_partsouq' ? 'derived_inference' : 'smart_search',
        label: strategy === 'online_partsouq' ? 'External search fallback' : 'Smart search result',
        detail: strategy === 'online_partsouq'
          ? 'The current result came from the online fallback search path and should be treated more cautiously.'
          : `The current result came from the owned smart search path using strategy: ${strategy}.`,
      },
    });

    setSelectedDetail(fallback);
    setDetailLoading(true);
    setDetailError('');

    try {
      const response = await getPartDetail(entry.legacyArticleId);
      setSelectedDetail(createPartDetailViewModelFromResponse(response));
    } catch (error) {
      setSelectedDetail(fallback);
      setDetailError(error instanceof Error ? error.message : 'Failed to load part detail.');
    } finally {
      setDetailLoading(false);
    }
  }

  async function openOEMDetail(ref: OEMReference) {
    const fallback = createPartDetailViewModel({
      legacyArticleId: ref.legacyArticleId,
      articleNumber: ref.articleNumber || ref.rawNumber,
      description: ref.description || 'OEM cross-reference result',
      brandName: ref.brandName || ref.manufacturer,
      oemNumbers: [ref.rawNumber],
      confidence: 0.9,
      confidenceReason: 'This result came from the dedicated OEM cross-reference flow for the searched number.',
      source: {
        kind: 'oem_crossref',
        label: 'OEM cross-reference',
        detail: 'The current result comes from the OEM lookup path and is shown as a cross-reference candidate for the searched number.',
      },
    });

    setSelectedDetail(fallback);
    setDetailLoading(true);
    setDetailError('');

    try {
      const response = await getPartDetail(ref.legacyArticleId);
      setSelectedDetail(createPartDetailViewModelFromResponse(response));
    } catch (error) {
      setSelectedDetail(fallback);
      setDetailError(error instanceof Error ? error.message : 'Failed to load part detail.');
    } finally {
      setDetailLoading(false);
    }
  }

  function openCatalogForVehicle(make: string, model: string, linkageTargetId?: number, description?: string) {
    const resolvedModel = deriveModelName(model, description);
    const params = new URLSearchParams({
      make,
      model: resolvedModel,
      sourceType: 'part search',
      sourceQuery: query.trim(),
    });
    if (linkageTargetId) {
      params.set('vehicleId', String(linkageTargetId));
    }
    navigate(`/catalog?${params.toString()}`);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = query.trim();
    if (!trimmed) return;
    setError('');
    setExpandedId(null);
    setOemResult(null);

    // Fire streaming search + OEM lookup in parallel.
    // Default to `combined` (Smart Search) — the only mode that fans out all
    // strategies in parallel AND emits per-strategy progress events so the
    // SearchProgress UI can show live per-strategy status. Explicit modes
    // (from the SearchModeSelector) override.
    const isOem = isLikelyOEM(trimmed);
    startSearch({
      q: trimmed,
      limit: 50,
      ...(hasVehicleContext ? { linkageTargetId: vehicleId } : {}),
      ...(vehicleCC > 0 ? { vehicleCC } : {}),
      ...(fuelType ? { fuelType } : {}),
      mode: searchMode || 'combined',
      enrichmentLevel: 'basic',
    });

    if (isOem) {
      searchOEM(trimmed, 20)
        .then(oemData => {
          if (!oemData) return;
          setOemResult({
            ...oemData,
            total: dedupeOEMResults(oemData.results).length,
            results: dedupeOEMResults(oemData.results),
          });
        })
        .catch(() => {});
    }
  }

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="rounded-[28px] border border-white/10 bg-white/95 p-6 text-slate-900 shadow-2xl shadow-slate-950/20">
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full border border-indigo-200 bg-indigo-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-indigo-800">
            OEM cross-reference
          </span>
          <span className="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-emerald-800">
            Smart search
          </span>
          <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-slate-700">
            Catalog handoff
          </span>
        </div>
        <h2 className="text-2xl font-semibold tracking-tight text-slate-950">Search by OEM, part number, or description</h2>
        <p className="mt-3 max-w-3xl text-sm leading-6 text-slate-600">
          This view combines OEM-reference evidence, owned catalog search, and cautious fallback signals. Click a part for full evidence or jump into catalog from a matched vehicle.
        </p>
        {hasVehicleContext && (
          <div data-testid="search-vehicle-context" className="mt-4 rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-900">
            Searching the confirmed vehicle context: <span className="font-semibold">{vehicleMake} {vehicleModel}</span>
            {vehicleCC > 0 ? ` · ${vehicleCC}cc` : ''}
            {fuelType ? ` · ${fuelType}` : ''}
            {sourceType ? ` · from ${sourceType}${sourceQuery ? ` ${sourceQuery}` : ''}` : ''}
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-6 flex flex-col gap-3">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-end">
            <div className="flex-1">
              <label className="mb-2 block text-sm font-medium text-slate-700">
                OEM / Part Number / Description
              </label>
              <input
                type="text"
                aria-label="OEM / Part Number / Description"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="e.g. 97133-D3000, OIL-01-0001, Oil Filter"
                className="w-full rounded-2xl border border-slate-300 px-4 py-3 text-lg font-mono text-slate-950 outline-none transition-colors focus:border-sky-500 focus:ring-2 focus:ring-sky-500/20"
              />
            </div>
            <button
              type="submit"
              disabled={!query.trim() || loading}
              className="rounded-2xl bg-slate-950 px-6 py-3 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {loading ? 'Searching...' : 'Search'}
            </button>
          </div>
          <SearchModeSelector value={searchMode} onChange={setSearchMode} />
        </form>
      </section>

      {/* Live search progress — shown while streaming */}
      <SearchProgress steps={steps} loading={loading} />

      {(error || streamError) && (
        <div className="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-red-700">
          {error || streamError}
        </div>
      )}

      {/* OEM First-Class Panel — shown when OEM endpoint returned results */}
      {oemResult && oemResult.total > 0 && (
        <OEMPanel
          result={oemResult}
          onOpenDetail={openOEMDetail}
          onOpenCatalog={openCatalogForVehicle}
        />
      )}

      {result && (
        <div>
          <div className="mb-4 flex items-center gap-4 flex-wrap">
            <span className="text-sm text-slate-300">
              {result.total} result{result.total !== 1 ? 's' : ''} for{' '}
              <span className="font-mono font-semibold text-white">{result.query}</span>
            </span>
            <StrategyBadge strategy={result.mode || result.searchStrategy} />
          </div>

          {/* Strategies summary bar for combined mode */}
          {result.mode === 'combined' && result.results.length > 0 && (
            <StrategiesSummaryBar
              strategies={result.results.map(r => r.sourceStrategy || '').filter(Boolean)}
            />
          )}

          {result.warnings && result.warnings.length > 0 && (
            <div className="mb-4 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
              {result.warnings.map((w, i) => (
                <p key={i}>{w}</p>
              ))}
            </div>
          )}

          {result.categories && result.categories.length > 0 && (
            <div className="flex flex-wrap gap-2 mb-4">
              {result.categories.map((cat) => (
                <span key={cat} className="px-2 py-1 bg-gray-100 text-gray-600 rounded text-xs">
                  {cat}
                </span>
              ))}
            </div>
          )}

          {result.vehicle && (
            <div data-testid="search-catalog-context" className="mb-4 rounded-2xl border border-blue-200 bg-blue-50 px-4 py-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold text-blue-900">Catalog context available</div>
                  <div className="text-sm text-blue-800">
                    {result.vehicle.make} {result.vehicle.model} {result.vehicle.description || ''}
                  </div>
                </div>
                <button
                  onClick={() => openCatalogForVehicle(result.vehicle!.make, result.vehicle!.model, result.vehicle!.linkageTargetId)}
                  data-testid="search-open-catalog"
                  className="rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"
                >
                  Open catalog
                </button>
              </div>
            </div>
          )}

          {result.results.length === 0 ? (
            <p className="text-gray-400 mt-6">No matches found.</p>
          ) : (
            <div className="space-y-3">
              {result.results.map((r) => {
                const cardKey = r.legacyArticleId ? `${r.legacyArticleId}` : `online-${r.articleNumber}`;
                return (
                <ResultCard
                  key={cardKey}
                  result={r}
                  onOpenDetail={() => openSearchDetail(r, result.searchStrategy)}
                  expanded={expandedId === cardKey}
                  onToggle={() =>
                    setExpandedId(expandedId === cardKey ? null : cardKey)
                  }
                  setQuery={setQuery}
                />
                );
              })}
            </div>
          )}
        </div>
      )}

      <PartDetailModal
        detail={selectedDetail}
        loading={detailLoading}
        error={detailError}
        onClose={() => {
          setSelectedDetail(null);
          setDetailLoading(false);
          setDetailError('');
        }}
      />
    </div>
  );
}

/** OEM First-Class Panel: shows decoded category, vehicle fits, cross-references */
function OEMPanel({
  result,
  onOpenDetail,
  onOpenCatalog,
}: {
  result: OEMSearchResponse;
  onOpenDetail: (ref: OEMReference) => void;
  onOpenCatalog: (make: string, model: string, linkageTargetId?: number, description?: string) => void;
}) {
  return (
    <div className="mb-6 rounded-[28px] border border-indigo-200 bg-indigo-50 p-5 text-slate-900 shadow-lg shadow-indigo-950/5">
      <div className="flex items-center gap-3 mb-3">
        <h3 className="text-sm font-bold text-indigo-900 uppercase tracking-wide">
          OEM Cross-Reference
        </h3>
        <span className="px-2 py-0.5 bg-indigo-100 text-indigo-700 rounded text-xs font-mono">
          {result.normalized || result.query}
        </span>
        {result.oemCategory && (
          <span className="px-2 py-0.5 bg-blue-100 text-blue-800 rounded text-xs font-medium">
            {result.oemCategory.system} → {result.oemCategory.subsystem}
          </span>
        )}
      </div>

      {/* Aftermarket matches */}
      {result.results.length > 0 && (
        <div className="mb-3">
          <h4 className="text-xs font-semibold text-indigo-700 mb-1.5">
            Matching Catalog Parts ({result.total})
          </h4>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {result.results.map((ref) => (
              <button
                key={ref.legacyArticleId || ref.rawNumber}
                onClick={() => onOpenDetail(ref)}
                data-testid="oem-result-card"
                className="flex w-full items-center gap-2 rounded border border-indigo-100 bg-white px-3 py-2 text-left hover:border-indigo-300"
              >
                <span data-testid="oem-result-article" className="font-mono text-sm font-semibold text-gray-900">
                  {ref.articleNumber || ref.rawNumber}
                </span>
                <span className="text-xs text-gray-500">{ref.brandName || ref.manufacturer}</span>
                {ref.description && (
                  <span className="text-xs text-gray-400 truncate ml-auto">{ref.description}</span>
                )}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Vehicles that use this OEM part */}
      {result.fitsVehicles && result.fitsVehicles.length > 0 && (
        <div>
          <h4 className="mb-2 text-xs font-semibold text-indigo-700">
            Fits Vehicles ({result.fitsVehicles.length})
          </h4>
          <div className="grid gap-3 xl:grid-cols-2">
            {result.fitsVehicles.map((v, i) => (
              <button
                key={i}
                onClick={() => onOpenCatalog(v.make, v.model, v.linkageTargetId, v.description)}
                data-testid="oem-fit-vehicle"
                className="overflow-hidden rounded-3xl border border-indigo-100 bg-white text-left transition-colors hover:border-indigo-300 hover:shadow-lg hover:shadow-indigo-100"
              >
                <div className="p-4">
                  <div className="mb-3 flex flex-wrap gap-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">
                    <span className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1">
                      Matched vehicle
                    </span>
                  </div>
                    <div className="font-medium text-slate-950">
                      {v.make} {v.model} {v.modelYear || ''}
                    </div>
                    <div className="mt-1 text-sm text-slate-600">
                      {v.description || 'Matched catalog vehicle'}
                    </div>
                    <div className="mt-3 text-sm font-medium text-indigo-700">Open this vehicle in catalog</div>
                </div>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function ResultCard({
  result: r,
  onOpenDetail,
  expanded,
  onToggle,
  setQuery,
}: {
  result: SmartResult;
  onOpenDetail: () => void;
  expanded: boolean;
  onToggle: () => void;
  setQuery: (q: string) => void;
}) {
  return (
    <div data-testid="search-result-card" className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden hover:border-blue-300 transition-colors">
      <div className="px-4 py-3 flex items-start gap-4">
        {/* Left: main info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <button data-testid="search-result-article" onClick={onOpenDetail} className="font-mono font-semibold text-gray-900 hover:text-blue-700 hover:underline">
              {r.articleNumber}
            </button>
            <span className="text-gray-400">·</span>
            <span className="text-sm text-gray-600">{r.brand || r.brandName}</span>
          </div>
          <p className="text-sm text-gray-700 mb-2">
            <button onClick={onOpenDetail} className="text-left hover:text-blue-700 hover:underline">
              {r.description}
            </button>
          </p>
          <div className="flex items-center gap-2 flex-wrap">
            <FitmentBadge driver={r.fitmentDriver} />
            <ConfidenceBadge value={r.confidence} />
            {r.sourceStrategy && <StrategyBadge strategy={r.sourceStrategy} />}
            {r.fitsVehicleCC && r.fitsVehicleCC > 0 && (
              <span className="text-xs text-gray-400">{r.fitsVehicleCC}cc</span>
            )}
          </div>
        </div>

        {/* Right: OEM numbers + actions */}
        <div className="text-right shrink-0">
          <button
            onClick={onOpenDetail}
            className="mb-2 block text-xs text-slate-600 hover:text-slate-900 hover:underline"
          >
            Details
          </button>
          {r.oemNumbers && r.oemNumbers.length > 0 && (
            <div className="mb-2">
              {r.oemNumbers.slice(0, 3).map((oem) => (
                <span
                  key={oem.rawNumber}
                  className="block text-xs font-mono text-gray-500"
                >
                  OEM: {oem.rawNumber}
                </span>
              ))}
              {r.oemNumbers.length > 3 && (
                <span className="text-xs text-gray-400">+{r.oemNumbers.length - 3} more</span>
              )}
            </div>
          )}
          {r.legacyArticleId > 0 && (
            <button
              onClick={onToggle}
              className="text-xs text-blue-600 hover:underline"
            >
              {expanded ? 'Hide chain' : 'Supersession chain'}
            </button>
          )}
        </div>
      </div>

      {r.confidenceNote && (
        <div className="px-4 pb-2">
          <p className="text-xs text-gray-400 italic">{r.confidenceNote}</p>
        </div>
      )}

      {/* S8-T6: vin_assembly distinctive note */}
      {r.sourceStrategy === 'vin_assembly' && (
        <div className="px-4 pb-2">
          <p className="text-xs text-emerald-600 italic">
            Matched via vehicle spec — verify fitment for your exact variant before ordering.
          </p>
        </div>
      )}

      {/* Substitutions (OEM replacements) */}
      {r.substitutions && r.substitutions.length > 0 && (
        <div className="border-t border-gray-100 px-4 py-3">
          <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
            Replacements / Substitutions
          </h4>
          <div className="flex flex-wrap gap-2">
            {r.substitutions.map((sub) => (
              <span
                key={sub.partNumber}
                className="inline-flex items-center gap-1 px-2 py-1 bg-green-50 text-green-800 rounded border border-green-200 text-xs font-mono cursor-pointer hover:bg-green-100"
                title={sub.description}
                onClick={() => { setQuery(sub.partNumber); setTimeout(() => { const form = document.querySelector('form'); form?.requestSubmit(); }, 50); }}
              >
                {sub.partNumber}
                <span className="font-sans text-green-600 text-[10px]">{sub.description}</span>
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Aftermarket alternatives */}
      {r.aftermarketAlternatives && r.aftermarketAlternatives.length > 0 && (
        <div className="border-t border-gray-100 px-4 py-3">
          <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
            Aftermarket Alternatives
          </h4>
          <div className="flex flex-wrap gap-2">
            {r.aftermarketAlternatives.map((alt) => (
              <span
                key={alt.brand + alt.partNumber}
                className="inline-flex items-center gap-1 px-2 py-1 bg-purple-50 text-purple-800 rounded border border-purple-200 text-xs"
              >
                <span className="font-semibold">{alt.brand}</span>
                <span className="font-mono">{alt.partNumber}</span>
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Vehicle compatibility chips (S3 enrichment) */}
      {r.compatibleVehicles && r.compatibleVehicles.length > 0 && (
        <div className="border-t border-gray-100 px-4 py-3">
          <CompatibilityChips vehicles={r.compatibleVehicles} maxVisible={4} />
        </div>
      )}

      {/* Specifications table (S3 enrichment) */}
      {r.specifications && r.specifications.length > 0 && (
        <div className="border-t border-gray-100 px-4 py-3">
          <SpecificationTable specs={r.specifications} />
        </div>
      )}

      {/* Documents and manuals (S3 enrichment, T-4C.4) */}
      {r.documents && r.documents.length > 0 && (
        <div className="border-t border-gray-100 px-4 py-3">
          <DocumentsList documents={r.documents} />
        </div>
      )}

      {/* Vehicle compatibility (legacy string list) */}
      {r.compatibility && r.compatibility.length > 0 && (!r.compatibleVehicles || r.compatibleVehicles.length === 0) && (
        <div className="border-t border-gray-100 px-4 py-3">
          <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
            Vehicle Compatibility
          </h4>
          <div className="flex flex-wrap gap-1">
            {r.compatibility.slice(0, 10).map((v) => (
              <span key={v} className="px-2 py-0.5 bg-gray-100 text-gray-600 rounded text-xs">
                {v}
              </span>
            ))}
            {r.compatibility.length > 10 && (
              <span className="text-xs text-gray-400">+{r.compatibility.length - 10} more</span>
            )}
          </div>
        </div>
      )}

      {expanded && r.legacyArticleId > 0 && (
        <div className="border-t border-gray-100 px-4 py-3 bg-gray-50">
          <SupersessionChain
            legacyArticleId={r.supersession ? undefined : r.legacyArticleId}
            chain={r.supersession}
          />
        </div>
      )}
    </div>
  );
}
