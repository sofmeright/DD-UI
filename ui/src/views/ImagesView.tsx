import React, { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronUp, Trash2, RefreshCw } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import type { Host } from "./App";

/* Inline helpers so file is self-contained */
function HostPicker({
  hosts, activeHost, setActiveHost,
}: { hosts: Host[]; activeHost: string; setActiveHost: (n: string)=>void }) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-slate-300">Host</span>
      <select
        className="bg-slate-950 border border-slate-800 text-slate-200 text-sm rounded px-2 py-1"
        value={activeHost}
        onChange={(e) => setActiveHost(e.target.value)}
      >
        {hosts.map(h => <option key={h.name} value={h.name}>{h.name}</option>)}
      </select>
    </div>
  );
}

function SortableHeader({
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
      <button className="flex items-center gap-1 hover:text-white transition" onClick={() => onSort(sortKey)}>
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

export default function ImagesView({ hosts }: { hosts: Host[] }) {
  const [hostName, setHostName] = useState(hosts[0]?.name || "");
  const [rows, setRows] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [sort, setSort] = useState<{ key: string; direction: 'asc' | 'desc' }>({ key: 'repo', direction: 'asc' });
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [force, setForce] = useState<boolean>(true);

  async function load() {
    if (!hostName) return;
    setLoading(true);
    try {
      const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images`, { credentials: "include" });
      const j = await r.json();
      setRows(j.items || []);
      setSelected({});
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); /* eslint-disable-next-line */ }, [hostName]);

  const allChecked = useMemo(() => {
    const ids = rows.map((r) => r.id);
    if (!ids.length) return false;
    return ids.every((id) => selected[id]);
  }, [rows, selected]);

  const toggleAll = (v: boolean) => {
    const next: Record<string, boolean> = {};
    if (v) for (const r of rows) next[r.id] = true;
    setSelected(next);
  };

  const sortedRows = useMemo(() => {
    return [...rows].sort((a, b) => {
      const aVal = (a[sort.key] || '').toString();
      const bVal = (b[sort.key] || '').toString();
      const result = aVal.localeCompare(bVal);
      return sort.direction === 'asc' ? result : -result;
    });
  }, [rows, sort]);

  const handleSort = (key: string) => {
    setSort(prev => ({
      key,
      direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc'
    }));
  };

  async function deleteSelected() {
    const ids = Object.keys(selected).filter(k => selected[k]);
    if (ids.length === 0) return;
    if (!confirm(`Delete ${ids.length} image(s) on ${hostName}?`)) return;

    setLoading(true);
    try {
      const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images/delete`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ids, force }),
      });
      if (!r.ok) {
        const txt = await r.text();
        alert(`Delete failed: ${r.status} ${txt}`);
      }
      await load();
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-lg font-semibold text-white">Images</div>
        <div className="flex gap-3 items-center">
          <div className="flex items-center gap-2 text-sm">
            <span className="text-slate-300">Force</span>
            <Switch checked={force} onCheckedChange={(v) => setForce(!!v)} />
          </div>
          <Button
            variant="outline"
            className="border-rose-700 text-rose-200"
            disabled={loading}
            onClick={deleteSelected}
          >
            <Trash2 className="h-4 w-4 mr-1" /> Delete Selected
          </Button>
          <Button variant="outline" className="border-slate-700" onClick={load} disabled={loading}>
            <RefreshCw className={`h-4 w-4 mr-1 ${loading ? "animate-spin" : ""}`} /> Refresh
          </Button>
          <HostPicker hosts={hosts} activeHost={hostName} setActiveHost={setHostName} />
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-2 w-10 text-left">
                <input type="checkbox" checked={allChecked} onChange={(e) => toggleAll(e.target.checked)} />
              </th>
              <SortableHeader sortKey="repo" currentSort={sort} onSort={handleSort}>Repository</SortableHeader>
              <SortableHeader sortKey="tag" currentSort={sort} onSort={handleSort}>Tag</SortableHeader>
              <SortableHeader sortKey="id" currentSort={sort} onSort={handleSort}>ID (sha256)</SortableHeader>
              <SortableHeader sortKey="size" currentSort={sort} onSort={handleSort}>Size</SortableHeader>
              <SortableHeader sortKey="created" currentSort={sort} onSort={handleSort}>Created</SortableHeader>
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td className="p-3 text-slate-500" colSpan={6}>Loading…</td></tr>}
            {(!loading && sortedRows.length === 0) && <tr><td className="p-3 text-slate-500" colSpan={6}>No images.</td></tr>}
            {sortedRows.map((im, i) => {
              const checked = !!selected[im.id];
              return (
                <tr key={im.id || i} className="border-t border-slate-800 hover:bg-slate-900/40">
                  <td className="p-2">
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={(e) => setSelected(s => ({ ...s, [im.id]: e.target.checked }))}
                    />
                  </td>
                  <td className="p-2 text-slate-300">{im.repo || "—"}</td>
                  <td className="p-2 text-slate-300">{im.tag || "—"}</td>
                  <td className="p-2 text-slate-300 font-mono text-xs break-all">{im.id || "—"}</td>
                  <td className="p-2 text-slate-300">{im.size || "—"}</td>
                  <td className="p-2 text-slate-300">{im.created || "—"}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}