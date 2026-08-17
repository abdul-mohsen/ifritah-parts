import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { VINDecodeResponse, Vehicle, Part } from '../types';
import { decodeVIN } from '../api/client';
import VehicleCard from './VehicleCard';
import PartsTable from './PartsTable';
import CrossBrandBadge from './CrossBrandBadge';
import RecallBanner from './RecallBanner';
import CategoryPicker from './CategoryPicker';
import VehicleConfigurator from './VehicleConfigurator';
import PlatformBadge from './PlatformBadge';

const VIN_RE = /^[A-HJ-NPR-Z0-9]{17}$/i;

export default function VinInput() {
  const navigate = useNavigate();
  const [vin, setVin] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<VINDecodeResponse | null>(null);
  const [confirmedVehicle, setConfirmedVehicle] = useState<Vehicle | null>(null);
  const [categoryParts, setCategoryParts] = useState<Part[] | null>(null);
  const [categoryTotal, setCategoryTotal] = useState(0);
  const [selectedCategory, setSelectedCategory] = useState('');
  const [partSearchQuery, setPartSearchQuery] = useState('');

  const valid = VIN_RE.test(vin);

  // The active vehicle is either user-confirmed or auto-selected
  const activeVehicle = confirmedVehicle || result?.vehicle || null;
  const catalogVehicle = result?.needsConfirmation ? confirmedVehicle : activeVehicle;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setError('');
    setLoading(true);
    setCategoryParts(null);
    setSelectedCategory('');
    setConfirmedVehicle(null);
    try {
      const data = await decodeVIN(vin.toUpperCase());
      setResult(data);
      // Auto-confirm if only one variant
      if (!data.needsConfirmation && data.vehicle) {
        setConfirmedVehicle(data.vehicle);
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Decode failed');
    } finally {
      setLoading(false);
    }
  }

  function handleVariantSelect(vehicle: Vehicle) {
    setConfirmedVehicle(vehicle);
    setCategoryParts(null);
    setSelectedCategory('');
  }

  function handlePartsLoaded(parts: Part[], total: number, category: string) {
    setCategoryParts(parts);
    setCategoryTotal(total);
    setSelectedCategory(category);
  }

  function openCatalogForVehicle(vehicle: Vehicle) {
    const make = result?.nhtsaRaw?.make || vehicle.make;
    const model = result?.nhtsaRaw?.model || vehicle.model;
    const params = new URLSearchParams({
      make,
      model,
      vehicleId: String(vehicle.linkageTargetId),
      sourceType: 'VIN',
      sourceQuery: vin.toUpperCase(),
    });
    navigate(`/catalog?${params.toString()}`);
  }

  function openVehiclePartSearch(event: React.FormEvent) {
    event.preventDefault();
    if (!catalogVehicle || !partSearchQuery.trim()) return;

    const params = new URLSearchParams({
      q: partSearchQuery.trim(),
      vehicleId: String(catalogVehicle.linkageTargetId),
      make: result?.nhtsaRaw?.make || catalogVehicle.make,
      model: result?.nhtsaRaw?.model || catalogVehicle.model,
      sourceType: 'VIN',
      sourceQuery: vin.toUpperCase(),
      // S8-T5: default to vin_assembly when vehicle context is confirmed
      mode: 'vin_assembly',
    });
    if (catalogVehicle.capacityCC) params.set('vehicleCC', String(catalogVehicle.capacityCC));
    if (catalogVehicle.fuelType) params.set('fuelType', catalogVehicle.fuelType);
    navigate(`/oem?${params.toString()}`);
  }

  const matchedVehicles = result?.allVariants?.length
    ? result.allVariants
    : activeVehicle
      ? [activeVehicle]
      : [];

  return (
    <div className="mx-auto max-w-7xl space-y-8">
      <section className="grid gap-6 lg:grid-cols-[1.15fr,0.85fr]">
        <div className="rounded-[28px] border border-white/10 bg-white/95 p-6 text-slate-900 shadow-2xl shadow-slate-950/20">
          <div className="mb-4 flex flex-wrap gap-2">
            <span className="rounded-full border border-sky-200 bg-sky-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-sky-800">
              NHTSA decode
            </span>
            <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-slate-700">
              Variant confirmation
            </span>
            <span className="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-emerald-800">
              Catalog handoff
            </span>
          </div>

          <h2 className="text-2xl font-semibold tracking-tight text-slate-950">
            Decode VIN and confirm the exact vehicle before parts browse
          </h2>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-600">
            This flow uses NHTSA-backed decode data and then asks the user to confirm the closest catalog vehicle.
            The confirmation tiles below stay text-first and avoid placeholder vehicle imagery.
          </p>

          <form onSubmit={handleSubmit} className="mt-6 flex flex-col gap-3 lg:flex-row lg:items-end">
            <div className="flex-1">
              <label className="mb-2 block text-sm font-medium text-slate-700">
                VIN (17 characters)
              </label>
              <input
                type="text"
                aria-label="VIN (17 characters)"
                maxLength={17}
                value={vin}
                onChange={(e) => setVin(e.target.value.toUpperCase())}
                placeholder="e.g. KM8J33A46GU123456"
                className="w-full rounded-2xl border border-slate-300 bg-white px-4 py-3 text-lg font-mono tracking-[0.25em] text-slate-950 outline-none transition-colors focus:border-sky-500 focus:ring-2 focus:ring-sky-500/20"
              />
            </div>
            <button
              type="submit"
              disabled={!valid || loading}
              className="rounded-2xl bg-slate-950 px-6 py-3 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {loading ? 'Decoding…' : 'Decode VIN'}
            </button>
          </form>
        </div>

        <div className="rounded-[28px] border border-white/10 bg-slate-950/60 p-6 shadow-2xl shadow-slate-950/20">
          <div className="text-xs font-semibold uppercase tracking-[0.22em] text-slate-300">What this stage verifies</div>
          <div className="mt-4 space-y-4 text-sm text-slate-200">
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="font-medium text-white">1. VIN identity</div>
              <div className="mt-1 text-slate-300">Make, model, year, body class, engine displacement, and fuel type from the NHTSA decode path.</div>
            </div>
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="font-medium text-white">2. Catalog variant confirmation</div>
              <div className="mt-1 text-slate-300">When multiple Hyundai/Kia variants match, the user must choose before catalog browse continues.</div>
            </div>
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="font-medium text-white">3. Safe browse handoff</div>
              <div className="mt-1 text-slate-300">Only confirmed catalog vehicles get the full catalog and part-detail workflow.</div>
            </div>
          </div>
        </div>
      </section>

      {error && (
        <div className="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-red-700">
          {error}
        </div>
      )}

      {result && (
        <div className="space-y-6">
          {result.nhtsaRaw && (
            <VehicleCard
              nhtsa={result.nhtsaRaw}
              vehicle={result.vehicle}
              onOpenCatalog={openCatalogForVehicle}
            />
          )}

          {matchedVehicles.length > 0 && (
            <div data-testid="vin-catalog-matches" className="rounded-[28px] border border-white/10 bg-white/95 p-5 text-slate-900 shadow-2xl shadow-slate-950/20">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h3 className="text-lg font-semibold text-slate-950">Confirm the catalog vehicle</h3>
                  <p className="text-sm text-slate-600">
                    Review the matched vehicles and jump directly into that vehicle&apos;s catalog.
                  </p>
                </div>
                <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-medium text-slate-600">
                  {matchedVehicles.length} match{matchedVehicles.length === 1 ? '' : 'es'}
                </span>
              </div>

              <div className="grid gap-4 xl:grid-cols-2">
                {matchedVehicles.map((vehicle) => (
                  <div key={vehicle.linkageTargetId} data-testid="vin-match-card" className="overflow-hidden rounded-3xl border border-slate-200 bg-white">
                    <div className="p-4">
                      <div className="mb-3 flex flex-wrap gap-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">
                        <span className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1">
                          Candidate vehicle
                        </span>
                        {result.nhtsaRaw?.bodyClass ? (
                          <span className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1">
                            {result.nhtsaRaw.bodyClass}
                          </span>
                        ) : null}
                      </div>
                        <div className="font-medium text-slate-950">
                          {vehicle.description || `${vehicle.make} ${vehicle.model}`}
                        </div>
                        <div className="mt-2 flex flex-wrap gap-2 text-xs text-slate-600">
                          <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">ID {vehicle.linkageTargetId}</span>
                          {vehicle.fuelType && <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{vehicle.fuelType}</span>}
                          {vehicle.capacityCC ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{vehicle.capacityCC}cc</span> : null}
                          {vehicle.horsePower ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{vehicle.horsePower}HP</span> : null}
                        </div>
                        <p className="mt-3 text-sm leading-6 text-slate-600">
                          Use this confirmation card if it matches the customer vehicle, then move into catalog or stay in-place and browse category results.
                        </p>
                        <div className="mt-4 flex gap-2">
                          <button
                            onClick={() => handleVariantSelect(vehicle)}
                            className="rounded-2xl border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50"
                          >
                            Use here
                          </button>
                          <button
                            onClick={() => openCatalogForVehicle(vehicle)}
                            data-testid="vin-open-catalog"
                            className="rounded-2xl bg-slate-950 px-3 py-2 text-sm font-medium text-white hover:bg-slate-800"
                          >
                            Open catalog
                          </button>
                        </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Vehicle Configurator — shown when multiple variants match */}
          {result.needsConfirmation && result.allVariants && (
            <VehicleConfigurator
              variants={result.allVariants}
              selected={confirmedVehicle ?? undefined}
              onSelect={handleVariantSelect}
            />
          )}

          {result.crossBrand && result.crossBrand.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {result.crossBrand.map((cb, i) => (
                <CrossBrandBadge key={i} hit={cb} />
              ))}
            </div>
          )}

          {result.recalls && result.recalls.length > 0 && (
            <RecallBanner recalls={result.recalls} />
          )}

          {/* Platform Compatibility */}
          {catalogVehicle && result.nhtsaRaw && (
            <PlatformBadge
              linkageTargetId={catalogVehicle.linkageTargetId}
              make={result.nhtsaRaw.make}
              model={result.nhtsaRaw.model}
            />
          )}

          {catalogVehicle && (
            <section data-testid="vin-part-search" className="rounded-[28px] border border-white/10 bg-white/95 p-5 text-slate-900 shadow-2xl shadow-slate-950/20">
              <div className="mb-4">
                <div className="flex items-center gap-2 mb-1">
                  <div className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">Confirmed vehicle search</div>
                  {/* S8-T6: vin_assembly mode badge */}
                  <span style={{
                    display: 'inline-block',
                    padding: '1px 7px',
                    borderRadius: '9999px',
                    fontSize: '0.7rem',
                    fontWeight: 600,
                    backgroundColor: '#10b98122',
                    color: '#10b981',
                    border: '1px solid #10b98155',
                    whiteSpace: 'nowrap',
                  }} title="Results matched via vehicle engine and chassis specifications">
                    vehicle spec match
                  </span>
                </div>
                <h3 className="text-lg font-semibold text-slate-950">Find a part for this vehicle</h3>
                <p className="mt-1 text-sm text-slate-600">
                  Results are matched to{' '}
                  <span className="font-medium">
                    {catalogVehicle.capacityCC ? `${(catalogVehicle.capacityCC / 1000).toFixed(1)}L ` : ''}
                    {catalogVehicle.fuelType ? `${catalogVehicle.fuelType} ` : ''}
                    {catalogVehicle.description || `${catalogVehicle.make} ${catalogVehicle.model}`}
                  </span>{' '}
                  by engine and chassis specs — finds parts even when not explicitly linked in the parts database.
                </p>
              </div>
              <form onSubmit={openVehiclePartSearch} className="flex flex-col gap-3 sm:flex-row">
                <input
                  aria-label="Part name for confirmed vehicle"
                  value={partSearchQuery}
                  onChange={(event) => setPartSearchQuery(event.target.value)}
                  placeholder="e.g. spark plug, oil filter, timing belt"
                  className="min-w-0 flex-1 rounded-2xl border border-slate-300 bg-white px-4 py-3 text-slate-950 outline-none transition-colors focus:border-sky-500 focus:ring-2 focus:ring-sky-500/20"
                />
                <button
                  type="submit"
                  disabled={!partSearchQuery.trim()}
                  className="rounded-2xl bg-slate-950 px-4 py-3 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  Search by vehicle spec
                </button>
              </form>
            </section>
          )}

          {/* Hierarchical Category Tree */}
          {catalogVehicle && (
            <div>
              <h3 className="mb-3 text-lg font-semibold text-white">Select part category</h3>
              <CategoryPicker
                linkageTargetId={catalogVehicle.linkageTargetId}
                onPartsLoaded={handlePartsLoaded}
              />
            </div>
          )}

          {/* Parts from category selection */}
          {categoryParts && catalogVehicle && (
            <div>
              <h3 className="text-lg font-semibold mb-2">
                {selectedCategory}{' '}
                <span className="text-gray-500 text-sm font-normal">
                  ({categoryTotal} parts)
                </span>
              </h3>
              <PartsTable
                linkageTargetId={catalogVehicle.linkageTargetId}
                initialParts={categoryParts}
                totalParts={categoryTotal}
              />
            </div>
          )}

          {/* Initial parts (before category selection) */}
          {!categoryParts && catalogVehicle && result.parts && (
            <PartsTable
              linkageTargetId={catalogVehicle.linkageTargetId}
              initialParts={result.parts}
              totalParts={result.totalParts ?? 0}
            />
          )}
        </div>
      )}
    </div>
  );
}
