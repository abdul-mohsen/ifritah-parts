import { useEffect } from 'react';
import type { PartDetailViewModel } from '../types';

interface Props {
  detail: PartDetailViewModel | null;
  loading?: boolean;
  error?: string;
  onClose: () => void;
}

const confidenceColors: Record<PartDetailViewModel['confidenceBand'], string> = {
  high: 'bg-green-50 text-green-800 border-green-200',
  medium: 'bg-amber-50 text-amber-800 border-amber-200',
  low: 'bg-red-50 text-red-800 border-red-200',
};

const sourceColors: Record<PartDetailViewModel['source']['kind'], string> = {
  owned_catalog: 'bg-blue-50 text-blue-800 border-blue-200',
  smart_search: 'bg-indigo-50 text-indigo-800 border-indigo-200',
  oem_crossref: 'bg-purple-50 text-purple-800 border-purple-200',
  derived_inference: 'bg-slate-50 text-slate-800 border-slate-200',
  external_source: 'bg-emerald-50 text-emerald-800 border-emerald-200',
};

const placementColors: Record<NonNullable<PartDetailViewModel['placement']>['kind'], string> = {
  exact: 'bg-green-50 text-green-800 border-green-200',
  catalog_group: 'bg-blue-50 text-blue-800 border-blue-200',
  inferred: 'bg-amber-50 text-amber-800 border-amber-200',
  unavailable: 'bg-slate-50 text-slate-700 border-slate-200',
};

const replacementColors: Record<NonNullable<PartDetailViewModel['replacements']>[number]['candidateType'], string> = {
  shared_oem_reference: 'bg-green-50 text-green-800 border-green-200',
  catalog_compatible: 'bg-amber-50 text-amber-800 border-amber-200',
  aftermarket_alternative: 'bg-purple-50 text-purple-800 border-purple-200',
  substitution: 'bg-blue-50 text-blue-800 border-blue-200',
};

