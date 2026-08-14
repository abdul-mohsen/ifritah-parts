import { useState, useEffect, useCallback } from 'react';

/* ── Types ──────────────────────────────────────────────────────────── */

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

/* ── Group icons (emoji stand-ins for TecDoc category icons) ──────── */
const groupIcons: Record<string, string> = {
  'Engine Oil & Filters': '🛢️',
  'Air Intake & Filters': '🌬️',
  'Ignition System': '⚡',
  'Cooling System': '❄️',
  'Fuel System': '⛽',
  'Exhaust System': '💨',
  'Timing / Drive Belt': '⏱️',
  'Engine Mounts': '🔩',
  'Sensors': '📡',
  'Front Brake System': '🛑',
  'Rear Brake System': '🛑',
  'Brake Hydraulics': '🔧',
  'Front Suspension': '🔗',
  'Rear Suspension': '🔗',
  'Steering': '🎯',
  'Headlights': '💡',
  'Rear Lights': '🔴',
  'Body Panels': '🚗',
  'Mirrors & Glass': '🪞',
  'Wipers': '🌧️',
  'Clutch': '⚙️',
  'Drive Shafts': '🔄',
  'Transmission Mounts': '🔩',
  'HVAC / Climate': '🌡️',
  'Cabin Filter & Blower': '🌀',
  'Electrical': '🔌',
  'ABS / Wheel Speed': '📏',
};

const BASE = '/api';

/* ── API helpers ────────────────────────────────────────────────────── */
async function api<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

