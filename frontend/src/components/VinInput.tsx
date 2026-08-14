import { useState } from 'react';
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
  const [vin, setVin] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<VINDecodeResponse | null>(null);
  const [confirmedVehicle, setConfirmedVehicle] = useState<Vehicle | null>(null);
  const [categoryParts, setCategoryParts] = useState<Part[] | null>(null);
  const [categoryTotal, setCategoryTotal] = useState(0);
  const [selectedCategory, setSelectedCategory] = useState('');

  const valid = VIN_RE.test(vin);

  // The active vehicle is either user-confirmed or auto-selected
  const activeVehicle = confirmedVehicle || result?.vehicle || null;

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

  return (
    <div className="max-w-5xl mx-auto">
      <form onSubmit={handleSubmit} className="flex gap-3 items-end mb-8">
        <div className="flex-1">
          <label className="block text-sm font-medium text-gray-700 mb-1">
            VIN (17 characters)
          </label>
          <input
            type="text"
            maxLength={17}
            value={vin}
            onChange={(e) => setVin(e.target.value.toUpperCase())}
            placeholder="e.g. 5NPE34AF7FH123456"
            className="w-full rounded-lg border border-gray-300 px-4 py-2.5 text-lg font-mono tracking-widest focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
          />
        </div>
        <button
          type="submit"
          disabled={!valid || loading}
          className="px-6 py-2.5 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          {loading ? 'Decoding…' : 'Decode'}
        </button>
      </form>

      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-6">
          {error}
        </div>
      )}

      {result && (
        <div className="space-y-6">
          {result.nhtsaRaw && <VehicleCard nhtsa={result.nhtsaRaw} vehicle={result.vehicle} />}

          {/* Vehicle Configurator — shown when multiple variants match */}
          {result.needsConfirmation && result.allVariants && (
            <VehicleConfigurator
              variants={result.allVariants}
              selected={confirmedVehicle ?? undefined}
              onSelect={handleVariantSelect}
            />
          )}

          {/* Engine Info */}
          {result.engines && result.engines.length > 0 && (
            <div className="bg-gray-900 rounded-lg p-4">
              <h3 className="text-sm font-semibold text-gray-400 mb-2">Resolved Engine(s)</h3>
              <div className="flex flex-wrap gap-3">
                {result.engines.map((eng) => (
                  <div
                    key={eng.motorCode}
                    className="bg-gray-800 rounded px-3 py-2 text-sm"
                  >
                    <span className="text-blue-400 font-bold">{eng.motorCode}</span>
                    <span className="text-gray-400 ml-2">
                      {eng.cc}cc {eng.fuelType} {eng.cylinders ? `${eng.cylinders}cyl` : ''}
                      {eng.powerHP ? ` ${eng.powerHP}HP` : ''}
                    </span>
                  </div>
                ))}
              </div>
            </div>
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
          {activeVehicle && result.nhtsaRaw && (
            <PlatformBadge
              linkageTargetId={activeVehicle.linkageTargetId}
              make={result.nhtsaRaw.make}
              model={result.nhtsaRaw.model}
            />
          )}

          {/* Hierarchical Category Tree */}
          {activeVehicle && (
            <div>
              <h3 className="text-lg font-semibold mb-3">Select Part Category</h3>
              <CategoryPicker
                linkageTargetId={activeVehicle.linkageTargetId}
                onPartsLoaded={handlePartsLoaded}
              />
            </div>
          )}

          {/* Parts from category selection */}
          {categoryParts && activeVehicle && (
            <div>
              <h3 className="text-lg font-semibold mb-2">
                {selectedCategory}{' '}
                <span className="text-gray-500 text-sm font-normal">
                  ({categoryTotal} parts)
                </span>
              </h3>
              <PartsTable
                linkageTargetId={activeVehicle.linkageTargetId}
                initialParts={categoryParts}
                totalParts={categoryTotal}
              />
            </div>
          )}

          {/* Initial parts (before category selection) */}
          {!categoryParts && activeVehicle && result.parts && (
            <PartsTable
              linkageTargetId={activeVehicle.linkageTargetId}
              initialParts={result.parts}
              totalParts={result.totalParts ?? 0}
            />
          )}
        </div>
      )}
    </div>
  );
}
