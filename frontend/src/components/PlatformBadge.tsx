import { useEffect, useState } from 'react';
import { getPlatformSiblings } from '../api/client';
import type { PlatformInfo } from '../types';

interface Props {
  linkageTargetId: number;
  make: string;
  model: string;
}

export default function PlatformBadge({ linkageTargetId, make, model }: Props) {
  const [siblings, setSiblings] = useState<PlatformInfo[]>([]);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    if (!linkageTargetId || !make || !model) return;
    getPlatformSiblings(linkageTargetId, make, model)
      .then((res) => setSiblings(res.siblings || []))
      .catch(() => setSiblings([]));
  }, [linkageTargetId, make, model]);

  if (siblings.length === 0) return null;

  return (
    <div className="bg-purple-900/20 border border-purple-700/40 rounded-lg p-3">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 w-full text-left"
      >
        <span className="text-purple-400 text-sm">🔗</span>
        <span className="text-purple-300 text-sm font-medium">
          Platform shared with{' '}
          {siblings.map((s) => `${s.siblingMake} ${s.siblingModel}`).join(', ')}
        </span>
        {siblings[0]?.platform && (
          <span className="px-1.5 py-0.5 bg-purple-800/50 text-purple-300 rounded text-xs font-mono">
            {siblings[0].platform}
          </span>
        )}
        <svg
          className={`w-4 h-4 text-purple-400 ml-auto transition-transform ${expanded ? 'rotate-180' : ''}`}
          fill="none" stroke="currentColor" viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {expanded && (
        <div className="mt-3 space-y-2">
          {siblings.map((sib) => (
            <div
              key={`${sib.siblingMake}-${sib.siblingModel}`}
              className="flex items-center justify-between px-3 py-2 bg-gray-800/50 rounded"
            >
              <div>
                <span className="text-white text-sm font-medium">
                  {sib.siblingMake} {sib.siblingModel}
                </span>
                <span className="text-gray-500 text-xs ml-2">({sib.platform})</span>
              </div>
              {sib.sharedParts > 0 && (
                <span className="text-green-400 text-xs font-medium">
                  {sib.sharedParts} shared parts
                </span>
              )}
            </div>
          ))}
          <p className="text-gray-500 text-xs">
            Parts from the sibling model may also fit your vehicle — same platform architecture.
          </p>
        </div>
      )}
    </div>
  );
}
