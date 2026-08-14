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
    <div className="bg-amber-900/20 border border-amber-700/50 rounded-lg p-4">
      <div className="flex items-center gap-2 mb-3">
        <svg className="w-5 h-5 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
        </svg>
        <h3 className="text-amber-300 font-semibold text-sm">
          Multiple variants found — confirm your vehicle
        </h3>
      </div>
      <p className="text-gray-400 text-xs mb-3">
        {variants.length} variants match your VIN. Select the correct engine/specification
        to get accurate parts.
      </p>

      <div className="space-y-2">
        {Array.from(byFuel.entries()).map(([fuel, vehicles]) => (
          <div key={fuel}>
            <div className="text-xs text-gray-500 font-medium uppercase tracking-wide mb-1">
              {fuel}
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5">
              {vehicles.map((v) => {
                const isSelected = selected?.linkageTargetId === v.linkageTargetId;
                const yearRange = v.beginYearMonth && v.endYearMonth
                  ? `${Math.floor(v.beginYearMonth / 100)}-${Math.floor(v.endYearMonth / 100)}`
                  : '';

                return (
                  <button
                    key={v.linkageTargetId}
                    onClick={() => onSelect(v)}
                    className={`text-left px-3 py-2.5 rounded-lg text-sm transition-all border ${
                      isSelected
                        ? 'bg-blue-700/30 border-blue-500 text-white ring-1 ring-blue-500'
                        : 'bg-gray-800/50 border-gray-700 text-gray-300 hover:bg-gray-700/50 hover:border-gray-600'
                    }`}
                  >
                    <div className="font-medium">
                      {v.capacityCC ? `${(v.capacityCC / 1000).toFixed(1)}L` : '?'}{' '}
                      {v.fuelType}{' '}
                      {v.horsePower ? `${v.horsePower}HP` : ''}
                    </div>
                    <div className="text-xs text-gray-500 mt-0.5">
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