export default function PartDetailModal({ detail, loading = false, error = '', onClose }: Props) {
  useEffect(() => {
    if (!detail && !loading && !error) {
      return;
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onClose();
      }
    }

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [detail, loading, error, onClose]);

  if (!detail && !loading && !error) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/75 p-4 backdrop-blur-sm">
      <div className="w-full max-w-5xl overflow-hidden rounded-[28px] bg-white shadow-2xl">
        <div className="flex items-start justify-between border-b border-slate-200 px-6 py-5">
          <div>
            {detail ? (
              <>
                <div className="mb-2 flex flex-wrap items-center gap-2">
                  <span className="font-mono text-lg font-semibold text-slate-950">
                    {detail.articleNumber}
                  </span>
                  {detail.brandName && (
                    <span className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs font-medium text-slate-700">
                      {detail.brandName}
                    </span>
                  )}
                </div>
                <h2 className="text-xl font-semibold text-slate-950">{detail.description}</h2>
                <p className="mt-1 text-sm text-slate-500">
                  Evidence-first part detail with provenance, fitment context, replacement guidance, and only real media when available.
                </p>
              </>
            ) : (
              <>
                <h2 className="text-lg font-semibold text-slate-900">Part detail</h2>
                <p className="mt-1 text-sm text-slate-500">Loading the server-owned detail payload.</p>
              </>
            )}
          </div>
          <button
            onClick={onClose}
            className="rounded-full border border-slate-200 px-3 py-1.5 text-sm text-slate-500 hover:bg-slate-50 hover:text-slate-700"
          >
            Close
          </button>
        </div>

        <div className="space-y-6 px-6 py-5">
          {loading && (
            <div className="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800">
              Loading part detail from the backend...
            </div>
          )}

          {error && (
            <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800">
              {error}
            </div>
          )}

          {!detail ? null : (
            <>
          <div className="flex flex-wrap gap-2">
            <span className={`rounded-full border px-3 py-1 text-xs font-semibold ${sourceColors[detail.source.kind]}`}>
              Source: {detail.source.label}
            </span>
            <span className={`rounded-full border px-3 py-1 text-xs font-semibold ${confidenceColors[detail.confidenceBand]}`}>
              Confidence: {Math.round(detail.confidence * 100)}%
            </span>
            {detail.fitmentDriver && (
              <span className="rounded-full border border-gray-200 bg-gray-50 px-3 py-1 text-xs font-medium text-gray-700">
                Fitment: {detail.fitmentDriver}
              </span>
            )}
            {detail.category && (
              <span className="rounded-full border border-gray-200 bg-gray-50 px-3 py-1 text-xs font-medium text-gray-700">
                Category: {detail.category}
              </span>
            )}
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <section className="rounded-xl border border-gray-200 bg-slate-50 p-4">
              <h3 className="mb-2 text-sm font-semibold text-slate-900">Provenance</h3>
              <p className="text-sm text-slate-700">{detail.source.detail}</p>
              <p className="mt-3 text-sm text-slate-700">{detail.confidenceReason}</p>
            </section>

            <section className="rounded-xl border border-gray-200 bg-slate-50 p-4">
              <h3 className="mb-2 text-sm font-semibold text-slate-900">Quality status</h3>
              <div className="space-y-2 text-sm text-slate-700">
                <div className="flex items-center justify-between">
                  <span>Evidence complete</span>
                  <span className={detail.quality.provenanceComplete ? 'text-green-700' : 'text-red-700'}>
                    {detail.quality.provenanceComplete ? 'Yes' : 'No'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>OEM references loaded</span>
                  <span className={detail.quality.hasOEMNumbers ? 'text-green-700' : 'text-amber-700'}>
                    {detail.quality.hasOEMNumbers ? 'Yes' : 'No'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>Technical specs loaded</span>
                  <span className={detail.quality.hasCriteria ? 'text-green-700' : 'text-amber-700'}>
                    {detail.quality.hasCriteria ? 'Yes' : 'No'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>Fitment evidence loaded</span>
                  <span className={detail.quality.hasFitmentEvidence ? 'text-green-700' : 'text-amber-700'}>
                    {detail.quality.hasFitmentEvidence ? 'Yes' : 'No'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>Placement guidance loaded</span>
                  <span className={detail.quality.hasPlacement ? 'text-green-700' : 'text-amber-700'}>
                    {detail.quality.hasPlacement ? 'Yes' : 'No'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>Replacement guidance loaded</span>
                  <span className={detail.quality.hasReplacementCandidates ? 'text-green-700' : 'text-amber-700'}>
                    {detail.quality.hasReplacementCandidates ? 'Yes' : 'No'}
                  </span>
                </div>
                {!detail.quality.provenanceComplete && detail.quality.provenanceGaps && detail.quality.provenanceGaps.length > 0 ? (
                  <div className="rounded-lg border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800">
                    Missing evidence: {detail.quality.provenanceGaps.join(', ')}.
                  </div>
                ) : null}
              </div>
            </section>
          </div>

          <section>
            <h3 className="mb-2 text-sm font-semibold text-slate-900">Core identity</h3>
            <div className="grid gap-3 rounded-xl border border-gray-200 p-4 sm:grid-cols-2">
              <Field label="Legacy article ID" value={String(detail.legacyArticleId)} mono />
              <Field label="Article number" value={detail.articleNumber} mono />
              <Field label="Brand" value={detail.brandName || 'Unknown'} />
              <Field label="Category" value={detail.category || 'Not classified in this view'} />
            </div>
          </section>

          <section>
            <h3 className="mb-2 text-sm font-semibold text-slate-900">Placement and diagram guidance</h3>
            {detail.placement ? (
              <div className="space-y-3 rounded-xl border border-gray-200 p-4">
                <div className="flex flex-wrap gap-2">
                  <span className={`rounded-full border px-3 py-1 text-xs font-semibold ${placementColors[detail.placement.kind]}`}>
                    Placement: {detail.placement.kind.replace('_', ' ')}
                  </span>
                  <span className="rounded-full border border-gray-200 bg-gray-50 px-3 py-1 text-xs font-medium text-gray-700">
                    Type: {detail.placement.placementType.replace('_', ' ')}
                  </span>
                  <span className="rounded-full border border-gray-200 bg-gray-50 px-3 py-1 text-xs font-medium text-gray-700">
                    Placement confidence: {Math.round(detail.placement.confidence * 100)}%
                  </span>
                </div>

                <div className={`grid gap-4 ${detail.placement.imageUrl ? 'md:grid-cols-[1.3fr,0.9fr]' : ''}`}>
                  <div className="space-y-2">
                    <div className="text-sm font-semibold text-slate-900">{detail.placement.title}</div>
                    <p className="text-sm text-slate-700">{detail.placement.summary}</p>
                    {detail.placement.locationArea ? (
                      <p className="text-sm text-slate-600">
                        <span className="font-medium text-slate-800">Area:</span> {detail.placement.locationArea}
                      </p>
                    ) : null}
                    <div className="rounded-lg border border-gray-100 bg-slate-50 p-3 text-sm text-slate-700">
                      <div className="font-medium text-slate-900">{detail.placement.source.label}</div>
                      <div className="mt-1">{detail.placement.source.detail}</div>
                    </div>
                  </div>

                  {detail.placement.imageUrl ? (
                    <div className="overflow-hidden rounded-xl border border-gray-200 bg-slate-50">
                      <img
                        src={detail.placement.imageUrl}
                        alt={detail.placement.title}
                        className="h-full w-full object-cover"
                      />
                    </div>
                  ) : null}
                </div>

                {detail.placement.hints && detail.placement.hints.length > 0 ? (
                  <div>
                    <div className="mb-2 text-sm font-semibold text-slate-900">Guidance</div>
                    <ul className="space-y-1 text-sm text-slate-700">
                      {detail.placement.hints.map((hint) => (
                        <li key={hint}>- {hint}</li>
                      ))}
                    </ul>
                  </div>
                ) : null}

                {detail.placement.warnings && detail.placement.warnings.length > 0 ? (
                  <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
                    {detail.placement.warnings.map((warning) => (
                      <div key={warning}>- {warning}</div>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : (
              <EmptyState text="No placement guidance is available in the current response." />
            )}
          </section>

          <section>
            <h3 className="mb-2 text-sm font-semibold text-slate-900">OEM references</h3>
            {detail.oemNumbers.length > 0 ? (
              <div className="flex flex-wrap gap-2 rounded-xl border border-gray-200 p-4">
                {detail.oemNumbers.map((oem) => (
                  <span
                    key={oem}
                    className="rounded-full border border-amber-200 bg-amber-50 px-3 py-1 text-xs font-mono text-amber-900"
                  >
                    {oem}
                  </span>
                ))}
              </div>
            ) : (
              <EmptyState text="No OEM references are available in the current response." />
            )}
          </section>

          <section>
            <h3 className="mb-2 text-sm font-semibold text-slate-900">Technical specs</h3>
            {detail.criteria && Object.keys(detail.criteria).length > 0 ? (
              <div className="grid gap-3 rounded-xl border border-gray-200 p-4 sm:grid-cols-2">
                {Object.entries(detail.criteria).map(([key, value]) => (
                  <Field key={key} label={key} value={value} />
                ))}
              </div>
            ) : (
              <div data-testid="missing-specification-guidance" className="space-y-3 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
                <div className="font-semibold">No sourced technical specifications are available for this part.</div>
                <p>
                  The current catalog does not provide dimensions, connector details, torque values, or installation attributes.
                  A category or description match is not evidence that those details match.
                </p>
                <div>
                  <div className="font-medium">Before fitting, verify:</div>
                  <ul className="mt-1 space-y-1 text-amber-800">
                    <li>- the OEM reference against the confirmed vehicle variant</li>
                    <li>- physical dimensions, mounting points, connectors, threads, or seals as applicable</li>
                    <li>- manufacturer service information for installation and torque requirements</li>
                  </ul>
                </div>
              </div>
            )}
          </section>

          <section>
            <h3 className="mb-2 text-sm font-semibold text-slate-900">Replacement / equivalent guidance</h3>
            {detail.replacements && detail.replacements.length > 0 ? (
              <div className="space-y-3 rounded-xl border border-gray-200 p-4">
                {detail.replacements.map((candidate) => (
                  <div key={`${candidate.legacyArticleId}-${candidate.articleNumber}-${candidate.candidateType}`} className="rounded-xl border border-gray-100 p-3">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm font-semibold text-slate-900">{candidate.articleNumber}</span>
                          <span className={`rounded-full border px-2.5 py-1 text-[11px] font-semibold ${replacementColors[candidate.candidateType]}`}>
                            {candidate.candidateType.replaceAll('_', ' ')}
                          </span>
                          <span className="rounded-full border border-gray-200 bg-gray-50 px-2.5 py-1 text-[11px] font-medium text-gray-700">
                            {Math.round(candidate.confidence * 100)}%
                          </span>
                        </div>
                        <div className="mt-1 text-sm text-slate-800">{candidate.description}</div>
                        <div className="mt-1 text-xs text-slate-500">
                          {candidate.brandName || 'Unknown brand'}
                          {candidate.category ? ` • ${candidate.category}` : ''}
                          {candidate.oemReference ? ` • shared OEM ${candidate.oemReference}` : ''}
                        </div>
                      </div>
                      <div className="max-w-sm rounded-lg border border-gray-100 bg-slate-50 px-3 py-2 text-xs text-slate-600">
                        <div className="font-medium text-slate-900">{candidate.source.label}</div>
                        <div className="mt-1">{candidate.source.detail}</div>
                      </div>
                    </div>

                    <p className="mt-3 text-sm text-slate-700">{candidate.explanation}</p>

                    {candidate.warnings && candidate.warnings.length > 0 ? (
                      <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
                        {candidate.warnings.map((warning) => (
                          <div key={warning}>- {warning}</div>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            ) : (
              <EmptyState text="No conservative replacement guidance is available in the current response." />
            )}
          </section>

          <section>
            <h3 className="mb-2 text-sm font-semibold text-slate-900">Compatible alternatives</h3>
            {detail.alternatives && detail.alternatives.length > 0 ? (
              <div className="space-y-2 rounded-xl border border-gray-200 p-4">
                {detail.alternatives.map((alternative) => (
                  <div key={alternative.legacyArticleId} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-gray-100 px-3 py-2">
                    <div>
                      <div className="font-mono text-sm font-semibold text-slate-900">{alternative.articleNumber}</div>
                      <div className="text-sm text-slate-700">{alternative.description}</div>
                    </div>
                    <div className="text-right text-xs text-slate-500">
                      <div>{alternative.brandName || 'Unknown brand'}</div>
                      {alternative.sharedVehicles ? <div>{alternative.sharedVehicles} shared vehicles</div> : null}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyState text="No compatible alternatives are available in the current response." />
            )}
          </section>

          <section>
            <h3 className="mb-2 text-sm font-semibold text-slate-900">Fitment context</h3>
            {detail.fitVehicles && detail.fitVehicles.length > 0 ? (
              <div className="space-y-2 rounded-xl border border-gray-200 p-4">
                {detail.fitVehicles.map((vehicle) => (
                  <div key={vehicle.linkageTargetId} className="rounded-lg border border-gray-100 px-3 py-2">
                    <div className="font-medium text-slate-900">
                      {vehicle.make} {vehicle.model} {vehicle.description || ''}
                    </div>
                    <div className="mt-1 flex flex-wrap gap-2 text-xs text-slate-500">
                      <span>ID {vehicle.linkageTargetId}</span>
                      {vehicle.fuelType ? <span>{vehicle.fuelType}</span> : null}
                      {vehicle.capacityCC ? <span>{vehicle.capacityCC}cc</span> : null}
                      {vehicle.horsePower ? <span>{vehicle.horsePower}HP</span> : null}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyState text="No expanded fitment list is available in the current response." />
            )}
          </section>

          {detail.warnings && detail.warnings.length > 0 && (
            <section className="rounded-xl border border-amber-200 bg-amber-50 p-4">
              <h3 className="mb-2 text-sm font-semibold text-amber-900">Cautions</h3>
              <ul className="space-y-1 text-sm text-amber-800">
                {detail.warnings.map((warning) => (
                  <li key={warning}>- {warning}</li>
                ))}
              </ul>
            </section>
          )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function Field({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <div className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</div>
      <div className={`mt-1 text-sm text-slate-900 ${mono ? 'font-mono' : ''}`}>{value}</div>
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="rounded-xl border border-dashed border-gray-300 bg-gray-50 p-4 text-sm text-gray-500">
      {text}
    </div>
  );
}
