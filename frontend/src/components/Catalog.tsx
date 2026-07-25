import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { PartDetailViewModel } from '../types';
import PartDetailModal from './PartDetailModal';
import { createPartDetailViewModel, createPartDetailViewModelFromResponse } from '../utils/partDetail';
import { getPartDetail } from '../api/client';

interface ModelInfo {
  model: string;
  yearFrom: number;
  yearTo: number;
  variants: number;
}

interface VehicleVariant {
  linkageTargetId: number;
  description: string;
  fuelType: string;
  capacityCC: number;
  horsePower: number;
  yearFrom: number;
  yearTo: number;
}

interface AssemblyGroup {
  groupId: number;
  groupName: string;
  partCount: number;
}

interface CatalogPart {
  legacyArticleId: number;
  articleNumber: string;
  description: string;
  brandName: string;
  category: string;
  assemblyGroupId: number;
  oemNumbers?: string[];
}

const groupIcons: Record<string, string> = {
  'Engine Oil & Filters': 'Oil and filters',
  'Air Intake & Filters': 'Air and intake',
  'Ignition System': 'Ignition',
  'Cooling System': 'Cooling',
  'Fuel System': 'Fuel',
  'Exhaust System': 'Exhaust',
  'Timing / Drive Belt': 'Timing',
  'Engine Mounts': 'Engine mounts',
  'Sensors': 'Sensors',
  'Front Brake System': 'Front brakes',
  'Rear Brake System': 'Rear brakes',
  'Brake Hydraulics': 'Brake hydraulics',
  'Front Suspension': 'Front suspension',
  'Rear Suspension': 'Rear suspension',
  'Steering': 'Steering',
  'Headlights': 'Headlights',
  'Rear Lights': 'Rear lights',
  'Body Panels': 'Body panels',
  'Mirrors & Glass': 'Mirrors and glass',
  'Wipers': 'Wipers',
  'Clutch': 'Clutch',
  'Drive Shafts': 'Drive shafts',
  'Transmission Mounts': 'Transmission mounts',
  'HVAC / Climate': 'HVAC and climate',
  'Cabin Filter & Blower': 'Cabin filter and blower',
  'Electrical': 'Electrical',
  'ABS / Wheel Speed': 'ABS and wheel speed',
};

const BASE = '/api';

async function api<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

