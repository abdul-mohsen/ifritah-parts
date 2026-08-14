import { useState, useEffect } from 'react';
import type { SupersessionLink } from '../types';
import { getSupersessionChain } from '../api/client';

interface Props {
  legacyArticleId: number;
}

export default function SupersessionChain({ legacyArticleId }: Props) {
  const [chain, setChain] = useState<SupersessionLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    getSupersessionChain(legacyArticleId)
      .then((res) => {
        if (!cancelled) setChain(res.chain);
      })
      .catch((err) => {
        if (!cancelled) setError(err.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [legacyArticleId]);

  if (loading) return <p className="text-sm text-gray-400">Loading chain…</p>;
  if (error) return <p className="text-sm text-red-500">{error}</p>;
  if (chain.length === 0)
    return <p className="text-sm text-gray-400">No supersession chain found.</p>;

  return (
    <div className="bg-gray-50 rounded-lg border border-gray-200 p-4">
      <h4 className="text-sm font-semibold text-gray-700 mb-3">Supersession Chain</h4>
      <div className="flex items-center gap-2 flex-wrap">
        {chain.map((link, i) => (
          <div key={i} className="flex items-center gap-2">
            {i > 0 && (
              <span className="text-gray-400 text-xs">
                {link.direction === 'replaced_by' ? '→' : '←'}
              </span>
            )}
            <div className="bg-white border border-gray-200 rounded px-3 py-1.5 text-sm">
              <span className="font-mono font-medium">{link.articleNumber}</span>
              {link.brandName && (
                <span className="text-gray-400 ml-1 text-xs">({link.brandName})</span>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
