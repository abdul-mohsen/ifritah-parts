import { useEffect, useReducer, useState } from 'react';
import { getCategoryTree, getPartsForVehicle } from '../api/client';
import type { CategoryGroup, CategoryLeaf, Part } from '../types';

const ICONS: Record<string, string> = {
  engine: '⚙️',
  brake: '🛑',
  suspension: '🔧',
  body: '🚗',
  climate: '❄️',
  electrical: '⚡',
  transmission: '🔩',
  other: '📦',
};

interface Props {
  linkageTargetId: number;
  onPartsLoaded?: (parts: Part[], total: number, category: string) => void;
}

interface CategoryState {
  tree: CategoryGroup[];
  expandedGroup: string | null;
  selectedLeaf: string | null;
  loading: boolean;
}

type CategoryAction =
  | { type: 'loading' }
  | { type: 'loaded'; tree: CategoryGroup[] }
  | { type: 'failed' }
  | { type: 'toggle-group'; groupName: string }
  | { type: 'select-leaf'; leafName: string };

const initialCategoryState: CategoryState = {
  tree: [],
  expandedGroup: null,
  selectedLeaf: null,
  loading: false,
};

function categoryReducer(state: CategoryState, action: CategoryAction): CategoryState {
  switch (action.type) {
    case 'loading':
      return { ...state, expandedGroup: null, selectedLeaf: null, loading: true };
    case 'loaded':
      return { ...state, tree: action.tree, loading: false };
    case 'failed':
      return { ...state, tree: [], loading: false };
    case 'toggle-group':
      return {
        ...state,
        expandedGroup: state.expandedGroup === action.groupName ? null : action.groupName,
      };
    case 'select-leaf':
      return { ...state, selectedLeaf: action.leafName };
  }
}

export default function CategoryPicker({ linkageTargetId, onPartsLoaded }: Props) {
  const [state, dispatch] = useReducer(categoryReducer, initialCategoryState);
  const [filter, setFilter] = useState('');

  useEffect(() => {
    if (!linkageTargetId) return;
    let cancelled = false;
    dispatch({ type: 'loading' });
    getCategoryTree(linkageTargetId)
      .then((res) => {
        if (!cancelled) {
          dispatch({ type: 'loaded', tree: res.tree || [] });
        }
      })
      .catch(() => {
        if (!cancelled) {
          dispatch({ type: 'failed' });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [linkageTargetId]);

  const handleGroupClick = (groupName: string) => {
    dispatch({ type: 'toggle-group', groupName });
  };

  const handleLeafClick = async (leaf: CategoryLeaf) => {
    dispatch({ type: 'select-leaf', leafName: leaf.name });
    if (!onPartsLoaded) return;

    try {
      const res = await getPartsForVehicle(linkageTargetId, 1, 50, leaf.name, true);
      onPartsLoaded(res.parts, res.total, leaf.name);
    } catch {
      onPartsLoaded([], 0, leaf.name);
    }
  };

  // Filter tree
  const filteredTree = filter
    ? state.tree
        .map((group) => ({
          ...group,
          categories: group.categories.filter((c) =>
            c.name.toLowerCase().includes(filter.toLowerCase()),
          ),
        }))
        .filter((g) => g.categories.length > 0)
    : state.tree;

  if (state.loading) {
    return <div className="text-gray-400 text-sm py-4">Loading categories...</div>;
  }

  if (state.tree.length === 0) {
    return <div className="text-gray-400 text-sm py-4">No categories found for this vehicle.</div>;
  }

  return (
    <div className="space-y-3">
      <input
        type="text"
        placeholder="Filter categories..."
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-blue-500"
      />

      <div className="space-y-1">
        {filteredTree.map((group) => {
          const isExpanded = state.expandedGroup === group.name || !!filter;
          const icon = ICONS[group.icon || 'other'] || '📦';

          return (
            <div key={group.name} className="rounded-lg overflow-hidden">
              {/* Group header */}
              <button
                onClick={() => handleGroupClick(group.name)}
                className={`w-full flex items-center justify-between px-4 py-3 text-sm font-medium transition-colors ${
                  isExpanded
                    ? 'bg-gray-700 text-white'
                    : 'bg-gray-800 text-gray-300 hover:bg-gray-750'
                }`}
              >
                <div className="flex items-center gap-2">
                  <span>{icon}</span>
                  <span>{group.name}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-gray-500 text-xs">
                    {group.categories.length} categories · {group.totalParts} parts
                  </span>
                  <svg
                    className={`w-4 h-4 transition-transform ${isExpanded ? 'rotate-180' : ''}`}
                    fill="none" stroke="currentColor" viewBox="0 0 24 24"
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                </div>
              </button>

              {/* Leaf categories */}
              {isExpanded && (
                <div className="bg-gray-850 border-t border-gray-700">
                  {group.categories.map((leaf) => (
                    <button
                      key={leaf.name}
                      onClick={() => handleLeafClick(leaf)}
                      className={`w-full text-left px-6 py-2 text-sm transition-colors border-b border-gray-800 last:border-0 ${
                        state.selectedLeaf === leaf.name
                          ? 'bg-blue-700/30 text-blue-300'
                          : 'text-gray-400 hover:bg-gray-800 hover:text-gray-200'
                      }`}
                    >
                      <span>{leaf.name}</span>
                      <span className="text-gray-600 ml-2">({leaf.partCount})</span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
