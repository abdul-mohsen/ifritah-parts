import type { ProgressStep } from '../hooks/useSearchStream';

interface Props {
  steps: ProgressStep[];
  loading: boolean;
}

// Keys match the actual strategy names emitted by internal/service/strategy.go
// searchCombined + smart_search.go SearchWithProgress. Any unknown step falls
// back to the "▸" bullet.
const STEP_ICONS: Record<string, string> = {
  // Strategy-level progress (per-strategy events from searchCombined)
  cache:            '💾',
  legacy:           '🕰',
  exact_oem:        '🔍',
  cross_reference:  '📋',
  vehicle_fitment:  '🚗',
  supersession:     '🔄',
  cross_brand:      '↔️',
  owned_catalog:    '🗄',
  keyword_gated:    '🔤',
  prefix_inference: '🧩',
  spec_match:       '📐',
  assembly_context: '🔩',
  vin_assembly:     '🆔',
  // Pipeline-level progress
  combined:         '⚡',
  search:           '🔎',
  enrichment:       '✨',
  done:             '✅',
};

function Spinner() {
  return (
    <span style={{ display: 'inline-block', animation: 'spin 0.8s linear infinite', fontSize: '0.85rem' }}>
      ⟳
    </span>
  );
}

export function SearchProgress({ steps, loading }: Props) {
  if (!loading && steps.length === 0) return null;

  const totalHits = steps.reduce((sum, s) => sum + (s.done && s.count ? s.count : 0), 0);
  const doneCount = steps.filter(s => s.done).length;

  return (
    <div style={{
      borderRadius: '12px',
      border: '1px solid #e2e8f0',
      backgroundColor: '#f8fafc',
      padding: '12px 16px',
      marginBottom: '16px',
      fontSize: '0.82rem',
    }}>
      <style>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        @keyframes pulse { 0%,100% { opacity:1; } 50% { opacity:0.5; } }
      `}</style>

      {/* Header — always shows something so the user never sees a blank spinner */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: '8px',
        paddingBottom: '6px',
        borderBottom: '1px solid #e2e8f0',
        color: '#374151',
        fontWeight: 600,
      }}>
        <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          {loading ? <Spinner /> : '✓'}
          {loading
            ? `Searching ${doneCount}/${steps.length || '…'} strategies`
            : `Search complete — ${doneCount} strategies ran`}
        </span>
        {totalHits > 0 && (
          <span style={{ fontSize: '0.72rem', color: '#065f46', fontWeight: 500 }}>
            {totalHits} hits across strategies
          </span>
        )}
      </div>

      {steps.length === 0 && loading && (
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: '#6b7280' }}>
          <Spinner />
          <span>Starting search…</span>
        </div>
      )}

      {steps.map((s) => {
        const icon = STEP_ICONS[s.step] ?? '▸';
        const isActive = !s.done && loading;
        const color = s.done ? '#10b981' : isActive ? '#3b82f6' : '#9ca3af';

        return (
          <div
            key={s.step}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              marginBottom: '4px',
              color,
              animation: isActive ? 'pulse 1.2s ease-in-out infinite' : 'none',
            }}
          >
            <span style={{ fontSize: '0.9rem', width: '18px', textAlign: 'center', flexShrink: 0 }}>
              {s.done ? '✓' : isActive ? <Spinner /> : icon}
            </span>
            <span style={{ flex: 1, color: s.done ? '#374151' : color }}>
              {s.label}
            </span>
            {s.done && s.count !== undefined && s.count > 0 && (
              <span style={{
                backgroundColor: '#d1fae5',
                color: '#065f46',
                borderRadius: '9999px',
                padding: '0 6px',
                fontSize: '0.72rem',
                fontWeight: 600,
              }}>
                {s.count} results
              </span>
            )}
            {s.done && s.count === 0 && (
              <span style={{
                color: '#9ca3af',
                fontSize: '0.72rem',
              }}>
                no match
              </span>
            )}
            {s.elapsedMs > 0 && (
              <span style={{ color: '#9ca3af', fontSize: '0.7rem', flexShrink: 0 }}>
                {s.elapsedMs < 1000 ? `${s.elapsedMs}ms` : `${(s.elapsedMs / 1000).toFixed(1)}s`}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}
