import { useState, useEffect } from 'react';
import type { Part } from '../types';
import { getPartsForVehicle, getAlternatives } from '../api/client';
import SupersessionChain from './SupersessionChain';

// Extended part with optional enrichment fields from the backend
interface EnrichedPart extends Part {
  oemNumbers?: string[];
  criteria?: Record<string, string>;
}

interface Props {
  linkageTargetId: number;
  initialParts?: Part[];
  totalParts: number;
}

const PAGE_SIZE = 20;

export default function PartsTable({ linkageTargetId, initialParts, totalParts }: Props) {
  const [parts, setParts] = useState<EnrichedPart[]>(initialParts ?? []);
  const [total, setTotal] = useState(totalParts);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [category, setCategory] = useState('');
  const [selectedArticle, setSelectedArticle] = useState<number | null>(null);
  const [expandedSpecs, setExpandedSpecs] = useState<number | null>(null);
  const [altCounts, setAltCounts] = useState<Record<number, number>>({});

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  useEffect(() => {
    if (page === 1 && initialParts && !category) return;
    let cancelled = false;
    setLoading(true);
    getPartsForVehicle(linkageTargetId, page, PAGE_SIZE, category, [], true)
      .then((res) => {
        if (cancelled) return;
        setParts(res.parts as EnrichedPart[]);
        setTotal(res.total);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [linkageTargetId, page, category]);

  // Fetch alternative counts for visible parts
  useEffect(() => {
    if (parts.length === 0) return;
    const ids = parts.map((p) => p.legacyArticleId).filter((id) => !altCounts[id]);
    if (ids.length === 0) return;

    ids.slice(0, 10).forEach((id) => {
      getAlternatives(id, linkageTargetId, 1)
        .then((res) => {
          setAltCounts((prev) => ({ ...prev, [id]: res.total }));
        })
        .catch(() => {});
    });
  }, [parts, linkageTargetId]);

  function handleCategoryChange(e: React.ChangeEvent<HTMLInputElement>) {
    setCategory(e.target.value);
    setPage(1);
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-semibold">
          Parts <span className="text-gray-400 font-normal">({total})</span>
        </h3>
        <input
          type="text"
          placeholder="Filter by category…"
          value={category}
          onChange={handleCategoryChange}
          className="rounded border border-gray-300 px-3 py-1.5 text-sm focus:ring-2 focus:ring-blue-500 outline-none w-56"
        />
      </div>

      <div className="overflow-x-auto rounded-lg border border-gray-200">
        <table className="min-w-full divide-y divide-gray-200 text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-gray-600">Article #</th>
              <th className="px-4 py-3 text-left font-medium text-gray-600">Brand</th>
              <th className="px-4 py-3 text-left font-medium text-gray-600">Description</th>
              <th className="px-4 py-3 text-left font-medium text-gray-600">Category</th>
              <th className="px-4 py-3 text-left font-medium text-gray-600">Info</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {loading ? (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-gray-400">
                  Loading…
                </td>
              </tr>
            ) : parts.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-gray-400">
                  No parts found.
                </td>
              </tr>
            ) : (
              parts.map((p) => (
                <PartRow
                  key={p.legacyArticleId}
                  part={p}
                  altCount={altCounts[p.legacyArticleId]}
                  specsExpanded={expandedSpecs === p.legacyArticleId}
                  chainExpanded={selectedArticle === p.legacyArticleId}
                  onToggleSpecs={() =>
                    setExpandedSpecs(expandedSpecs === p.legacyArticleId ? null : p.legacyArticleId)
                  }
                  onToggleChain={() =>
                    setSelectedArticle(selectedArticle === p.legacyArticleId ? null : p.legacyArticleId)
                  }
                />
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between mt-4 text-sm">
        <button
          disabled={page <= 1}
          onClick={() => setPage((p) => p - 1)}
          className="px-3 py-1.5 border rounded disabled:opacity-30 hover:bg-gray-50 transition-colors"
        >
          ← Prev
        </button>
        <span className="text-gray-500">
          Page {page} of {totalPages}
        </span>
        <button
          disabled={page >= totalPages}
          onClick={() => setPage((p) => p + 1)}
          className="px-3 py-1.5 border rounded disabled:opacity-30 hover:bg-gray-50 transition-colors"
        >
          Next →
        </button>
      </div>

      {selectedArticle !== null && (
        <div className="mt-4">
          <SupersessionChain legacyArticleId={selectedArticle} />
        </div>
      )}
    </div>
  );
}

function PartRow({
  part: p,
  altCount,
  specsExpanded,
  chainExpanded,
  onToggleSpecs,
  onToggleChain,
}: {
  part: EnrichedPart;
  altCount?: number;
  specsExpanded: boolean;
  chainExpanded: boolean;
  onToggleSpecs: () => void;
  onToggleChain: () => void;
}) {
  const hasCriteria = p.criteria && Object.keys(p.criteria).length > 0;
  const hasOEM = p.oemNumbers && p.oemNumbers.length > 0;

  return (
    <>
      <tr className="hover:bg-blue-50 transition-colors">
        <td className="px-4 py-2">
          <span className="font-mono">{p.articleNumber}</span>
          {hasOEM && (
            <div className="mt-0.5">
              {p.oemNumbers!.slice(0, 2).map((oem) => (
                <span key={oem} className="block text-[10px] font-mono text-gray-400">
                  OEM: {oem}
                </span>
              ))}
              {p.oemNumbers!.length > 2 && (
                <span className="text-[10px] text-gray-400">+{p.oemNumbers!.length - 2} more</span>
              )}
            </div>
          )}
        </td>
        <td className="px-4 py-2">{p.brandName ?? '—'}</td>
        <td className="px-4 py-2">{p.description}</td>
        <td className="px-4 py-2 text-xs text-gray-500">{p.category ?? '—'}</td>
        <td className="px-4 py-2">
          <div className="flex items-center gap-2">
            {hasCriteria && (
              <button
                onClick={onToggleSpecs}
                className={`text-xs px-1.5 py-0.5 rounded ${
                  specsExpanded ? 'bg-blue-100 text-blue-700' : 'text-blue-600 hover:bg-blue-50'
                }`}
                title="View specifications"
              >
                Specs
              </button>
            )}
            <button
              onClick={onToggleChain}
              className={`text-xs ${chainExpanded ? 'text-blue-700 font-medium' : 'text-blue-600 hover:underline'}`}
            >
              Chain
            </button>
            {altCount !== undefined && altCount > 0 && (
              <span className="text-[10px] text-green-600 font-medium" title="Also compatible alternatives">
                +{altCount} alt
              </span>
            )}
          </div>
        </td>
      </tr>
      {specsExpanded && hasCriteria && (
        <tr>
          <td colSpan={5} className="px-6 py-2 bg-gray-50">
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-x-4 gap-y-1 text-xs">
              {Object.entries(p.criteria!).map(([key, val]) => (
                <div key={key}>
                  <span className="text-gray-500">{key}:</span>{' '}
                  <span className="text-gray-800 font-medium">{val}</span>
                </div>
              ))}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
