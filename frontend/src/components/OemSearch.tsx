import { useState } from 'react';
import type { SmartSearchResponse, SmartResult, OEMSearchResponse } from '../types';
import { smartSearch, searchOEM } from '../api/client';
import SupersessionChain from './SupersessionChain';

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

export default function OemSearch() {
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<SmartSearchResponse | null>(null);
  const [oemResult, setOemResult] = useState<OEMSearchResponse | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = query.trim();
    if (!trimmed) return;
    setError('');
    setLoading(true);
    setExpandedId(null);
    setOemResult(null);
    try {
      // Fire both queries in parallel when it looks like an OEM number
      const isOem = isLikelyOEM(trimmed);
      const [smartData, oemData] = await Promise.all([
        smartSearch(trimmed, { limit: 50 }),
        isOem ? searchOEM(trimmed, 20) : Promise.resolve(null),
      ]);
      if (smartData.results == null) smartData.results = [];
      setResult(smartData);
      setOemResult(oemData);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Search failed');
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-5xl mx-auto">
      <form onSubmit={handleSubmit} className="flex gap-3 items-end mb-6">
        <div className="flex-1">
          <label className="block text-sm font-medium text-gray-700 mb-1">
            OEM / Part Number / Description
          </label>
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="e.g. 97133-D3000, OIL-01-0001, Oil Filter"
            className="w-full rounded-lg border border-gray-300 px-4 py-2.5 text-lg font-mono focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
          />
        </div>
        <button
          type="submit"
          disabled={!query.trim() || loading}
          className="px-6 py-2.5 bg-green-600 text-white rounded-lg font-medium hover:bg-green-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          {loading ? 'Searching…' : 'Search'}
        </button>
      </form>

      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-6">
          {error}
        </div>
      )}

      {/* OEM First-Class Panel — shown when OEM endpoint returned results */}
      {oemResult && oemResult.total > 0 && (
        <OEMPanel result={oemResult} />
      )}

      {result && (
        <div>
          <div className="flex items-center gap-4 mb-4">
            <span className="text-sm text-gray-500">
              {result.total} result{result.total !== 1 ? 's' : ''} for{' '}
              <span className="font-mono font-semibold">{result.query}</span>
            </span>
            <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium border ${
              result.searchStrategy === 'online_partsouq'
                ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
                : 'bg-indigo-50 text-indigo-700 border-indigo-200'
            }`}>
              {result.searchStrategy === 'online_partsouq' ? 'Online Lookup' : `strategy: ${result.searchStrategy}`}
            </span>
          </div>

          {result.warnings && result.warnings.length > 0 && (
            <div className="bg-amber-50 border border-amber-200 text-amber-800 px-4 py-3 rounded-lg mb-4 text-sm">
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
    </div>
  );
}

/** OEM First-Class Panel: shows decoded category, vehicle fits, cross-references */
function OEMPanel({ result }: { result: OEMSearchResponse }) {
  return (
    <div className="bg-indigo-50 border border-indigo-200 rounded-lg p-4 mb-6">
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
            Aftermarket Parts ({result.total})
          </h4>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {result.results.map((ref) => (
              <div
                key={ref.legacyArticleId || ref.rawNumber}
                className="flex items-center gap-2 px-3 py-2 bg-white rounded border border-indigo-100"
              >
                <span className="font-mono text-sm font-semibold text-gray-900">
                  {ref.articleNumber || ref.rawNumber}
                </span>
                <span className="text-xs text-gray-500">{ref.brandName || ref.manufacturer}</span>
                {ref.description && (
                  <span className="text-xs text-gray-400 truncate ml-auto">{ref.description}</span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Vehicles that use this OEM part */}
      {result.fitsVehicles && result.fitsVehicles.length > 0 && (
        <div>
          <h4 className="text-xs font-semibold text-indigo-700 mb-1.5">
            Fits Vehicles ({result.fitsVehicles.length})
          </h4>
          <div className="flex flex-wrap gap-1.5">
            {result.fitsVehicles.map((v, i) => (
              <span
                key={i}
                className="px-2 py-0.5 bg-white text-gray-700 rounded border border-gray-200 text-xs"
              >
                {v.make} {v.model} {v.modelYear || ''} {v.description || ''}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function ResultCard({
  result: r,
  expanded,
  onToggle,
  setQuery,
}: {
  result: SmartResult;
  expanded: boolean;
  onToggle: () => void;
  setQuery: (q: string) => void;
}) {
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden hover:border-blue-300 transition-colors">
      <div className="px-4 py-3 flex items-start gap-4">
        {/* Left: main info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className="font-mono font-semibold text-gray-900">{r.articleNumber}</span>
            <span className="text-gray-400">·</span>
            <span className="text-sm text-gray-600">{r.brand || r.brandName}</span>
          </div>
          <p className="text-sm text-gray-700 mb-2">{r.description}</p>
          <div className="flex items-center gap-2 flex-wrap">
            <FitmentBadge driver={r.fitmentDriver} />
            <ConfidenceBadge value={r.confidence} />
            {r.fitsVehicleCC && r.fitsVehicleCC > 0 && (
              <span className="text-xs text-gray-400">{r.fitsVehicleCC}cc</span>
            )}
          </div>
        </div>

        {/* Right: OEM numbers + actions */}
        <div className="text-right shrink-0">
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

      {/* Vehicle compatibility */}
      {r.compatibility && r.compatibility.length > 0 && (
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
          <SupersessionChain legacyArticleId={r.legacyArticleId} />
        </div>
      )}
    </div>
  );
}
