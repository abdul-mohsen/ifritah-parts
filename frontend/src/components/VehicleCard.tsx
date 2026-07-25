import type { NHTSAVehicle, Vehicle } from '../types';

interface Props {
  nhtsa: NHTSAVehicle;
  vehicle?: Vehicle;
  onOpenCatalog?: (vehicle: Vehicle) => void;
}

export default function VehicleCard({ nhtsa, vehicle, onOpenCatalog }: Props) {
  return (
    <div className="overflow-hidden rounded-3xl border border-white/10 bg-white/95 shadow-2xl shadow-slate-950/20">
      <div className="p-6 text-slate-900">
          <div className="mb-4 flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-sky-200 bg-sky-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-sky-800">
              NHTSA decode
            </span>
            {vehicle ? (
              <span className="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-emerald-800">
                Catalog candidate found
              </span>
            ) : null}
          </div>

          <h2 className="text-2xl font-semibold tracking-tight text-slate-950">
            {nhtsa.modelYear} {nhtsa.make} {nhtsa.model}
          </h2>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">
            Confirm the vehicle details before catalog browse. This card shows decoded vehicle facts only and avoids placeholder imagery.
          </p>

          <dl className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            <Stat label="Body class" value={nhtsa.bodyClass} />
            <Stat label="Drive type" value={nhtsa.driveType} />
            <Stat label="Fuel type" value={nhtsa.fuelType} />
            <Stat
              label="Engine"
              value={nhtsa.engineDisplacementCC
                ? `${nhtsa.engineDisplacementCC}cc${nhtsa.engineNumberOfCylinders ? ` • ${nhtsa.engineNumberOfCylinders} cyl` : ''}`
                : undefined}
            />
            <Stat label="Catalog linkage" value={vehicle ? String(vehicle.linkageTargetId) : undefined} mono />
            <Stat label="Catalog match" value={vehicle?.description || `${nhtsa.make} ${nhtsa.model}`} />
          </dl>

          {vehicle && onOpenCatalog ? (
            <div className="mt-6 flex flex-wrap gap-3">
              <button
                onClick={() => onOpenCatalog(vehicle)}
                className="rounded-2xl bg-slate-950 px-4 py-3 text-sm font-medium text-white transition-colors hover:bg-slate-800"
              >
                Open this vehicle in catalog
              </button>
            </div>
          ) : null}
      </div>
    </div>
  );
}

function Stat({ label, value, mono = false }: { label: string; value?: string; mono?: boolean }) {
  return (
    <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
      <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">{label}</div>
      <div className={`mt-1 text-sm text-slate-900 ${mono ? 'font-mono' : 'font-medium'}`}>
        {value || 'Not decoded'}
      </div>
    </div>
  );
}
