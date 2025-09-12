// ui/src/views/ImagesView.tsx
import { useEffect, useMemo, useState } from "react";
import HostPicker from "@/components/HostPicker";
import SortableHeader from "@/components/SortableHeader";
import { Host } from "@/types";

type ImageRow = {
  repo: string;
  tag: string;
  id: string;       // full sha256:...
  size: string;
  created: string;
};

export default function ImagesView({ hosts }: { hosts: Host[] }) {
  const [hostName, setHostName] = useState(hosts[0]?.name || "");
  const [rows, setRows] = useState<ImageRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [sort, setSort] = useState<{ key: keyof ImageRow; direction: 'asc' | 'desc' }>({ key: 'repo', direction: 'asc' });

  // selection
  const [selected, setSelected] = useState<string[]>([]); // image IDs
  const [lastIndex, setLastIndex] = useState<number | null>(null);
  const [force, setForce] = useState<boolean>(true);

  useEffect(() => {
    if (!hostName) return;
    (async () => {
      setLoading(true);
      try {
        const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images`, { credentials: "include" });
        const j = await r.json();
        setRows((j.items || []) as ImageRow[]);
        setSelected([]); setLastIndex(null);
      } finally {
        setLoading(false);
      }
    })();
  }, [hostName]);

  const sortedRows = useMemo(() => {
    const copy = [...rows];
    copy.sort((a, b) => {
      const aVal = (a[sort.key] || '') as string;
      const bVal = (b[sort.key] || '') as string;
      const result = aVal.localeCompare(bVal);
      return sort.direction === 'asc' ? result : -result;
    });
    return copy;
  }, [rows, sort]);

  const handleSort = (key: keyof ImageRow) => {
    setSort(prev => ({ key, direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc' }));
  };

  // selection helpers
  const isSelected = (id: string) => selected.includes(id);
  const toggleOne = (id: string) =>
    setSelected(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]));

  const handleRowClick = (e: React.MouseEvent, index: number, id: string) => {
    if (e.shiftKey && lastIndex !== null) {
      // range select (union with existing)
      const [a, b] = index < lastIndex ? [index, lastIndex] : [lastIndex, index];
      const ids = sortedRows.slice(a, b + 1).map(r => r.id);
      setSelected(prev => Array.from(new Set([...prev, ...ids])));
    } else if (e.metaKey || e.ctrlKey) {
      // toggle this row
      toggleOne(id);
      setLastIndex(index);
    } else {
      // single select
      setSelected([id]);
      setLastIndex(index);
    }
  };

  const allSelected = sortedRows.length > 0 && selected.length === sortedRows.length;
  const toggleSelectAll = () => {
    if (allSelected) {
      setSelected([]);
    } else {
      setSelected(sortedRows.map(r => r.id));
    }
  };

  async function handleDeleteSelected() {
    if (!selected.length) return;
    const body = JSON.stringify({ ids: selected, force });
    const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images/delete`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body,
    });
    const j = await r.json().catch(() => ({}));
    // naive feedback; you can replace with a toast
    if (!r.ok) {
      alert(typeof j === "string" ? j : "Delete failed");
    }
    // refresh
    const rr = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images`, { credentials: "include" });
    const jj = await rr.json();
    setRows((jj.items || []) as ImageRow[]);
    setSelected([]); setLastIndex(null);
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="text-lg font-semibold text-white">Images</div>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-slate-300 text-sm">
            <input
              type="checkbox"
              className="h-4 w-4"
              checked={force}
              onChange={(e) => setForce(e.target.checked)}
            />
            Force
          </label>
          <button
            onClick={handleDeleteSelected}
            disabled={!selected.length || loading}
            className={`px-3 py-1.5 rounded-lg text-sm ${
              selected.length ? "bg-rose-600 hover:bg-rose-700 text-white" : "bg-slate-800 text-slate-400 cursor-not-allowed"
            }`}
            title={selected.length ? `Delete ${selected.length} selected` : "Select rows to delete"}
          >
            Delete selected{selected.length ? ` (${selected.length})` : ""}
          </button>
          <HostPicker hosts={hosts} activeHost={hostName} setActiveHost={setHostName} />
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="w-10 p-2 text-center">
                <input
                  type="checkbox"
                  aria-label="Select all"
                  className="h-4 w-4"
                  checked={allSelected}
                  onChange={toggleSelectAll}
                />
              </th>
              <SortableHeader sortKey="repo" currentSort={sort} onSort={(k)=>handleSort(k as keyof ImageRow)}>Repository</SortableHeader>
              <SortableHeader sortKey="tag" currentSort={sort} onSort={(k)=>handleSort(k as keyof ImageRow)}>Tag</SortableHeader>
              <SortableHeader sortKey="id" currentSort={sort} onSort={(k)=>handleSort(k as keyof ImageRow)}>ID</SortableHeader>
              <SortableHeader sortKey="size" currentSort={sort} onSort={(k)=>handleSort(k as keyof ImageRow)}>Size</SortableHeader>
              <SortableHeader sortKey="created" currentSort={sort} onSort={(k)=>handleSort(k as keyof ImageRow)}>Created</SortableHeader>
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td className="p-3 text-slate-500" colSpan={6}>Loading…</td></tr>}
            {(!loading && sortedRows.length === 0) && <tr><td className="p-3 text-slate-500" colSpan={6}>No images.</td></tr>}
            {sortedRows.map((im, i) => {
              const sel = isSelected(im.id);
              return (
                <tr
                  key={im.id}
                  onClick={(e) => handleRowClick(e, i, im.id)}
                  className={`border-t border-slate-800 cursor-default ${sel ? "bg-slate-900/70" : "hover:bg-slate-900/40"}`}
                >
                  <td className="p-2 text-center" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      className="h-4 w-4"
                      checked={sel}
                      onChange={() => toggleOne(im.id)}
                    />
                  </td>
                  <td className="p-2 text-slate-300">{im.repo || "—"}</td>
                  <td className="p-2 text-slate-300">{im.tag || "—"}</td>
                  <td className="p-2 text-slate-300 font-mono">{im.id?.slice(7, 19) || "—"}</td>
                  <td className="p-2 text-slate-300">{im.size || "—"}</td>
                  <td className="p-2 text-slate-300">{im.created || "—"}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <p className="text-xs text-slate-500">
        Tip: Click to select one, Ctrl/Cmd-click to toggle, Shift-click to select a range.
      </p>
    </div>
  );
}
