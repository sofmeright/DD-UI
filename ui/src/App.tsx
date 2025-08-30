// ui/src/App.tsx
import React, { useEffect, useMemo, useState } from "react";
import {
  Boxes, Layers, AlertTriangle, XCircle, Search, PanelLeft,
  RefreshCw, PlayCircle, Rocket, ShieldCheck
} from "lucide-react";
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
  lastSync?: string;
  stacks?: Array<{
    name: string;                // stack/project name or "(none)"
    containers: Array<{
      name: string;
      image: string;
      state: string;             // running/exited/…
      status: string;
    }>;
  }>;
};

type ScanHostResult = {
  host: string;
  saved?: number;
  status?: "ok" | "skipped";
  err?: string;
  reason?: string;
};

type ScanAllResponse = {
  hosts_total: number;
  scanned: number;
  skipped: number;
  errors: number;
  saved: number;
  results: Array<{
    host: string;
    saved?: number;
    err?: string;
    skipped?: boolean;
    reason?: string;
  }>;
  status: "ok";
};

type ApiContainer = {
  name: string;
  image: string;
  state: string;
  status: string;
  stack?: string | null;         // from backend SELECT s.project as stack
  // other fields exist but we don't need them here
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

function StatusPill({ result }: { result?: { kind: "ok" | "skipped" | "error"; saved?: number; reason?: string; err?: string } }) {
  if (!result) return null;
  const base = "px-2 py-0.5 rounded text-xs border";
  if (result.kind === "ok") {
    return <span className={`${base} border-emerald-700/50 bg-emerald-900/30 text-emerald-200`}>OK{typeof result.saved === "number" ? ` • saved ${result.saved}` : ""}</span>;
  }
  if (result.kind === "skipped") {
    return <span className={`${base} border-amber-700/50 bg-amber-900/30 text-amber-200`}>Skipped{result.reason ? ` • ${result.reason}` : ""}</span>;
  }
  return <span className={`${base} border-rose-700/50 bg-rose-900/30 text-rose-200`}>Error{result.err ? ` • ${result.err}` : ""}</span>;
}

export default function App() {
  const [query, setQuery] = useState("");
  const [toggles, setToggles] = useState({ staging: false, autoPull: false, applyOnChange: false });

  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [stacksLoading, setStacksLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // scanning UI state
  const [scanning, setScanning] = useState(false);
  const [scanSummary, setScanSummary] = useState<ScanAllResponse | null>(null);
  const [scanErr, setScanErr] = useState<string | null>(null);
  const [hostScanState, setHostScanState] = useState<Record<string, { kind: "ok" | "skipped" | "error"; saved?: number; reason?: string; err?: string }>>({});

  // --- fetch hosts then their stacks/containers ---
  async function fetchHosts() {
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
      const items = Array.isArray(data.items) ? data.items : [];
      const mapped: Host[] = items.map((h: any) => ({
        name: h.name,
        address: h.addr ?? h.address ?? "",
        groups: h.groups ?? [],
        stacks: [],               // will fill below
      }));
      setHosts(mapped);
      // then hydrate stacks
      await hydrateStacks(mapped);
    } catch (e: any) {
      setErr(e?.message || "Failed to load hosts");
    } finally {
      setLoading(false);
    }
  }

  async function hydrateStacks(hostList: Host[]) {
    if (!hostList.length) return;
    setStacksLoading(true);
    try {
      // fetch containers for all hosts in parallel
      const pairs = await Promise.all(
        hostList.map(async (h) => {
          const res = await fetch(`/api/hosts/${encodeURIComponent(h.name)}/containers`, { credentials: "include" });
          if (res.status === 401) { window.location.href = "/login"; return [h.name, []] as const; }
          if (!res.ok) return [h.name, []] as const;
          const data = await res.json();
          const items: ApiContainer[] = Array.isArray(data.items) ? data.items : [];
          const byStack = new Map<string, ApiContainer[]>();
          for (const c of items) {
            const key = (c.stack && c.stack.trim()) ? c.stack.trim() : "(none)";
            if (!byStack.has(key)) byStack.set(key, []);
            byStack.get(key)!.push(c);
          }
          const stacks = Array.from(byStack.entries()).map(([name, cs]) => ({
            name,
            containers: cs.map(c => ({
              name: c.name,
              image: c.image,
              state: c.state,
              status: c.status,
            })),
          }));
          return [h.name, stacks] as const;
        })
      );

      const map: Record<string, Host["stacks"]> = {};
      for (const [host, stacks] of pairs) {
        map[host] = stacks;
      }
      // merge back into hosts
      setHosts(prev =>
        prev.map(h => ({ ...h, stacks: map[h.name] ?? h.stacks ?? [] }))
      );
    } finally {
      setStacksLoading(false);
    }
  }

  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!cancelled) await fetchHosts();
    })();
    return () => { cancelled = true; };
  }, []);

  // ---- SCAN handlers (unchanged) ----
  async function handleScanAll() {
    if (scanning) return;
    setScanning(true);
    setScanErr(null);
    setScanSummary(null);
    try {
      const res = await fetch("/api/scan/all", { method: "POST", credentials: "include" });
      if (res.status === 401) { window.location.href = "/login"; return; }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data: ScanAllResponse = await res.json();
      setScanSummary(data);
      const map: Record<string, { kind: "ok" | "skipped" | "error"; saved?: number; reason?: string; err?: string }> = {};
      for (const r of data.results) {
        if (r.skipped) map[r.host] = { kind: "skipped", reason: r.reason };
        else if (r.err) map[r.host] = { kind: "error", err: r.err };
        else map[r.host] = { kind: "ok", saved: r.saved ?? 0 };
      }
      setHostScanState(prev => ({ ...prev, ...map }));
      await fetchHosts(); // refresh stacks after scan
    } catch (e: any) {
      setScanErr(e?.message || "Scan failed");
    } finally {
      setScanning(false);
    }
  }

  async function handleScanHost(name: string) {
    if (scanning) return;
    setScanning(true);
    setScanErr(null);
    try {
      const res = await fetch(`/api/scan/host/${encodeURIComponent(name)}`, { method: "POST", credentials: "include" });
      if (res.status === 401) { window.location.href = "/login"; return; }
      const payload = await res.json() as ScanHostResult | { error?: string };
      if ("status" in payload && payload.status === "skipped") {
        setHostScanState(prev => ({ ...prev, [name]: { kind: "skipped" } }));
      } else if ("err" in payload) {
        setHostScanState(prev => ({ ...prev, [name]: { kind: "error", err: (payload as any).err || "error" } }));
      } else {
        const saved = (payload as any).saved ?? 0;
        setHostScanState(prev => ({ ...prev, [name]: { kind: "ok", saved } }));
      }
      // refresh just this host’s stacks
      await hydrateStacks([{ name } as Host]);
    } catch (e: any) {
      setScanErr(e?.message || "Host scan failed");
      setHostScanState(prev => ({ ...prev, [name]: { kind: "error", err: String(e?.message || e) } }));
    } finally {
      setScanning(false);
    }
  }

  // ---- filters/metrics ----
  const filteredHosts = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter(h => {
      const hay = [h.name, h.address || "", ...(h.groups || [])].join(" ").toLowerCase();
      return hay.includes(q);
    });
  }, [hosts, query]);

  const allStacks = useMemo(
    () => filteredHosts.flatMap(h => (h.stacks || []).map(s => ({ host: h.name, ...s }))),
    [filteredHosts]
  );
  const metrics = useMemo(() => {
    const stacks = allStacks.length;
    const containers = allStacks.reduce((acc, s) => acc + (s.containers?.length || 0), 0);
    const drift = 0;  // placeholder until you compute drift
    const errors = 0; // placeholder until you track stack errors
    return { stacks, containers, drift, errors };
  }, [allStacks]);

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
            <Button
              onClick={handleScanAll}
              disabled={scanning}
              className="bg-[#310937] hover:bg-[#2a0830] text-white"
            >
              <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "animate-spin" : ""}`} />
              {scanning ? "Scanning…" : "Sync"}
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
        {stacksLoading && !loading && (
          <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">
            Loading stacks…
          </div>
        )}
        {err && (
          <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">
            Error: {err}
          </div>
        )}

        {/* Scan summary banner */}
        {scanSummary && (
          <div className="text-sm px-3 py-2 rounded-lg border border-slate-700 bg-slate-900/70 text-slate-200">
            Scan complete — scanned {scanSummary.scanned} of {scanSummary.hosts_total}, skipped {scanSummary.skipped}, errors {scanSummary.errors}, total saved {scanSummary.saved}.
          </div>
        )}
        {scanErr && (
          <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">
            Scan error: {scanErr}
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
            <TabsTrigger value="stacks">Stacks</TabsTrigger>
            <TabsTrigger value="cards">Cards View</TabsTrigger>
          </TabsList>

          {/* Manifest: host table with stacks count + scan actions */}
          <TabsContent value="manifest" className="mt-4">
            <div className="overflow-hidden rounded-xl border border-slate-800">
              <table className="w-full text-sm">
                <thead className="bg-slate-900/70 text-slate-300">
                  <tr>
                    <th className="p-3 text-left">Host</th>
                    <th className="p-3 text-left">Address</th>
                    <th className="p-3 text-left">Groups</th>
                    <th className="p-3 text-left">Stacks</th>
                    <th className="p-3 text-left">Scan</th>
                    <th className="p-3 text-left">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredHosts.map((h) => (
                    <tr key={h.name} className="border-t border-slate-800 hover:bg-slate-900/40">
                      <td className="p-3 font-medium text-slate-200">{h.name}</td>
                      <td className="p-3 text-slate-300">{h.address || "—"}</td>
                      <td className="p-3 text-slate-300">{(h.groups || []).length ? (h.groups || []).join(", ") : "—"}</td>
                      <td className="p-3 text-slate-300">{h.stacks?.length ?? 0}</td>
                      <td className="p-3">
                        <Button
                          size="sm"
                          variant="outline"
                          className="border-slate-700 text-slate-200 hover:bg-slate-800"
                          onClick={() => handleScanHost(h.name)}
                          disabled={scanning}
                        >
                          <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "opacity-60" : ""}`} />
                          Scan
                        </Button>
                      </td>
                      <td className="p-3">
                        <StatusPill result={hostScanState[h.name]} />
                      </td>
                    </tr>
                  ))}
                  {!loading && !err && filteredHosts.length === 0 && (
                    <tr>
                      <td className="p-6 text-center text-slate-500" colSpan={6}>No hosts.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </TabsContent>

          {/* Stacks tab: rows of host/stack with container counts */}
          <TabsContent value="stacks" className="mt-4">
            <div className="overflow-hidden rounded-xl border border-slate-800">
              <table className="w-full text-sm">
                <thead className="bg-slate-900/70 text-slate-300">
                  <tr>
                    <th className="p-3 text-left">Host</th>
                    <th className="p-3 text-left">Stack</th>
                    <th className="p-3 text-left">Containers</th>
                    <th className="p-3 text-left">Running</th>
                  </tr>
                </thead>
                <tbody>
                  {allStacks.map((s, i) => {
                    const running = s.containers.filter((c:any) => c.state === "running").length;
                    return (
                      <tr key={`${s.host}:${s.name}:${i}`} className="border-t border-slate-800 hover:bg-slate-900/40">
                        <td className="p-3 font-medium text-slate-200">{s.host}</td>
                        <td className="p-3 text-slate-300">{s.name}</td>
                        <td className="p-3 text-slate-300">{s.containers.length}</td>
                        <td className="p-3 text-slate-300">{running}</td>
                      </tr>
                    );
                  })}
                  {!loading && !stacksLoading && allStacks.length === 0 && (
                    <tr>
                      <td className="p-6 text-center text-slate-500" colSpan={4}>No stacks yet. Try Sync.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </TabsContent>

          {/* Cards: per-host tiles with stacks count */}
          <TabsContent value="cards" className="mt-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
              {filteredHosts.map(h => (
                <Card key={h.name} className="bg-slate-900/50 border-slate-800 rounded-2xl">
                  <CardHeader className="pb-2">
                    <CardTitle className="text-2xl font-extrabold tracking-tight text-white">{h.name}</CardTitle>
                    <div className="text-xs text-slate-400">{h.address || "—"}</div>
                  </CardHeader>
                  <CardContent className="text-sm text-slate-300 space-y-2">
                    <div><span className="text-slate-400">Groups:</span> {(h.groups || []).length ? (h.groups || []).join(", ") : "—"}</div>
                    <div><span className="text-slate-400">Stacks:</span> {h.stacks?.length ?? 0}</div>
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="outline"
                        className="border-slate-700 text-slate-200 hover:bg-slate-800"
                        onClick={() => handleScanHost(h.name)}
                        disabled={scanning}
                      >
                        <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "opacity-60" : ""}`} />
                        Scan
                      </Button>
                      <StatusPill result={hostScanState[h.name]} />
                    </div>
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
