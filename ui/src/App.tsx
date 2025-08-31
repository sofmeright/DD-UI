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
  stacks?: Stack[];
};

type Stack = {
  name: string; // compose project / swarm namespace / "(none)"
  drift?: "in_sync" | "drift" | "unknown";
  iacEnabled?: boolean; // UI-only toggle for now
  pullPolicy?: string;  // placeholder
  sops?: boolean;       // placeholder
  deployKind?: "compose" | "script" | "unmanaged" | "unknown";
  containers: Container[];
};

type Container = {
  name: string;
  image: string;
  state: string;
  status: string;
  owner?: string;
  created?: string; // not provided by current API; left blank
  ip?: string;      // derived from ports if available
  portsText?: string;
  stack?: string;
};

type ApiContainer = {
  name: string;
  image: string;
  state: string;
  status: string;
  stack?: string | null;        // SELECT s.project as stack
  labels?: Record<string,string>;
  ports?: any;                  // JSONB (we store c.Ports from Docker API)
  owner?: string;
  updated_at?: string;
};

type ScanHostResult = { host: string; saved?: number; status?: "ok"|"skipped"; err?: string; reason?: string; };
type ScanAllResponse = {
  hosts_total: number; scanned: number; skipped: number; errors: number; saved: number;
  results: Array<{host:string; saved?:number; err?:string; skipped?:boolean; reason?:string;}>;
  status: "ok";
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

/* ---------------- helpers ---------------- */

function formatPorts(ports: any): { text: string; ip?: string } {
  // We stored list API's c.Ports (array of objects with IP, PublicPort, PrivatePort, Type)
  // but earlier we wrapped as { ports: [...] }. Handle both.
  const arr: any[] =
    Array.isArray(ports) ? ports :
    (ports && Array.isArray(ports.ports)) ? ports.ports : [];

  if (!arr.length) return { text: "—" };

  const chunks: string[] = [];
  let firstIP: string | undefined;

  for (const p of arr) {
    const ip = p.IP || p.Ip || p.ip || "";
    const pub = p.PublicPort ?? p.publicPort;
    const priv = p.PrivatePort ?? p.privatePort;
    const typ = (p.Type ?? p.type ?? "").toString().toLowerCase() || "tcp";
    if (!firstIP && ip) firstIP = ip;
    if (pub && priv) {
      chunks.push(`${ip ? ip + ":" : ""}${pub} → ${priv}/${typ}`);
    } else if (priv) {
      chunks.push(`${priv}/${typ}`);
    }
  }
  return { text: chunks.join(", "), ip: firstIP };
}

function driftBadge(d: Stack["drift"]) {
  if (d === "in_sync")   return <Badge className="bg-emerald-900/40 border-emerald-700/40 text-emerald-200">In sync</Badge>;
  if (d === "drift")     return <Badge variant="destructive">Drift</Badge>;
  return <Badge variant="outline" className="border-slate-700 text-slate-300">Unknown</Badge>;
}

/* ---------------- main component ---------------- */

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

  /* -------- data fetching -------- */

  useEffect(() => {
    let cancel = false;
    (async () => {
      setLoading(true);
      setErr(null);
      try {
        const r = await fetch("/api/hosts", { credentials: "include" });
        if (r.status === 401) { window.location.href = "/login"; return; }
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = await r.json();
        const items = Array.isArray(data.items) ? data.items : [];
        const mapped: Host[] = items.map((h: any) => ({
          name: h.name,
          address: h.addr ?? h.address ?? "",
          groups: h.groups ?? [],
          stacks: [],
        }));
        if (!cancel) setHosts(mapped);
        if (!cancel) await hydrateStacks(mapped);
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load hosts");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
  }, []);

  async function hydrateStacks(hostList: Host[]) {
    if (!hostList.length) return;
    setStacksLoading(true);
    try {
      const results = await Promise.all(
        hostList.map(async (h) => {
          const res = await fetch(`/api/hosts/${encodeURIComponent(h.name)}/containers`, { credentials: "include" });
          if (res.status === 401) { window.location.href = "/login"; return [h.name, []] as const; }
          if (!res.ok) return [h.name, []] as const;
          const j = await res.json();
          const items: ApiContainer[] = Array.isArray(j.items) ? j.items : [];

          // group by stack/project
          const byStack = new Map<string, ApiContainer[]>();
          for (const c of items) {
            const key = (c.stack && c.stack.trim()) ? c.stack.trim() : "(none)";
            if (!byStack.has(key)) byStack.set(key, []);
            byStack.get(key)!.push(c);
          }

          const stacks: Stack[] = Array.from(byStack.entries()).map(([name, cs]) => {
            const conts: Container[] = cs.map((c) => {
              const { text: portsText, ip } = formatPorts((c as any).ports);
              return {
                name: c.name,
                image: c.image,
                state: c.state,
                status: c.status,
                owner: c.owner,
                ip,
                portsText,
                stack: c.stack || "(none)",
              };
            });
            return {
              name,
              drift: "unknown",          // until IaC check exists
              iacEnabled: true,          // default UI state
              pullPolicy: "if_not_present",
              sops: false,
              deployKind: name === "(none)" ? "unmanaged" : "compose",
              containers: conts,
            };
          });

          return [h.name, stacks] as const;
        })
      );

      const map: Record<string, Stack[]> = {};
      for (const [host, stacks] of results) map[host] = stacks;

      setHosts(prev => prev.map(h => ({ ...h, stacks: map[h.name] ?? [] })));
    } finally {
      setStacksLoading(false);
    }
  }

  /* -------- scan handlers -------- */

  async function handleScanAll() {
    if (scanning) return;
    setScanning(true); setScanErr(null); setScanSummary(null);
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
      // refresh stacks after scan
      await hydrateStacks(hosts);
    } catch (e: any) {
      setScanErr(e?.message || "Scan failed");
    } finally {
      setScanning(false);
    }
  }

  async function handleScanHost(name: string) {
    if (scanning) return;
    setScanning(true); setScanErr(null);
    try {
      const res = await fetch(`/api/scan/host/${encodeURIComponent(name)}`, { method: "POST", credentials: "include" });
      if (res.status === 401) { window.location.href = "/login"; return; }
      const p = await res.json() as ScanHostResult | { error?: string };
      if ("status" in p && p.status === "skipped") {
        setHostScanState(prev => ({ ...prev, [name]: { kind: "skipped" } }));
      } else if ("err" in p) {
        setHostScanState(prev => ({ ...prev, [name]: { kind: "error", err: (p as any).err || "error" } }));
      } else {
        const saved = (p as any).saved ?? 0;
        setHostScanState(prev => ({ ...prev, [name]: { kind: "ok", saved } }));
      }
      // refresh just this host
      const h = hosts.find(x => x.name === name);
      if (h) await hydrateStacks([h]);
    } catch (e: any) {
      setScanErr(e?.message || "Host scan failed");
      setHostScanState(prev => ({ ...prev, [name]: { kind: "error", err: String(e?.message || e) } }));
    } finally {
      setScanning(false);
    }
  }

  /* -------- filters/metrics -------- */

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
    const drift = allStacks.filter(s => s.drift === "drift").length;
    const errors = 0; // None tracked yet
    return { stacks, containers, drift, errors };
  }, [allStacks]);

  /* ---------------- render ---------------- */

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
            <Button onClick={handleScanAll} disabled={scanning} className="bg-[#310937] hover:bg-[#2a0830] text-white">
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
        {loading && (
          <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">
            Loading hosts…
          </div>
        )}
        {stacksLoading && !loading && (
          <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">
            Loading stacks & containers…
          </div>
        )}
        {err && (
          <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">
            Error: {err}
          </div>
        )}
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

        <Tabs defaultValue="stacks" className="mt-2">
          <TabsList className="bg-slate-900/60 border border-slate-800">
            <TabsTrigger value="stacks">Stacks</TabsTrigger>
            <TabsTrigger value="manifest">Hosts</TabsTrigger>
            <TabsTrigger value="cards">Cards</TabsTrigger>
          </TabsList>

          {/* Stacks view: host → stacks → container table */}
          <TabsContent value="stacks" className="mt-4 space-y-6">
            {filteredHosts.map((h) => (
              <div key={h.name} className="space-y-3">
                <div className="text-lg font-semibold text-white">{h.name} <span className="text-slate-400 text-sm">{h.address || ""}</span></div>
                {(h.stacks || []).map((s, i) => (
                  <Card key={`${h.name}:${s.name}:${i}`} className="bg-slate-900/50 border-slate-800 rounded-xl">
                    <CardHeader className="pb-2 flex flex-row items-center justify-between">
                      <div className="space-y-1">
                        <CardTitle className="text-xl text-white">{s.name}</CardTitle>
                        <div className="flex items-center gap-2">
                          {driftBadge(s.drift)}
                          <Badge variant="outline" className="border-slate-700 text-slate-300">
                            {s.deployKind || "unknown"}
                          </Badge>
                          <Badge variant="outline" className="border-slate-700 text-slate-300">
                            pull: {s.pullPolicy || "—"}
                          </Badge>
                          {s.sops ? (
                            <Badge className="bg-indigo-900/40 border-indigo-700/40 text-indigo-200">SOPS</Badge>
                          ) : (
                            <Badge variant="outline" className="border-slate-700 text-slate-300">no SOPS</Badge>
                          )}
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-slate-300">IaC enabled</span>
                        <Switch
                          checked={!!s.iacEnabled}
                          onCheckedChange={(v:boolean) => {
                            setHosts(prev => prev.map(H => H.name === h.name
                              ? ({ ...H, stacks: (H.stacks || []).map(S => S.name === s.name ? ({ ...S, iacEnabled: v }) : S) })
                              : H
                            ));
                          }}
                        />
                      </div>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <div className="overflow-x-auto rounded-lg border border-slate-800">
                        <table className="w-full text-sm">
                          <thead className="bg-slate-900/70 text-slate-300">
                            <tr>
                              <th className="p-3 text-left">Name</th>
                              <th className="p-3 text-left">State</th>
                              <th className="p-3 text-left">Stack</th>
                              <th className="p-3 text-left">Image</th>
                              <th className="p-3 text-left">Created</th>
                              <th className="p-3 text-left">IP Address</th>
                              <th className="p-3 text-left">Published Ports</th>
                              <th className="p-3 text-left">Owner</th>
                            </tr>
                          </thead>
                          <tbody>
                            {s.containers.map((c, idx) => (
                              <tr key={idx} className="border-t border-slate-800 hover:bg-slate-900/40">
                                <td className="p-3 font-medium text-slate-200">{c.name}</td>
                                <td className="p-3 text-slate-300">{c.state}</td>
                                <td className="p-3 text-slate-300">{c.stack || s.name}</td>
                                <td className="p-3 text-slate-300">{c.image}</td>
                                <td className="p-3 text-slate-300">{c.created || "—"}</td>
                                <td className="p-3 text-slate-300">{c.ip || "—"}</td>
                                <td className="p-3 text-slate-300">{c.portsText || "—"}</td>
                                <td className="p-3 text-slate-300">{c.owner || "—"}</td>
                              </tr>
                            ))}
                            {(!s.containers || s.containers.length === 0) && (
                              <tr><td className="p-4 text-slate-500" colSpan={8}>No containers</td></tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    </CardContent>
                  </Card>
                ))}
                {(!h.stacks || h.stacks.length === 0) && (
                  <div className="text-slate-400 text-sm">No stacks yet for this host. Try Sync.</div>
                )}
              </div>
            ))}

            {/* security section under the stacks */}
            <Card className="bg-slate-900/40 border-slate-800">
              <CardContent className="py-4 flex flex-wrap items-center gap-3 text-sm text-slate-300">
                <ShieldCheck className="h-4 w-4" /> Security by default:
                <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">AGE key never persisted</span>
                <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Decrypt to tmpfs only</span>
                <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
                <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
              </CardContent>
            </Card>
          </TabsContent>

          {/* Hosts (manifest) */}
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
                        <Button size="sm" variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={() => handleScanHost(h.name)} disabled={scanning}>
                          <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "opacity-60" : ""}`} />
                          Scan
                        </Button>
                      </td>
                      <td className="p-3"><StatusPill result={hostScanState[h.name]} /></td>
                    </tr>
                  ))}
                  {!loading && filteredHosts.length === 0 && (
                    <tr><td className="p-6 text-center text-slate-500" colSpan={6}>No hosts.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </TabsContent>

          {/* Cards */}
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
                      <Button size="sm" variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={() => handleScanHost(h.name)} disabled={scanning}>
                        <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "opacity-60" : ""}`} />
                        Scan
                      </Button>
                      <StatusPill result={hostScanState[h.name]} />
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
            {!loading && filteredHosts.length === 0 && (
              <div className="text-center py-20 text-slate-400">No hosts.</div>
            )}
          </TabsContent>
        </Tabs>

        {/* Footer */}
        <div className="pt-6 pb-10 text-center text-xs text-slate-500">
          © 2025 PrecisionPlanIT &amp; SoFMeRight (Kai)
        </div>
      </main>
    </div>
  );
}
