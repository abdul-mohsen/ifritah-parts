import { useEffect, useReducer } from 'react';
import type { SupersessionLink, SupersessionChain as SupersessionChainType } from '../types';
import { getSupersessionChain } from '../api/client';

// Props can accept either a legacyArticleId (fetch mode) or an already-loaded chain (inline mode)
interface Props {
  legacyArticleId?: number;
  chain?: SupersessionChainType; // S3: inline enriched chain
}

interface ChainState {
  links: SupersessionLink[];
  truncated: boolean;
  error: string;
  loading: boolean;
}

type ChainAction =
  | { type: 'loading' }
  | { type: 'loaded'; links: SupersessionLink[]; truncated: boolean }
  | { type: 'failed'; error: string };

const initialChainState: ChainState = {
  links: [],
  truncated: false,
  error: '',
  loading: false,
};

function chainReducer(state: ChainState, action: ChainAction): ChainState {
  switch (action.type) {
    case 'loading':
      return { ...state, error: '', loading: true };
    case 'loaded':
      return { links: action.links, truncated: action.truncated, error: '', loading: false };
    case 'failed':
      return { ...state, error: action.error, loading: false };
  }
}

function SupersessionLinkItem({ link, i }: { link: SupersessionLink; i: number }) {
  return (
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
          {Math.round(link.confidence * 100)}% {link.source?.label ?? 'evidence'}
        </span>
        {link.brandName && <span className="text-gray-400 text-xs">({link.brandName})</span>}
      </div>
      {link.description && <p className="mt-1 text-xs text-gray-600">{link.description}</p>}
      {link.source && (
        <p className="mt-1 text-xs text-gray-500">
          <span className="font-medium text-gray-700">{link.source.label}:</span> {link.source.detail}
        </p>
      )}
      {link.warnings?.map((warning) => (
        <p key={warning} className="mt-1 text-xs text-amber-800">- {warning}</p>
      ))}
    </div>
  );
}

export default function SupersessionChain({ legacyArticleId, chain: inlineChain }: Props) {
  const [state, dispatch] = useReducer(chainReducer, {
    ...initialChainState,
    loading: !inlineChain && !!legacyArticleId,
  });

  useEffect(() => {
    // If we have an inline chain (from enrichment), use it directly
    if (inlineChain) {
      const links = [
        ...(inlineChain.replacedBy ?? []),
        inlineChain.current,
        ...(inlineChain.replaces ?? []),
      ].filter(Boolean);
      dispatch({ type: 'loaded', links, truncated: inlineChain.truncated ?? false });
      return;
    }
    if (!legacyArticleId) return;

    let cancelled = false;
    dispatch({ type: 'loading' });
    getSupersessionChain(legacyArticleId)
      .then((res) => {
        if (!cancelled) {
          dispatch({ type: 'loaded', links: res.chain, truncated: false });
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
  }, [legacyArticleId, inlineChain]);

  if (state.loading) return <p className="text-sm text-gray-400">Loading chain…</p>;
  if (state.error) return <p className="text-sm text-red-500">{state.error}</p>;
  if (state.links.length === 0)
    return <p className="text-sm text-gray-400">No source-backed replacement links found.</p>;

  return (
    <div className="bg-gray-50 rounded-lg border border-gray-200 p-4">
      <h4 className="text-sm font-semibold text-gray-700 mb-1">Source-backed replacement links</h4>
      <p className="mb-3 text-xs text-gray-500">
        These links retain their source and caution level; they are not automatically OEM-confirmed supersessions.
        {state.truncated && ' Chain was truncated at depth limit.'}
      </p>
      <div className="space-y-2">
        {state.links.map((link, i) => <SupersessionLinkItem key={i} link={link} i={i} />)}
      </div>
    </div>
  );
}
