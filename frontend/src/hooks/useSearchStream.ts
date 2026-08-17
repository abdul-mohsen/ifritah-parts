import { useCallback, useRef, useState } from 'react';
import type { SmartSearchResponse } from '../types';

export interface ProgressStep {
  step: string;
  label: string;
  elapsedMs: number;
  done: boolean;
  count?: number;
  error?: string;
}

export interface SearchStreamState {
  loading: boolean;
  steps: ProgressStep[];
  result: SmartSearchResponse | null;
  error: string;
}

type SearchStreamOptions = {
  q: string;
  linkageTargetId?: number;
  vehicleCC?: number;
  fuelType?: string;
  category?: string;
  mode?: string;
  enrichmentLevel?: string;
  limit?: number;
  page?: number;
};

/**
 * useSearchStream runs a search against /api/search/stream (SSE) and exposes
 * live progress steps so the UI can show what the server is doing.
 *
 * Falls back to /api/search (plain JSON) when EventSource is not available.
 */
export function useSearchStream() {
  const [state, setState] = useState<SearchStreamState>({
    loading: false,
    steps: [],
    result: null,
    error: '',
  });
  const esRef = useRef<EventSource | null>(null);

  const search = useCallback((opts: SearchStreamOptions) => {
    // Close any in-flight stream.
    esRef.current?.close();

    setState({ loading: true, steps: [], result: null, error: '' });

    const params = new URLSearchParams();
    if (opts.q) params.set('q', opts.q);
    if (opts.linkageTargetId) params.set('linkageTargetId', String(opts.linkageTargetId));
    if (opts.vehicleCC) params.set('vehicleCC', String(opts.vehicleCC));
    if (opts.fuelType) params.set('fuelType', opts.fuelType);
    if (opts.category) params.set('category', opts.category);
    if (opts.mode) params.set('mode', opts.mode);
    if (opts.enrichmentLevel) params.set('enrichmentLevel', opts.enrichmentLevel);
    if (opts.limit) params.set('limit', String(opts.limit));
    if (opts.page) params.set('page', String(opts.page));

    const url = `/api/search/stream?${params}`;
    const es = new EventSource(url);
    esRef.current = es;

    es.onmessage = (e) => {
      if (e.data === '[DONE]') {
        es.close();
        setState(prev => ({ ...prev, loading: false }));
        return;
      }
      try {
        const payload = JSON.parse(e.data) as {
          type: string;
          step?: string;
          label?: string;
          elapsed_ms?: number;
          done?: boolean;
          count?: number;
          error?: string;
          // result fields
          results?: SmartSearchResponse['results'];
          total?: number;
          searchStrategy?: string;
          mode?: string;
          warnings?: string[];
          categories?: string[];
          query?: string;
        };

        if (payload.type === 'progress') {
          setState(prev => {
            const steps = [...prev.steps];
            const idx = steps.findIndex(s => s.step === payload.step);
            const updated: ProgressStep = {
              step: payload.step ?? '',
              label: payload.label ?? '',
              elapsedMs: payload.elapsed_ms ?? 0,
              done: payload.done ?? false,
              count: payload.count,
            };
            if (idx >= 0) {
              steps[idx] = updated;
            } else {
              steps.push(updated);
            }
            return { ...prev, steps };
          });
        } else if (payload.type === 'result') {
          const result: SmartSearchResponse = {
            query: payload.query ?? opts.q,
            results: payload.results ?? [],
            total: payload.total ?? 0,
            searchStrategy: payload.searchStrategy ?? '',
            mode: payload.mode,
            warnings: payload.warnings,
            categories: payload.categories,
          };
          setState(prev => ({ ...prev, result }));
        } else if (payload.type === 'error') {
          setState(prev => ({
            ...prev,
            loading: false,
            error: payload.error ?? 'Search failed',
          }));
          es.close();
        }
      } catch {
        // malformed event — ignore
      }
    };

    es.onerror = () => {
      es.close();
      setState(prev => ({
        ...prev,
        loading: false,
        error: prev.result ? '' : 'Search stream disconnected',
      }));
    };

    return () => {
      es.close();
      esRef.current = null;
    };
  }, []);

  const cancel = useCallback(() => {
    esRef.current?.close();
    esRef.current = null;
    setState(prev => ({ ...prev, loading: false }));
  }, []);

  return { ...state, search, cancel };
}
