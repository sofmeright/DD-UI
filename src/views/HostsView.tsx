// ui/src/views/HostsView.tsx  
import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import MetricCard from "@/components/MetricCard";
import { Boxes, Layers, AlertTriangle, XCircle, RefreshCw } from "lucide-react";
import { Host } from "@/types";

export default function HostsView({
  metrics, hosts, filteredHosts, loading, err, scanning, onScanAll, onFilter, onOpenHost, refreshMetricsForHosts,
}: {
  metrics: { hosts: number; stacks: number; containers: number; drift: number; errors: number };
  hosts: Host[];
  filteredHosts: Host[];
  loading: boolean;
  err: string | null;
  scanning: boolean;
  onScanAll: () => Promise<void> | void;
  onFilter: (v: string) => void;
  onOpenHost: (name: string) => void;
  refreshMetricsForHosts: (names: string[]) => Promise<void>;
}) {
  const [query, setQuery] = useState("");

  return (
    <div className="space-y-4">
      <div className="grid md:grid-cols-5 gap-4">
        <MetricCard title="Hosts" value={metrics.hosts} icon={Boxes} />
        <MetricCard title="Stacks" value={metrics.stacks} icon={Boxes} />
        <MetricCard title="Containers" value={metrics.containers} icon={Layers} />
        <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
        <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
      </div>

      <Card className="bg-slate-900/40 border-slate-800">
        <CardContent className="py-4">
          <div className="flex items-center gap-2">
            <Button onClick={onScanAll} disabled={scanning} className="bg-[#310937] hover:bg-[#2a0830] text-white">
              <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "animate-spin" : ""}`} />
              {scanning ? "Scanning…" : "Sync"}
            </Button>
            <div className="relative w-full md:w-96">
              <Input
                placeholder="Filter by host, group, address…"
                className="pl-3 bg-slate-900/50 border-slate-800 text-slate-200 placeholder:text-slate-500"
                value={query}
                onChange={(e) => { setQuery(e.target.value); onFilter(e.target.value); }}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-3 text-left">Host</th>
              <th className="p-3 text-left">Address</th>
              <th className="p-3 text-left">Groups</th>
              <th className="p-3 text-left">Scan</th>
              <th className="p-3 text-left">Status</th>
            </tr>
          </thead>
          <tbody>
            {loading && (<tr><td className="p-4 text-slate-500" colSpan={5}>Loading hosts…</td></tr>)}
            {err && !loading && (<tr><td className="p-4 text-rose-300" colSpan={5}>{err}</td></tr>)}
            {!loading && filteredHosts.map((h) => (
              <tr key={h.name} className="border-t border-slate-800 hover:bg-slate-900/40">
                <td className="p-3 font-medium text-slate-200">
                  <button className="hover:underline" onClick={() => onOpenHost(h.name)}>{h.name}</button>
                </td>
                <td className="p-3 text-slate-300">{h.address || "—"}</td>
                <td className="p-3 text-slate-300">{(h.groups || []).length ? (h.groups || []).join(", ") : "—"}</td>
                <td className="p-3">
                  <Button
                    size="sm"
                    variant="outline"
                    className="border-slate-700 text-slate-200 hover:bg-slate-800"
                    onClick={async () => {
                      await fetch(`/api/scan/host/${encodeURIComponent(h.name)}`, { method: "POST", credentials: "include" });
                      await refreshMetricsForHosts([h.name]);
                    }}
                  >
                    <RefreshCw className="h-4 w-4 mr-1" />
                    Scan
                  </Button>
                </td>
                <td className="p-3"></td>
              </tr>
            ))}
            {!loading && filteredHosts.length === 0 && (
              <tr><td className="p-6 text-center text-slate-500" colSpan={5}>No hosts.</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}