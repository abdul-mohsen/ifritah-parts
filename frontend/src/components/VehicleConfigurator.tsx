import type { Vehicle } from '../types';

interface Props {
  variants: Vehicle[];
  selected?: Vehicle;
  onSelect: (vehicle: Vehicle) => void;
}

export default function VehicleConfigurator({ variants, selected, onSelect }: Props) {
  if (variants.length <= 1) return null;

  // Group variants by fuel type for easier selection
  const byFuel = new Map<string, Vehicle[]>();
  for (const v of variants) {
    const fuel = v.fuelType || 'Unknown';
    if (!byFuel.has(fuel)) byFuel.set(fuel, []);
    byFuel.get(fuel)!.push(v);
  }

  return (
    <div className="rounded-3xl border border-amber-200 bg-amber-50/90 p-5 text-slate-900 shadow-lg shadow-amber-950/5">
      <div className="mb-3 flex items-center gap-2">
        <svg className="h-5 w-5 text-amber-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
        </svg>
        <h3 className="text-sm font-semibold text-amber-900">
          Multiple variants found — confirm your vehicle
        </h3>
      </div>
      <p className="mb-4 text-sm text-amber-900/80">
        {variants.length} variants match your VIN. Select the correct engine/specification
        to get accurate parts.
      </p>

      <div className="space-y-2">
        {Array.from(byFuel.entries()).map(([fuel, vehicles]) => (
          <div key={fuel}>
            <div className="mb-2 text-xs font-medium uppercase tracking-[0.18em] text-amber-800/80">
              {fuel}
            </div>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {vehicles.map((v) => {
                const isSelected = selected?.linkageTargetId === v.linkageTargetId;
                const yearRange = v.beginYearMonth && v.endYearMonth
                  ? `${Math.floor(v.beginYearMonth / 100)}-${Math.floor(v.endYearMonth / 100)}`
                  : '';

                return (
                  <button
                    key={v.linkageTargetId}
                    onClick={() => onSelect(v)}
                    className={`rounded-2xl border px-4 py-3 text-left text-sm transition-all ${
                      isSelected
                        ? 'border-slate-950 bg-slate-950 text-white shadow-lg shadow-slate-900/15'
                        : 'border-amber-200 bg-white text-slate-800 hover:border-amber-300 hover:bg-amber-100/50'
                    }`}
                  >
                    <div className="font-medium">
                      {v.capacityCC ? `${(v.capacityCC / 1000).toFixed(1)}L` : '?'}{' '}
                      {v.fuelType}{' '}
                      {v.horsePower ? `${v.horsePower}HP` : ''}
                    </div>
                    <div className={`mt-1 text-xs ${isSelected ? 'text-slate-300' : 'text-slate-500'}`}>
                      {v.description || `ID: ${v.linkageTargetId}`}
                      {yearRange && ` (${yearRange})`}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
