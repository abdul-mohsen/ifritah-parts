import type { Recall } from '../types';

interface Props {
  recalls: Recall[];
}

export default function RecallBanner({ recalls }: Props) {
  return (
    <div data-testid="recall-banner" className="bg-amber-50 border border-amber-300 rounded-lg p-4">
      <div className="flex items-center gap-2 mb-2">
        <svg className="w-5 h-5 text-amber-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        <h4 className="font-semibold text-amber-800">
          {recalls.length} NHTSA Safety Recall Notice{recalls.length !== 1 ? 's' : ''}
        </h4>
      </div>
      <ul className="space-y-3">
        {recalls.map((r) => (
          <li key={r.nhtsaCampaignNumber} className="text-sm">
            <div className="font-medium text-amber-900">
              {r.nhtsaCampaignNumber} — {r.component}
            </div>
            <p className="text-amber-700 mt-0.5">{r.summary}</p>
            {r.remedy && (
              <p className="text-amber-600 text-xs mt-1">
                <span className="font-medium">Remedy:</span> {r.remedy}
              </p>
            )}
            <p className="mt-1 text-xs text-amber-700">
              <span className="font-medium">Source:</span>{' '}
              <a href={r.sourceUrl} target="_blank" rel="noreferrer" className="underline hover:text-amber-900">
                {r.sourceLabel}
              </a>
            </p>
            {r.warning && <p className="mt-1 text-xs text-amber-800">- {r.warning}</p>}
          </li>
        ))}
      </ul>
    </div>
  );
}
