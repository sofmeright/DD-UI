import React from "react";
import { RefreshCw, Boxes, Layers, AlertTriangle, XCircle, Search } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Host } from "./App";

/* Small metric card */
function MetricCard({
  title, value, icon: Icon, accent = false,
}: { title: string; value: React.ReactNode; icon: any; accent?: boolean }) {
  return (
    <Card className={`border-slate-800 ${accent ? "bg-slate-900/40 border-brand/40" : "bg-slate-900/40"}`}>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-slate-300">{title}</CardTitle>
        <Icon className="h-4 w-4 text-slate-400" />
      </CardHeader>
      <CardContent>
        <div className={`text-2xl font-extrabold ${accent ? "text-brand" : "text-white"}`}>{value}</div>
      </CardContent>
    </Card>
  );
}

export default function DeploymentsView({
  hosts,
  filteredHosts,
  loading,
  err,
  query,
  setQuery,
  scanning,
  onScanAll,
  onScanHost,
  onOpenHost,
  metricsSummary,
  hostMetrics,
}: {
  hosts: Host[];
  filteredHosts: Host[];
  loading?: boolean;
  err?: string | null;
  query: string;
  setQuery: (v: string) => void;
  scanning: boolean;
  onScanAll: () => void | Promise<void>;
  onScanHost: (hostName: string) => void | Promise<void>;
  onOpenHost: (hostName: string) => void;
  metricsSummary: { hosts: number; stacks: number; containers: number; drift: number; errors: number };
  hostMetrics: Record<string, { stacks: number; containers: number; drift: number; errors: number }>;
}) {
  return (
    <div className="space-y-4">
      {/* Summary metrics */}
      <div className="grid md:grid-cols-5 gap-4">
        <MetricCard title="Hosts" value={metricsSummary.hosts} icon={Boxes} accent />
        <MetricCard title="Stacks" value={metricsSummary.stacks} icon={Boxes} />
        <MetricCard title="Containers" value={metricsSummary.containers} icon={Layers} />
        <MetricCard title="Drift" value={<span className="text-amber-400">{metricsSummary.drift}</span>} icon={AlertTriangle} />
        <MetricCard title="Errors" value={<span className="text-rose-400">{metricsSummary.errors}</span>} icon={XCircle} />
      </div>

      {/* Toolbar */}
      <Card className="bg-slate-900/40 border-slate-800">
        <CardContent className="py-4">
          <div className="flex items-center gap-2">
            <Button onClick={onScanAll} disabled={scanning} className="bg-[#310937] hover:bg-[#2a0830] text-white">
              <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "animate-spin" : ""}`} />
              {scanning ? "Scanning…" : "Sync"}
            </Button>
            <div className="relative w-full md:w-96">
              <Search className="h-4 w-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
              <Input
                placeholder="Filter by host, group, address…"
                className="pl-9 bg-slate-900/50 border-slate-800 text-slate-200 placeholder:text-slate-500"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Hosts table */}
      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-3 text-left">Host</th>
              <th className="p-3 text-left">Address</th>
              <th className="p-3 text-left">Groups</th>
              <th className="p-3 text-left">Scan</th>
              <th className="p-3 text-left">Stacks</th>
              <th className="p-3 text-left">Containers</th>
              <th className="p-3 text-left">Drift</th>
              <th className="p-3 text-left">Errors</th>
            </tr>
          </thead>
          <tbody>
            {loading && (
              <tr><td className="p-4 text-slate-500" colSpan={8}>Loading hosts…</td></tr>
            )}
            {err && !loading && (
              <tr><td className="p-4 text-rose-300" colSpan={8}>{err}</td></tr>
            )}
            {!loading && filteredHosts.map((h) => {
              const m = hostMetrics[h.name] || { stacks: 0, containers: 0, drift: 0, errors: 0 };
              return (
                <tr key={h.name} className="border-t border-slate-800 hover:bg-slate-900/40">
                  <td className="p-3 font-medium text-slate-200">
                    <button className="hover:underline" onClick={() => onOpenHost(h.name)}>
                      {h.name}
                    </button>
                  </td>
                  <td className="p-3 text-slate-300">{h.address || "—"}</td>
                  <td className="p-3 text-slate-300">{(h.groups || []).length ? (h.groups || []).join(", ") : "—"}</td>
                  <td className="p-3">
                    <Button
                      size="sm"
                      variant="outline"
                      className="border-slate-700 text-slate-200 hover:bg-slate-800"
                      onClick={() => onScanHost(h.name)}
                      disabled={scanning}
                    >
                      <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "opacity-60" : ""}`} />
                      Scan
                    </Button>
                  </td>
                  <td className="p-3 text-slate-300">{m.stacks}</td>
                  <td className="p-3 text-slate-300">{m.containers}</td>
                  <td className="p-3 text-amber-300">{m.drift}</td>
                  <td className="p-3 text-rose-300">{m.errors}</td>
                </tr>
              );
            })}
            {!loading && filteredHosts.length === 0 && (
              <tr><td className="p-6 text-center text-slate-500" colSpan={8}>No hosts.</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