/* ── Main Component ─────────────────────────────────────────────────── */
export default function Catalog() {
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

  // Load models when make changes
  useEffect(() => {
    if (!make) { setModels([]); return; }
    setLoading(true);
    api<{ models: ModelInfo[] }>(`/catalog/models?make=${make}`)
      .then(d => { setModels(d.models || []); setSelectedModel(''); setVariants([]); setSelectedVehicle(null); setGroups([]); setParts([]); })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [make]);

  // Load variants when model changes
  useEffect(() => {
    if (!make || !selectedModel) { setVariants([]); return; }
    setLoading(true);
    api<{ vehicles: VehicleVariant[] }>(`/catalog/vehicles?make=${make}&model=${encodeURIComponent(selectedModel)}`)
      .then(d => { setVariants(d.vehicles || []); setSelectedVehicle(null); setGroups([]); setParts([]); })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [make, selectedModel]);

  // Load assembly groups when vehicle changes
  useEffect(() => {
    if (!selectedVehicle) { setGroups([]); return; }
    setLoading(true);
    api<{ groups: AssemblyGroup[] }>(`/catalog/groups?vehicleId=${selectedVehicle.linkageTargetId}`)
      .then(d => { setGroups(d.groups || []); setSelectedGroup(null); setParts([]); })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedVehicle]);

  // Load parts when group changes
  const loadParts = useCallback((group: AssemblyGroup | null) => {
    if (!selectedVehicle) return;
    setLoading(true);
    const gid = group ? `&groupId=${group.groupId}` : '';
    api<{ parts: CatalogPart[] }>(`/catalog/parts?vehicleId=${selectedVehicle.linkageTargetId}${gid}`)
      .then(d => setParts(d.parts || []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedVehicle]);

  useEffect(() => {
    if (selectedGroup) loadParts(selectedGroup);
  }, [selectedGroup, loadParts]);

  return (
    <div className="max-w-7xl mx-auto">
      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-4 text-sm">
          {error}
          <button onClick={() => setError('')} className="ml-3 text-red-400 hover:text-red-600">✕</button>
        </div>
      )}

      {/* ── Breadcrumb / Navigation Bar ──────────────────────────────── */}
      <div className="bg-white border border-gray-200 rounded-lg px-4 py-3 mb-6 flex items-center gap-2 text-sm flex-wrap">
        <span className="text-gray-400">Catalog</span>
        <Chevron />
        <MakeSelector make={make} onChange={setMake} />
        {make && (
          <>
            <Chevron />
            <select
              value={selectedModel}
              onChange={e => setSelectedModel(e.target.value)}
              className="border border-gray-200 rounded px-2 py-1 text-sm font-medium bg-white"
            >
              <option value="">Select Model</option>
              {models.map(m => (
                <option key={m.model} value={m.model}>
                  {m.model} ({m.yearFrom}–{m.yearTo}) · {m.variants} variants
                </option>
              ))}
            </select>
          </>
        )}
        {selectedVehicle && (
          <>
            <Chevron />
            <span className="text-gray-700 font-medium truncate max-w-xs" title={selectedVehicle.description}>
              {selectedVehicle.description}
            </span>
          </>
        )}
        {selectedGroup && (
          <>
            <Chevron />
            <span className="text-blue-700 font-medium">{selectedGroup.groupName}</span>
          </>
        )}
      </div>

      {/* ── Vehicle Variants ─────────────────────────────────────────── */}
      {selectedModel && !selectedVehicle && variants.length > 0 && (
        <div>
          <h2 className="text-lg font-semibold text-gray-900 mb-3">
            Select Vehicle — {make} {selectedModel}
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {variants.map(v => (
              <button
                key={v.linkageTargetId}
                onClick={() => setSelectedVehicle(v)}
                className="text-left bg-white border border-gray-200 rounded-lg px-4 py-3 hover:border-blue-400 hover:shadow-sm transition-all group"
              >
                <div className="font-medium text-gray-900 group-hover:text-blue-700">
                  {v.description}
                </div>
                <div className="flex items-center gap-3 mt-1 text-xs text-gray-500">
                  <span className="bg-gray-100 px-2 py-0.5 rounded">{v.fuelType}</span>
                  <span>{v.capacityCC}cc</span>
                  <span>{v.horsePower}HP</span>
                  <span>{v.yearFrom}–{v.yearTo}</span>
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* ── Catalog: Sidebar + Parts ─────────────────────────────────── */}
      {selectedVehicle && (
        <div className="flex gap-6">
          {/* Sidebar — Assembly Groups */}
          <aside className="w-64 shrink-0">
            <div className="bg-white border border-gray-200 rounded-lg overflow-hidden sticky top-4">
              <div className="bg-gray-50 px-4 py-2 border-b border-gray-200">
                <h3 className="text-sm font-semibold text-gray-700">Assembly Groups</h3>
                <p className="text-xs text-gray-400">{groups.length} groups</p>
              </div>
              <div className="max-h-[70vh] overflow-y-auto">
                {/* All Parts option */}
                <button
                  onClick={() => { setSelectedGroup(null); loadParts(null); }}
                  className={`w-full text-left px-4 py-2.5 text-sm border-b border-gray-100 flex items-center justify-between hover:bg-blue-50 transition-colors ${
                    !selectedGroup ? 'bg-blue-50 text-blue-700 font-medium' : 'text-gray-700'
                  }`}
                >
                  <span>📋 All Parts</span>
                </button>
                {groups.map(g => (
                  <button
                    key={g.groupId}
                    onClick={() => setSelectedGroup(g)}
                    className={`w-full text-left px-4 py-2.5 text-sm border-b border-gray-100 flex items-center justify-between hover:bg-blue-50 transition-colors ${
                      selectedGroup?.groupId === g.groupId ? 'bg-blue-50 text-blue-700 font-medium' : 'text-gray-700'
                    }`}
                  >
                    <span>
                      {groupIcons[g.groupName] || '🔧'} {g.groupName}
                    </span>
                    <span className="text-xs bg-gray-100 px-1.5 py-0.5 rounded text-gray-500">
                      {g.partCount}
                    </span>
                  </button>
                ))}
              </div>
            </div>
          </aside>

          {/* Main content — Parts Table */}
          <div className="flex-1 min-w-0">
            <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
              <div className="bg-gray-50 px-4 py-2 border-b border-gray-200 flex items-center justify-between">
                <h3 className="text-sm font-semibold text-gray-700">
                  {selectedGroup ? selectedGroup.groupName : 'All Parts'}
                </h3>
                <span className="text-xs text-gray-400">{parts.length} parts</span>
              </div>

              {parts.length === 0 && !loading ? (
                <div className="px-4 py-12 text-center text-gray-400">
                  {selectedGroup ? 'No parts in this group' : 'Select an assembly group to view parts'}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-gray-200 text-sm">
                    <thead className="bg-gray-50">
                      <tr>
                        <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">#</th>
                        <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Part Number</th>
                        <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Description</th>
                        <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Brand</th>
                        <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">OEM Number(s)</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-100">
                      {parts.map((p, idx) => (
                        <tr key={`${p.legacyArticleId}-${idx}`} className="hover:bg-blue-50/50 transition-colors">
                          <td className="px-4 py-2 text-gray-400 text-xs">{idx + 1}</td>
                          <td className="px-4 py-2">
                            <span className="font-mono font-semibold text-blue-700">{p.articleNumber}</span>
                          </td>
                          <td className="px-4 py-2 text-gray-700">{p.description}</td>
                          <td className="px-4 py-2">
                            <BrandBadge brand={p.brandName} />
                          </td>
                          <td className="px-4 py-2">
                            {p.oemNumbers && p.oemNumbers.length > 0 ? (
                              <div className="flex flex-wrap gap-1">
                                {p.oemNumbers.map(oem => (
                                  <span key={oem} className="inline-block font-mono text-xs bg-amber-50 text-amber-800 border border-amber-200 px-1.5 py-0.5 rounded">
                                    {oem}
                                  </span>
                                ))}
                              </div>
                            ) : (
                              <span className="text-gray-300">—</span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Loading overlay */}
      {loading && (
        <div className="fixed bottom-4 right-4 bg-blue-600 text-white px-4 py-2 rounded-lg shadow-lg text-sm animate-pulse">
          Loading…
        </div>
      )}
    </div>
  );
}

/* ── Subcomponents ──────────────────────────────────────────────────── */

function MakeSelector({ make, onChange }: { make: string; onChange: (v: string) => void }) {
  return (
    <div className="flex gap-1">
      {['HYUNDAI', 'KIA'].map(m => (
        <button
          key={m}
          onClick={() => onChange(m)}
          className={`px-3 py-1 rounded text-sm font-medium transition-colors ${
            make === m
              ? 'bg-blue-600 text-white'
              : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
          }`}
        >
          {m}
        </button>
      ))}
    </div>
  );
}

function BrandBadge({ brand }: { brand: string }) {
  const isOEM = brand === 'HYUNDAI/KIA' || brand === 'HYUNDAI' || brand === 'KIA';
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${
      isOEM
        ? 'bg-green-50 text-green-800 border border-green-200'
        : 'bg-gray-50 text-gray-700 border border-gray-200'
    }`}>
      {isOEM ? '🏭 ' : ''}{brand}
    </span>
  );
}

function Chevron() {
  return (
    <svg className="w-4 h-4 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
    </svg>
  );
}
