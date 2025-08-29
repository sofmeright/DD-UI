import React, { useEffect, useMemo, useState } from "react";
import { Boxes, Layers, AlertTriangle, XCircle, Search, PanelLeft, RefreshCw, PlayCircle, Rocket, ShieldCheck } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";

type Host = {
  name: string;
  address?: string;
  groups?: string[];
  lastSync?: string;      // ISO string if your API sets this later
  stacks?: Array<{
    name: string;
    type?: string;
    status?: "in_sync" | "drift" | "stopped" | "error";
    pullPolicy?: string;
    sops?: boolean;
    containers: Array<{
      name: string;
      image: string;
      desiredImage?: string;
      state: "running" | "exited" | string;
      ports?: string;
      created?: string;
      lastPulled?: string;
    }>;
  }>;
};

function MetricCard({ title, value, icon: Icon, accent=false }: { title: string; value: React.ReactNode; icon: any; accent?: boolean }) {
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

export default function App() {
  const [query, setQuery] = useState("");
  const [toggles, setToggles] = useState({ staging: false, autoPull: false, applyOnChange: false });
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  // Fetch hosts on load
  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setErr(null);
      try {
        const res = await fetch("/api/hosts", { credentials: "include" });
        if (res.status === 401) {
          window.location.href = "/login";
          return;
        }
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        if (!cancelled) setHosts(Array.isArray(data.items) ? data.items : []);
      } catch (e: any) {
        if (!cancelled) setErr(e?.message || "Failed to load hosts");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const filteredHosts = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter(h => {
      const hay = [
        h.name,
        h.address || "",
        ...(h.groups || [])
      ].join(" ").toLowerCase();
      return hay.includes(q);
    });
  }, [hosts, query]);

  const flatStacks = useMemo(
    () => filteredHosts.flatMap(h => h.stacks || []),
    [filteredHosts]
  );
  const metrics = useMemo(() => {
    const stacks = flatStacks.length;
    const containers = flatStacks.reduce((acc, s) => acc + (s.containers?.length || 0), 0);
    const drift = flatStacks.filter(s => s.status === "drift").length;
    const errors = flatStacks.filter(s => s.status === "error").length;
    return { stacks, containers, drift, errors };
  }, [flatStacks]);

  return (
    <div className="min-h-screen">
      <div className="bg-slate-950 border-b border-slate-800">
        <div className="max-w-7xl mx-auto px-4 py-3 flex items-center gap-3">
          <Button variant="ghost" size="icon"><PanelLeft className="h-5 w-5 text-slate-300" /></Button>
          <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none flex items-center gap-2">
            DD<span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">UI</span>
            <Badge variant="outline">Community</Badge>
          </div>
          <Separator orientation="vertical" className="mx-2 h-8 bg-slate-800" />
          <div className="ml-auto flex items-center gap-2">
            <Button className="bg-[#310937] hover:bg-[#2a0830] text-white">
              <RefreshCw className="h-4 w-4 mr-1" /> Sync
            </Button>
            <Button variant="outline" className="border-brand bg-slate-900/70 text-white hover:bg-slate-800">
              <PlayCircle className="h-4 w-4 mr-1" /> Plan
            </Button>
            <Button className="bg-brand hover:brightness-110 text-slate-900">
              <Rocket className="h-4 w-4 mr-1" /> Apply
            </Button>
          </div>
        </div>
      </div>

      <main className="max-w-7xl mx-auto px-4 py-6 space-y-6">
        {/* Loading/Error banners */}
        {loading && (
          <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">
            Loading hosts…
          </div>
        )}
        {err && (
          <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">
            Error: {err}
          </div>
        )}

        <div className="grid md:grid-cols-4 gap-4">
          <MetricCard title="Stacks" value={metrics.stacks} icon={Boxes} accent />
          <MetricCard title="Containers" value={metrics.containers} icon={Layers} accent />
          <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
          <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
        </div>

        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-4">
            <div className="flex flex-col md:flex-row gap-3 md:items-center md:justify-between">
              <div className="flex items-center gap-2 w-full md:w-auto">
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
              <div className="flex items-center gap-5">
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <Switch checked={toggles.staging} onCheckedChange={(v:boolean)=>setToggles(t=>({...t, staging:v}))} /> Staging Mode
                </label>
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <Switch checked={toggles.autoPull} onCheckedChange={(v:boolean)=>setToggles(t=>({...t, autoPull:v}))} /> Auto pull latest
                </label>
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <Switch checked={toggles.applyOnChange} onCheckedChange={(v:boolean)=>setToggles(t=>({...t, applyOnChange:v}))} /> Apply on change
                </label>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-4 flex flex-wrap items-center gap-3 text-sm text-slate-300">
            <ShieldCheck className="h-4 w-4" /> Security by default:
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">AGE key never persisted</span>
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Decrypt to tmpfs only</span>
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
          </CardContent>
        </Card>

        <Tabs defaultValue="manifest" className="mt-2">
          <TabsList className="bg-slate-900/60 border border-slate-800">
            <TabsTrigger value="manifest">Manifest View</TabsTrigger>
            <TabsTrigger value="cards">Cards View</TabsTrigger>
          </TabsList>

          {/* Manifest: simple host table for now */}
          <TabsContent value="manifest" className="mt-4">
            <div className="overflow-hidden rounded-xl border border-slate-800">
              <table className="w-full text-sm">
                <thead className="bg-slate-900/70 text-slate-300">
                  <tr>
                    <th className="p-3 text-left">Host</th>
                    <th className="p-3 text-left">Address</th>
                    <th className="p-3 text-left">Groups</th>
                    <th className="p-3 text-left">Last Sync</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredHosts.map((h) => (
                    <tr key={h.name} className="border-t border-slate-800 hover:bg-slate-900/40">
                      <td className="p-3 font-medium text-slate-200">{h.name}</td>
                      <td className="p-3 text-slate-300">{h.address || "—"}</td>
                      <td className="p-3 text-slate-300">
                        {(h.groups || []).length ? (h.groups || []).join(", ") : "—"}
                      </td>
                      <td className="p-3 text-slate-300">
                        {h.lastSync ? new Date(h.lastSync).toLocaleString() : "—"}
                      </td>
                    </tr>
                  ))}
                  {!loading && !err && filteredHosts.length === 0 && (
                    <tr>
                      <td className="p-6 text-center text-slate-500" colSpan={4}>No hosts.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </TabsContent>

          {/* Cards: minimal host tiles (stacks will populate later) */}
          <TabsContent value="cards" className="mt-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
              {filteredHosts.map(h => (
                <Card key={h.name} className="bg-slate-900/50 border-slate-800 rounded-2xl">
                  <CardHeader className="pb-2">
                    <CardTitle className="text-2xl font-extrabold tracking-tight text-white">{h.name}</CardTitle>
                    <div className="text-xs text-slate-400">{h.address || "—"}</div>
                  </CardHeader>
                  <CardContent className="text-sm text-slate-300">
                    <div><span className="text-slate-400">Groups:</span> {(h.groups || []).length ? (h.groups || []).join(", ") : "—"}</div>
                    <div><span className="text-slate-400">Stacks:</span> {(h.stacks || []).length}</div>
                  </CardContent>
                </Card>
              ))}
            </div>
            {!loading && !err && filteredHosts.length === 0 && (
              <div className="text-center py-20 text-slate-400">No hosts.</div>
            )}
          </TabsContent>
        </Tabs>
      </main>
    </div>
  );
}
