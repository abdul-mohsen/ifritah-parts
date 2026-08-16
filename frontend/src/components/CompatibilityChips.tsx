import type { CompatibleVehicle } from '../types';

interface Props {
  vehicles: CompatibleVehicle[];
  maxVisible?: number;
}

export function CompatibilityChips({ vehicles, maxVisible = 5 }: Props) {
  if (!vehicles || vehicles.length === 0) return null;

  const visible = vehicles.slice(0, maxVisible);
  const remaining = vehicles.length - visible.length;

  return (
    <div style={{ marginTop: '8px' }}>
      <div style={{ fontSize: '0.75rem', fontWeight: 600, color: '#374151', marginBottom: '4px' }}>
        Fits vehicles
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
        {visible.map((v, i) => (
          <span
            key={i}
            title={`${v.yearFrom ?? ''}–${v.yearTo ?? ''} ${v.fuelType ?? ''} ${v.capacityCC ? v.capacityCC + 'cc' : ''}`}
            style={{
              display: 'inline-block',
              padding: '2px 8px',
              borderRadius: '9999px',
              fontSize: '0.72rem',
              backgroundColor: '#eff6ff',
              color: '#1d4ed8',
              border: '1px solid #bfdbfe',
              whiteSpace: 'nowrap',
            }}
          >
            {v.vehicleName}
          </span>
        ))}
        {remaining > 0 && (
          <span
            style={{
              padding: '2px 8px',
              borderRadius: '9999px',
              fontSize: '0.72rem',
              backgroundColor: '#f9fafb',
              color: '#6b7280',
              border: '1px solid #e5e7eb',
            }}
          >
            +{remaining} more
          </span>
        )}
      </div>
    </div>
  );
}
