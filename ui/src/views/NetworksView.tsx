import { useEffect, useMemo, useState } from "react";
import HostPicker from "@/components/HostPicker";
import SortableHeader from "@/components/SortableHeader";
import { Host } from "@/types";

export default function NetworksView({ hosts }: { hosts: Host[] }) {
  const [hostName, setHostName] = useState(hosts[0]?.name || "");
  const [rows, setRows] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [sort, setSort] = useState<{ key: string; direction: 'asc' | 'desc' }>({ key: 'name', direction: 'asc' });

  useEffect(() => {
    if (!hostName) return;
    (async () => {
      setLoading(true);
      const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/networks`, { credentials: "include" });
      const j = await r.json();
      setRows(j.items || []);
      setLoading(false);
    })();
  }, [hostName]);

  const sortedRows = useMemo(() => {
    return [...rows].sort((a, b) => {
      const aVal = a[sort.key] || '';
      const bVal = b[sort.key] || '';
      const result = aVal.localeCompare(bVal);
      return sort.direction === 'asc' ? result : -result;
    });
  }, [rows, sort]);

  const handleSort = (key: string) => {
    setSort(prev => ({ key, direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc' }));
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-lg font-semibold text-white">Networks</div>
        <HostPicker hosts={hosts} activeHost={hostName} setActiveHost={setHostName} />
      </div>
      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <SortableHeader sortKey="name" currentSort={sort} onSort={handleSort}>Name</SortableHeader>
              <SortableHeader sortKey="driver" currentSort={sort} onSort={handleSort}>Driver</SortableHeader>
              <SortableHeader sortKey="scope" currentSort={sort} onSort={handleSort}>Scope</SortableHeader>
              <SortableHeader sortKey="id" currentSort={sort} onSort={handleSort}>ID</SortableHeader>
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td className="p-3 text-slate-500" colSpan={4}>Loading…</td></tr>}
            {(!loading && sortedRows.length === 0) && <tr><td className="p-3 text-slate-500" colSpan={4}>No networks.</td></tr>}
            {sortedRows.map((n, i) => (
              <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40">
                <td className="p-2 text-slate-300">{n.name}</td>
                <td className="p-2 text-slate-300">{n.driver}</td>
                <td className="p-2 text-slate-300">{n.scope}</td>
                <td className="p-2 text-slate-300 font-mono">{n.id?.slice(0,12)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
