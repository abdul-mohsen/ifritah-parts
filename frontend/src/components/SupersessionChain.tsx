import { useEffect, useReducer } from 'react';
import type { SupersessionLink } from '../types';
import { getSupersessionChain } from '../api/client';

interface Props {
  legacyArticleId: number;
}

interface ChainState {
  chain: SupersessionLink[];
  error: string;
  loading: boolean;
}

type ChainAction =
  | { type: 'loading' }
  | { type: 'loaded'; chain: SupersessionLink[] }
  | { type: 'failed'; error: string };

const initialChainState: ChainState = {
  chain: [],
  error: '',
  loading: true,
};

function chainReducer(state: ChainState, action: ChainAction): ChainState {
  switch (action.type) {
    case 'loading':
      return { ...state, error: '', loading: true };
    case 'loaded':
      return { chain: action.chain, error: '', loading: false };
    case 'failed':
      return { ...state, error: action.error, loading: false };
  }
}

export default function SupersessionChain({ legacyArticleId }: Props) {
  const [state, dispatch] = useReducer(chainReducer, initialChainState);

  useEffect(() => {
    let cancelled = false;
    dispatch({ type: 'loading' });
    getSupersessionChain(legacyArticleId)
      .then((res) => {
        if (!cancelled) {
          dispatch({ type: 'loaded', chain: res.chain });
        }
      })
      .catch((err) => {
        if (!cancelled) {
          dispatch({
            type: 'failed',
            error: err instanceof Error ? err.message : 'Failed to load supersession chain.',
          });
        }
      });
    return () => { cancelled = true; };
  }, [legacyArticleId]);

  if (state.loading) return <p className="text-sm text-gray-400">Loading chain…</p>;
  if (state.error) return <p className="text-sm text-red-500">{state.error}</p>;
  if (state.chain.length === 0)
    return <p className="text-sm text-gray-400">No source-backed replacement links found.</p>;

  return (
    <div className="bg-gray-50 rounded-lg border border-gray-200 p-4">
      <h4 className="text-sm font-semibold text-gray-700 mb-1">Source-backed replacement links</h4>
      <p className="mb-3 text-xs text-gray-500">These links retain their source and caution level; they are not automatically OEM-confirmed supersessions.</p>
      <div className="space-y-2">
        {state.chain.map((link, i) => (
          <div key={`${link.articleNumber}-${link.direction}-${i}`} className="rounded border border-gray-200 bg-white px-3 py-2 text-sm">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-gray-400 text-xs">
                {link.direction === 'reported_related'
                  ? '↔'
                  : link.direction === 'replaces' || link.direction === 'reported_predecessor'
                    ? '←'
                    : '→'}
              </span>
              <span className="font-mono font-medium">{link.articleNumber}</span>
              <span className="rounded border border-amber-200 bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-800">
                {Math.round(link.confidence * 100)}% reported evidence
              </span>
              {link.brandName && <span className="text-gray-400 text-xs">({link.brandName})</span>}
            </div>
            {link.description && <p className="mt-1 text-xs text-gray-600">{link.description}</p>}
            <p className="mt-1 text-xs text-gray-500">
              <span className="font-medium text-gray-700">{link.source.label}:</span> {link.source.detail}
            </p>
            {link.warnings?.map((warning) => (
              <p key={warning} className="mt-1 text-xs text-amber-800">- {warning}</p>
            ))}
            </div>
        ))}
      </div>
    </div>
  );
}
