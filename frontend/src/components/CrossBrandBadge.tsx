import type { CrossBrandHit } from '../types';

interface Props {
  hit: CrossBrandHit;
}

export default function CrossBrandBadge({ hit }: Props) {
  return (
    <span className="inline-flex items-center gap-1.5 bg-purple-50 text-purple-700 border border-purple-200 rounded-full px-3 py-1 text-sm font-medium">
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
      </svg>
      Also fits {hit.siblingMake} {hit.siblingModel}
      {hit.sharedParts !== undefined && (
        <span className="text-purple-500 text-xs ml-1">({hit.sharedParts} shared)</span>
      )}
    </span>
  );
}
