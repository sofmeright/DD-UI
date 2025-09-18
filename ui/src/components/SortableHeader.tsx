// ui/src/components/SortableHeader.tsx
import { ChevronUp, ChevronDown } from "lucide-react";

export default function SortableHeader({
  children, sortKey, currentSort, onSort
}: {
  children: React.ReactNode;
  sortKey: string;
  currentSort: { key: string; direction: 'asc' | 'desc' };
  onSort: (key: string) => void;
}) {
  const isActive = currentSort.key === sortKey;
  const direction = isActive ? currentSort.direction : 'asc';
  
  return (
    <th className="p-2 text-left">
      <button
        className="flex items-center gap-1 hover:text-white transition"
        onClick={() => onSort(sortKey)}
      >
        {children}
        {isActive ? (
          direction === 'asc' ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />
        ) : (
          <ChevronUp className="h-3 w-3 opacity-30" />
        )}
      </button>
    </th>
  );
}
