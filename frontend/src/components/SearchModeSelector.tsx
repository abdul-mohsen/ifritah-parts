import { useEffect, useState } from 'react';
import { getSearchModes } from '../api/client';
import type { SearchMode } from '../types';

interface Props {
  value: string;
  onChange: (mode: string) => void;
}

const STRATEGY_COLORS: Record<string, string> = {
  exact_oem:        '#22c55e',
  cross_reference:  '#3b82f6',
  vehicle_fitment:  '#8b5cf6',
  supersession:     '#f59e0b',
  cross_brand:      '#f97316',
  spec_match:       '#06b6d4',
  assembly_context: '#ec4899',
  vin_assembly:     '#10b981',
  combined:         '#6366f1',
};

export function SearchModeSelector({ value, onChange }: Props) {
  const [modes, setModes] = useState<SearchMode[]>([]);

  useEffect(() => {
    getSearchModes()
      .then(res => setModes(res.modes))
      .catch(() => {});
  }, []);

  if (modes.length === 0) return null;

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
      <label style={{ fontSize: '0.875rem', color: '#6b7280', whiteSpace: 'nowrap' }}>
        Search mode:
      </label>
      <select
        value={value}
        onChange={e => onChange(e.target.value)}
        style={{
          padding: '4px 8px',
          borderRadius: '6px',
          border: '1px solid #d1d5db',
          fontSize: '0.875rem',
          backgroundColor: '#fff',
          cursor: 'pointer',
        }}
      >
        <option value="">Auto (legacy cascade)</option>
        {modes.map(m => (
          <option key={m.key} value={m.key} title={m.description}>
            {m.name}
          </option>
        ))}
      </select>
      {value && (
        <span
          style={{
            fontSize: '0.75rem',
            color: '#6b7280',
            maxWidth: '300px',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
          title={modes.find(m => m.key === value)?.description}
        >
          {modes.find(m => m.key === value)?.description}
        </span>
      )}
    </div>
  );
}

export function StrategyBadge({ strategy }: { strategy?: string }) {
  if (!strategy) return null;
  const color = STRATEGY_COLORS[strategy] ?? '#9ca3af';
  const label = strategy.replace(/_/g, ' ');
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '1px 7px',
        borderRadius: '9999px',
        fontSize: '0.7rem',
        fontWeight: 600,
        backgroundColor: color + '22',
        color,
        border: `1px solid ${color}55`,
        whiteSpace: 'nowrap',
      }}
      title={`Returned by ${label} strategy`}
    >
      {label}
    </span>
  );
}

export function StrategiesSummaryBar({ strategies }: { strategies: string[] }) {
  const unique = Array.from(new Set(strategies.filter(Boolean)));
  if (unique.length === 0) return null;
  return (
    <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginBottom: '8px' }}>
      <span style={{ fontSize: '0.75rem', color: '#6b7280' }}>Strategies used:</span>
      {unique.map(s => <StrategyBadge key={s} strategy={s} />)}
    </div>
  );
}