export default function Catalog() {
  const [searchParams] = useSearchParams();
  const [make, setMake] = useState<string>('');
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [selectedModel, setSelectedModel] = useState<string>('');
  const [variants, setVariants] = useState<VehicleVariant[]>([]);
  const [selectedVehicle, setSelectedVehicle] = useState<VehicleVariant | null>(null);
  const [groups, setGroups] = useState<AssemblyGroup[]>([]);
  const [selectedGroup, setSelectedGroup] = useState<AssemblyGroup | null>(null);
  const [parts, setParts] = useState<CatalogPart[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selectedDetail, setSelectedDetail] = useState<PartDetailViewModel | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');

  const requestedMake = searchParams.get('make') ?? '';
  const requestedModel = searchParams.get('model') ?? '';
  const requestedVehicleId = Number(searchParams.get('vehicleId') || '0');
  const sourceQuery = searchParams.get('sourceQuery') ?? '';
  const sourceType = searchParams.get('sourceType') ?? '';

  useEffect(() => {
    if (requestedMake && requestedMake !== make) setMake(requestedMake);
  }, [requestedMake, make]);

  useEffect(() => {
    if (!make) {
      setModels([]);
      return;
    }
    setLoading(true);
    api<{ models: ModelInfo[] }>(`/catalog/models?make=${make}`)
      .then((d) => {
        setModels(d.models || []);
        setSelectedModel('');
        setVariants([]);
        setSelectedVehicle(null);
        setGroups([]);
        setParts([]);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [make]);

  useEffect(() => {
    if (!requestedModel || models.length === 0 || selectedModel === requestedModel) return;
    if (models.some((model) => model.model === requestedModel)) setSelectedModel(requestedModel);
  }, [requestedModel, models, selectedModel]);

  useEffect(() => {
    if (!make || !selectedModel) {
      setVariants([]);
      return;
    }
    setLoading(true);
    api<{ vehicles: VehicleVariant[] }>(`/catalog/vehicles?make=${make}&model=${encodeURIComponent(selectedModel)}`)
      .then((d) => {
        setVariants(d.vehicles || []);
        setSelectedVehicle(null);
        setGroups([]);
        setParts([]);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [make, selectedModel]);

  useEffect(() => {
    if (!requestedVehicleId || variants.length === 0) return;
    if (selectedVehicle?.linkageTargetId === requestedVehicleId) return;
    const matched = variants.find((variant) => variant.linkageTargetId === requestedVehicleId);
    if (matched) setSelectedVehicle(matched);
  }, [requestedVehicleId, variants, selectedVehicle]);

  useEffect(() => {
    if (!selectedVehicle) {
      setGroups([]);
      return;
    }
    setLoading(true);
    api<{ groups: AssemblyGroup[] }>(`/catalog/groups?vehicleId=${selectedVehicle.linkageTargetId}`)
      .then((d) => {
        setGroups(d.groups || []);
        setSelectedGroup(null);
        setParts([]);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedVehicle]);

  const loadParts = useCallback((group: AssemblyGroup | null) => {
    if (!selectedVehicle) return;
    setLoading(true);
    const gid = group ? `&groupId=${group.groupId}` : '';
    api<{ parts: CatalogPart[] }>(`/catalog/parts?vehicleId=${selectedVehicle.linkageTargetId}${gid}`)
      .then((d) => setParts(d.parts || []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedVehicle]);

  useEffect(() => {
    if (selectedGroup) loadParts(selectedGroup);
  }, [selectedGroup, loadParts]);

  async function openPartDetail(part: CatalogPart) {
    const fallback = createPartDetailViewModel({
      legacyArticleId: part.legacyArticleId,
      articleNumber: part.articleNumber,
      description: part.description,
      brandName: part.brandName,
      category: part.category,
      oemNumbers: part.oemNumbers,
      confidence: part.oemNumbers && part.oemNumbers.length > 0 ? 0.9 : 0.84,
      confidenceReason: 'This part comes from the owned catalog browse flow for the selected vehicle and assembly group.',
      source: {
        kind: 'owned_catalog',
        label: 'Owned catalog browse',
        detail: 'The current part record was loaded from the existing catalog browsing endpoint for the selected vehicle.',
      },
      warnings: selectedGroup
        ? [`Assembly group context: ${selectedGroup.groupName}`]
        : ['Showing the part from the full vehicle parts browse, not a narrowed group.'],
    });

    setSelectedDetail(fallback);
    setDetailLoading(true);
    setDetailError('');

    try {
      const response = await getPartDetail(part.legacyArticleId, selectedVehicle?.linkageTargetId);
      setSelectedDetail(createPartDetailViewModelFromResponse(response));
    } catch (err) {
      setSelectedDetail(fallback);
      setDetailError(err instanceof Error ? err.message : 'Failed to load part detail.');
    } finally {
      setDetailLoading(false);
    }
  }

  return (
    <div className="space-y-6">
      {sourceQuery ? (
        <div data-testid="catalog-source-banner" className="rounded-2xl border border-sky-200 bg-sky-50 px-4 py-3 text-sm text-sky-900">
          Opened from {sourceType || 'search'} context: <span className="font-semibold">{sourceQuery}</span>
        </div>
      ) : null}

      {error ? (
        <div className="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
          <button onClick={() => setError('')} className="ml-3 text-red-400 hover:text-red-600">x</button>
        </div>
      ) : null}

      <section className="rounded-[28px] border border-white/10 bg-white/95 p-6 text-slate-900 shadow-2xl shadow-slate-950/20">
        <div className="grid gap-5 xl:grid-cols-[1.2fr,0.8fr]">
          <div>
            <div className="mb-3 flex flex-wrap gap-2">
              <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-slate-700">
                Real catalog browse
              </span>
              <span className="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-emerald-800">
                No fake picture-side filler
              </span>
            </div>
            <h2 className="text-2xl font-semibold tracking-tight text-slate-950">Browse the actual catalog structure</h2>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-600">
              This catalog view is intentionally grounded in real assembly groups and part records. If we do not have an exact picture-side catalog graphic, we do not invent one.
            </p>
          </div>

          <div className="rounded-3xl border border-slate-200 bg-slate-50 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">Browse setup</div>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <MakeButton value="HYUNDAI" current={make} onChange={setMake} />
              <MakeButton value="KIA" current={make} onChange={setMake} />
            </div>
            {make ? (
              <select
                value={selectedModel}
                onChange={(e) => setSelectedModel(e.target.value)}
                className="mt-4 w-full rounded-2xl border border-slate-300 bg-white px-4 py-3 text-sm font-medium text-slate-900 outline-none transition-colors focus:border-sky-500 focus:ring-2 focus:ring-sky-500/20"
              >
                <option value="">Select model</option>
                {models.map((m) => (
                  <option key={m.model} value={m.model}>
                    {m.model} ({m.yearFrom}-{m.yearTo}) - {m.variants} variants
                  </option>
                ))}
              </select>
            ) : null}
          </div>
        </div>
      </section>

      {selectedModel && !selectedVehicle && variants.length > 0 ? (
        <section className="rounded-[28px] border border-white/10 bg-white/95 p-6 text-slate-900 shadow-2xl shadow-slate-950/20">
          <div className="mb-4">
            <h3 className="text-xl font-semibold text-slate-950">{make} {selectedModel} variants</h3>
            <p className="mt-1 text-sm text-slate-600">Choose the exact catalog vehicle before loading assembly groups and part rows.</p>
          </div>
          <div className="grid gap-4 xl:grid-cols-2">
            {variants.map((vehicle) => (
              <button
                key={vehicle.linkageTargetId}
                onClick={() => setSelectedVehicle(vehicle)}
                className="overflow-hidden rounded-3xl border border-slate-200 bg-white text-left transition-colors hover:border-sky-300 hover:shadow-lg hover:shadow-sky-100"
              >
                <div className="p-4">
                  <div className="mb-3 flex flex-wrap gap-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">
                    <span className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1">
                      Vehicle variant
                    </span>
                    <span className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1">
                      {make} {selectedModel}
                    </span>
                  </div>
                    <div className="font-medium text-slate-950">{vehicle.description}</div>
                    <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-600">
                      <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">ID {vehicle.linkageTargetId}</span>
                      {vehicle.capacityCC ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{vehicle.capacityCC}cc</span> : null}
                      {vehicle.horsePower ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{vehicle.horsePower}HP</span> : null}
                      {vehicle.yearFrom ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{vehicle.yearFrom}-{vehicle.yearTo}</span> : null}
                    </div>
                    <div className="mt-4 text-sm font-medium text-sky-700">Use this catalog vehicle</div>
                </div>
              </button>
            ))}
          </div>
        </section>
      ) : null}

      {selectedVehicle ? (
        <section className="grid gap-6 xl:grid-cols-[0.82fr,1.18fr]">
          <aside className="space-y-6">
            <div className="overflow-hidden rounded-[28px] border border-white/10 bg-white/95 text-slate-900 shadow-2xl shadow-slate-950/20">
              <div className="p-5">
                <div className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">Selected vehicle</div>
                <div className="mt-2 text-lg font-semibold text-slate-950">{selectedVehicle.description}</div>
                <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-600">
                  <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">ID {selectedVehicle.linkageTargetId}</span>
                  {selectedVehicle.fuelType ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{selectedVehicle.fuelType}</span> : null}
                  {selectedVehicle.capacityCC ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{selectedVehicle.capacityCC}cc</span> : null}
                  {selectedVehicle.horsePower ? <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-1">{selectedVehicle.horsePower}HP</span> : null}
                </div>
              </div>
            </div>

            <div className="rounded-[28px] border border-white/10 bg-white/95 p-5 text-slate-900 shadow-2xl shadow-slate-950/20">
              <div className="mb-4 flex items-center justify-between gap-3">
                <div>
                  <h3 className="text-lg font-semibold text-slate-950">Assembly groups</h3>
                  <p className="mt-1 text-sm text-slate-600">Choose a real catalog group. No decorative picture-side placeholder is shown here.</p>
                </div>
                <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-medium text-slate-600">
                  {groups.length} groups
                </span>
              </div>

              <div className="space-y-2">
                <button
                  onClick={() => {
                    setSelectedGroup(null);
                    loadParts(null);
                  }}
                  data-testid="catalog-all-parts"
                  className={`w-full rounded-2xl border px-4 py-3 text-left text-sm transition-colors ${
                    !selectedGroup
                      ? 'border-slate-950 bg-slate-950 text-white'
                      : 'border-slate-200 bg-white text-slate-700 hover:border-sky-300 hover:bg-slate-50'
                  }`}
                >
                  <div className="font-medium">All parts</div>
                  <div className={`mt-1 text-xs ${!selectedGroup ? 'text-slate-300' : 'text-slate-500'}`}>Browse every loaded part record for this vehicle.</div>
                </button>

                {groups.map((group) => (
                  <button
                    key={group.groupId}
                    onClick={() => setSelectedGroup(group)}
                    className={`w-full rounded-2xl border px-4 py-3 text-left text-sm transition-colors ${
                      selectedGroup?.groupId === group.groupId
                        ? 'border-sky-300 bg-sky-50 text-sky-950'
                        : 'border-slate-200 bg-white text-slate-700 hover:border-sky-300 hover:bg-slate-50'
                    }`}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <div className="font-medium">{group.groupName}</div>
                        <div className="mt-1 text-xs text-slate-500">{groupIcons[group.groupName] || 'Catalog assembly group'}</div>
                      </div>
                      <span className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs text-slate-600">
                        {group.partCount}
                      </span>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          </aside>

          <div className="rounded-[28px] border border-white/10 bg-white/95 text-slate-900 shadow-2xl shadow-slate-950/20">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 px-5 py-4">
              <div>
                <h3 className="text-lg font-semibold text-slate-950">{selectedGroup ? selectedGroup.groupName : 'All parts'}</h3>
                <p className="mt-1 text-sm text-slate-600">
                  {selectedGroup
                    ? 'This table is scoped to the selected real assembly group.'
                    : 'This table is showing the full loaded part list for the selected vehicle.'}
                </p>
              </div>
              <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-medium text-slate-600">
                {parts.length} parts
              </span>
            </div>

            {parts.length === 0 && !loading ? (
              <div className="px-5 py-16 text-center text-slate-400">
                {selectedGroup ? 'No parts are loaded in this group.' : 'Choose All parts or a specific group to load the catalog rows.'}
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-slate-200 text-sm">
                  <thead className="bg-slate-50">
                    <tr>
                      <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">#</th>
                      <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Part number</th>
                      <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Description</th>
                      <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Brand</th>
                      <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">OEM refs</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {parts.map((part, idx) => (
                      <tr key={`${part.legacyArticleId}-${idx}`} className="hover:bg-slate-50">
                        <td className="px-5 py-3 text-xs text-slate-400">{idx + 1}</td>
                        <td className="px-5 py-3">
                          <button
                            onClick={() => openPartDetail(part)}
                            data-testid="catalog-part-article"
                            className="font-mono font-semibold text-sky-800 hover:text-sky-950 hover:underline"
                          >
                            {part.articleNumber}
                          </button>
                        </td>
                        <td className="px-5 py-3 text-slate-700">
                          <button
                            onClick={() => openPartDetail(part)}
                            data-testid="catalog-part-description"
                            className="text-left hover:text-sky-900 hover:underline"
                          >
                            {part.description}
                          </button>
                        </td>
                        <td className="px-5 py-3">
                          <span className="inline-flex rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs font-medium text-slate-700">
                            {part.brandName || 'Unknown brand'}
                          </span>
                        </td>
                        <td className="px-5 py-3">
                          {part.oemNumbers && part.oemNumbers.length > 0 ? (
                            <div className="flex flex-wrap gap-1">
                              {part.oemNumbers.map((oem) => (
                                <span key={oem} className="inline-flex rounded-full border border-amber-200 bg-amber-50 px-2 py-1 text-xs font-mono text-amber-900">
                                  {oem}
                                </span>
                              ))}
                            </div>
                          ) : (
                            <span className="text-slate-300">-</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </section>
      ) : null}

      <PartDetailModal
        detail={selectedDetail}
        loading={detailLoading}
        error={detailError}
        onClose={() => {
          setSelectedDetail(null);
          setDetailLoading(false);
          setDetailError('');
        }}
      />

      {loading ? (
        <div className="fixed bottom-4 right-4 rounded-2xl bg-slate-950 px-4 py-3 text-sm font-medium text-white shadow-lg">
          Loading...
        </div>
      ) : null}
    </div>
  );
}

function MakeButton({ value, current, onChange }: { value: string; current: string; onChange: (v: string) => void }) {
  const active = current === value;
  return (
    <button
      onClick={() => onChange(value)}
      className={`rounded-2xl border px-4 py-3 text-sm font-medium transition-colors ${
        active
          ? 'border-slate-950 bg-slate-950 text-white'
          : 'border-slate-200 bg-white text-slate-700 hover:border-sky-300 hover:bg-slate-50'
      }`}
    >
      {value}
    </button>
  );
}
