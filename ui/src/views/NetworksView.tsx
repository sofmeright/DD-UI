// ui/src/views/NetworksView.tsx
import { useEffect, useMemo, useState } from "react";
import HostPicker from "@/components/HostPicker";
import SortableHeader from "@/components/SortableHeader";
import { Host } from "@/types";

type NetRow = { name: string; driver: string; scope: string; id: string };

export default function NetworksView({ hosts }: { hosts: Host[] }) {
  const [hostName, setHostName] = useState(hosts[0]?.name || "");
  const [rows, setRows] = useState<NetRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [sort, setSort] = useState<{ key: keyof NetRow; direction: 'asc' | 'desc' }>({ key: 'name', direction: 'asc' });

  // selection
  const [selected, setSelected] = useState<string[]>([]); // names
  const [lastIndex, setLastIndex] = useState<number | null>(null);

  useEffect(() => {
    if (!hostName) return;
    (async () => {
      setLoading(true);
      try {
        const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/networks`, { credentials: "include" });
        const j = await r.json();
        setRows((j.items || []) as NetRow[]);
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

  const handleSort = (key: keyof NetRow) => {
    setSort(prev => ({ key, direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc' }));
  };

  // selection helpers
  const isSelected = (name: string) => selected.includes(name);
  const toggleOne = (name: string) =>
    setSelected(prev => (prev.includes(name) ? prev.filter(x => x !== name) : [...prev, name]));

  const handleRowClick = (e: React.MouseEvent, index: number, name: string) => {
    if (e.shiftKey && lastIndex !== null) {
      const [a, b] = index < lastIndex ? [index, lastIndex] : [lastIndex, index];
      const names = sortedRows.slice(a, b + 1).map(r => r.name);
      setSelected(prev => Array.from(new Set([...prev, ...names])));
    } else if (e.metaKey || e.ctrlKey) {
      toggleOne(name);
      setLastIndex(index);
    } else {
      setSelected([name]);
      setLastIndex(index);
    }
  };

  const allSelected = sortedRows.length > 0 && selected.length === sortedRows.length;
  const toggleSelectAll = () => {
    if (allSelected) {
      setSelected([]);
    } else {
      setSelected(sortedRows.map(r => r.name));
    }
  };

  async function handleDeleteSelected() {
    if (!selected.length) return;
    const body = JSON.stringify({ names: selected });
    const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/networks/delete`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body,
    });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) {
      alert(typeof j === "string" ? j : "Delete failed");
    }
    // refresh
    const rr = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/networks`, { credentials: "include" });
    const jj = await rr.json();
    setRows((jj.items || []) as NetRow[]);
    setSelected([]); setLastIndex(null);
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="text-lg font-semibold text-white">Networks</div>
        <div className="flex items-center gap-3">
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
              <SortableHeader sortKey="name" currentSort={sort} onSort={(k)=>handleSort(k as keyof NetRow)}>Name</SortableHeader>
              <SortableHeader sortKey="driver" currentSort={sort} onSort={(k)=>handleSort(k as keyof NetRow)}>Driver</SortableHeader>
              <SortableHeader sortKey="scope" currentSort={sort} onSort={(k)=>handleSort(k as keyof NetRow)}>Scope</SortableHeader>
              <SortableHeader sortKey="id" currentSort={sort} onSort={(k)=>handleSort(k as keyof NetRow)}>ID</SortableHeader>
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td className="p-3 text-slate-500" colSpan={5}>Loading…</td></tr>}
            {(!loading && sortedRows.length === 0) && <tr><td className="p-3 text-slate-500" colSpan={5}>No networks.</td></tr>}
            {sortedRows.map((n, i) => {
              const sel = isSelected(n.name);
              return (
                <tr
                  key={n.name}
                  onClick={(e) => handleRowClick(e, i, n.name)}
                  className={`border-t border-slate-800 cursor-default ${sel ? "bg-slate-900/70" : "hover:bg-slate-900/40"}`}
                >
                  <td className="p-2 text-center" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      className="h-4 w-4"
                      checked={sel}
                      onChange={() => toggleOne(n.name)}
                    />
                  </td>
                  <td className="p-2 text-slate-300">{n.name}</td>
                  <td className="p-2 text-slate-300">{n.driver}</td>
                  <td className="p-2 text-slate-300">{n.scope}</td>
                  <td className="p-2 text-slate-300 font-mono">{n.id?.slice(0,12)}</td>
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
