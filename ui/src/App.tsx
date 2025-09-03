// ui/src/App.tsx
import React, { useEffect, useMemo, useState } from "react";
import {
  Boxes, Layers, AlertTriangle, XCircle, Search, PanelLeft,
  RefreshCw, PlayCircle, Rocket, ShieldCheck, ChevronRight, ArrowLeft
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";

// ---------- Types ----------

type Host = {
  name: string;
  address?: string;
  groups?: string[];
};

type ApiContainer = {
  name: string;
  image: string;
  state: string;
  status: string;
  owner?: string;
  ports?: any;
  labels?: Record<string, string>;
  updated_at?: string;
  created_ts?: string;
  ip_addr?: string;
  compose_project?: string;
  compose_service?: string;
  stack?: string | null; // legacy field from older API versions
};

type IacEnvFile = { path: string; sops: boolean };

type IacService = {
  id: number;
  stack_id: number;
  service_name: string;
  container_name?: string;
  image?: string;
  labels: Record<string, string>;
  env_keys: string[];
  env_files: IacEnvFile[];
  ports: any[];
  volumes: any[];
  deploy: Record<string, any>;
};

type IacStack = {
  id: number;
  name: string; // stack_name
  scope_kind: string;
  scope_name: string;
  deploy_kind: "compose" | "script" | "unmanaged" | string;
  pull_policy?: string;
  sops_status: "all" | "partial" | "none" | string;
  iac_enabled: boolean;
  rel_path: string;
  compose?: string;
  services: IacService[];
};

// ---------- Small UI bits ----------

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

function driftBadge(d: "in_sync" | "drift" | "unknown") {
  if (d === "in_sync")   return <Badge className="bg-emerald-900/40 border-emerald-700/40 text-emerald-200">In sync</Badge>;
  if (d === "drift")     return <Badge variant="destructive">Drift</Badge>;
  return <Badge variant="outline" className="border-slate-700 text-slate-300">Unknown</Badge>;
}

function formatDT(s?: string) {
  if (!s) return "—";
  const d = new Date(s);
  if (isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

function formatPorts(ports: any): { text: string; ip?: string } {
  const arr: any[] = Array.isArray(ports) ? ports : (ports && Array.isArray(ports.ports)) ? ports.ports : [];
  if (!arr.length) return { text: "—" };
  const chunks: string[] = [];
  let firstIP: string | undefined;
  for (const p of arr) {
    const ip = p.IP || p.Ip || p.ip || "";
    const pub = p.PublicPort ?? p.publicPort;
    const priv = p.PrivatePort ?? p.privatePort;
    const typ = (p.Type ?? p.type ?? "").toString().toLowerCase() || "tcp";
    if (!firstIP && ip) firstIP = ip;
    if (pub && priv) chunks.push(`${ip ? ip + ":" : ""}${pub} → ${priv}/${typ}`);
    else if (priv) chunks.push(`${priv}/${typ}`);
  }
  return { text: chunks.join(", "), ip: firstIP };
}

// ---------- Layout: Left Nav ----------

function LeftNav({ page, onGoDeployments }: { page: string; onGoDeployments: ()=>void }) {
  return (
    <div className="hidden md:flex md:flex-col w-60 shrink-0 border-r border-slate-800 bg-slate-950/60">
      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">Resources</div>
      <nav className="px-2 pb-4 space-y-1">
        <button className={`w-full text-left px-3 py-2 rounded-lg text-sm transition border ${page==='deployments' ? 'bg-slate-800/60 border-slate-700 text-white' : 'hover:bg-slate-900/40 border-transparent text-slate-300'}`} onClick={onGoDeployments}>
          Deployments
        </button>
        <div className="px-3 py-2 text-slate-500 text-sm cursor-not-allowed">Images</div>
        <div className="px-3 py-2 text-slate-500 text-sm cursor-not-allowed">Networks</div>
        <div className="px-3 py-2 text-slate-500 text-sm cursor-not-allowed">Volumes</div>
      </nav>
      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">System</div>
      <nav className="px-2 space-y-1">
        <div className="px-3 py-2 text-slate-300 text-sm">Settings</div>
        <div className="px-3 py-2 text-slate-300 text-sm">About</div>
        <div className="px-3 py-2 text-slate-300 text-sm">Help</div>
        <a className="px-3 py-2 text-slate-300 text-sm hover:underline" href="/logout">Logout</a>
      </nav>
    </div>
  );
}

// ---------- Host Stacks (merge runtime + IaC) ----------

type MergedRow = {
  name: string;        // container name (runtime) or service/container_name (iac-only)
  state: string;
  stack: string;
  imageRun?: string;
  imageIac?: string;
  created?: string;
  ip?: string;
  portsText?: string;
  owner?: string;
  drift?: boolean;
};

type MergedStack = {
  name: string;
  drift: "in_sync" | "drift" | "unknown";
  iacEnabled: boolean;
  pullPolicy?: string;
  sops?: boolean;
  deployKind: string;
  rows: MergedRow[];
};

function HostStacksView({ host, onBack }: { host: Host; onBack: ()=>void }) {
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string|null>(null);
  const [stacks, setStacks] = useState<MergedStack[]>([]);

  useEffect(() => {
    let cancel = false;
    (async () => {
      setLoading(true); setErr(null);
      try {
        const [rc, ri] = await Promise.all([
          fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers`, { credentials: "include" }),
          fetch(`/api/hosts/${encodeURIComponent(host.name)}/iac`, { credentials: "include" }),
        ]);
        if (rc.status === 401 || ri.status === 401) { window.location.href = "/login"; return; }
        const contJson = await rc.json();
        const iacJson = await ri.json();
        const runtime: ApiContainer[] = (contJson.items || []) as ApiContainer[];
        const iacStacks: IacStack[] = (iacJson.stacks || []) as IacStack[];

        // Group runtime by stack name (compose project) with fallback to label/stack/null
        const rtByStack = new Map<string, ApiContainer[]>();
        for (const c of runtime) {
          const key = (c.compose_project || c.stack || "(none)").trim() || "(none)";
          if (!rtByStack.has(key)) rtByStack.set(key, []);
          rtByStack.get(key)!.push(c);
        }

        // Index IaC stacks by name
        const iacByStack = new Map<string, IacStack>();
        for (const s of iacStacks) iacByStack.set(s.name, s);

        // Union of stack names
        const names = new Set<string>([...rtByStack.keys(), ...iacByStack.keys()]);
        const merged: MergedStack[] = [];

        for (const sname of Array.from(names).sort()) {
          const rcs = rtByStack.get(sname) || [];
          const is = iacByStack.get(sname);
          const rows: MergedRow[] = [];

          // helper to find desired image for a runtime container
          function matchDesiredImage(c: ApiContainer): string|undefined {
            if (!is) return undefined;
            // Prefer compose_service match, then container_name
            const svc = is.services.find(x =>
              (c.compose_service && x.service_name === c.compose_service) ||
              (x.container_name && x.container_name === c.name)
            );
            return svc?.image || undefined;
          }

          for (const c of rcs) {
            const { text: portsText, ip } = formatPorts((c as any).ports);
            const desired = matchDesiredImage(c);
            const drift = !!(desired && desired.trim() && desired.trim() !== (c.image||"").trim());
            rows.push({
              name: c.name,
              state: c.state,
              stack: sname,
              imageRun: c.image,
              imageIac: desired,
              created: formatDT(c.created_ts),
              ip: c.ip_addr || ip,
              portsText,
              owner: c.owner || "—",
              drift,
            });
          }

          // Add IaC-only rows for desired services that are not running
          if (is) {
            for (const svc of is.services) {
              const found = rows.some(r =>
                r.name === svc.container_name || r.name === svc.service_name
              );
              if (!found) {
                rows.push({
                  name: svc.container_name || svc.service_name,
                  state: "missing",
                  stack: sname,
                  imageRun: undefined,
                  imageIac: svc.image,
                  created: "—",
                  ip: "—",
                  portsText: "—",
                  owner: "—",
                  drift: true,
                });
              }
            }
          }

          const stackDrift = rows.some(r => r.drift) ? "drift" : (is ? "in_sync" : (rcs.length ? "unknown" : "unknown"));
          merged.push({
            name: sname,
            drift: stackDrift,
            iacEnabled: is ? is.iac_enabled : false,
            pullPolicy: is?.pull_policy,
            sops: is ? (is.sops_status === 'all') : false,
            deployKind: is?.deploy_kind || (sname === "(none)" ? "unmanaged" : "compose"),
            rows,
          });
        }

        if (!cancel) setStacks(merged);
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load host stacks");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
  }, [host.name]);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={onBack}>
          <ArrowLeft className="h-4 w-4 mr-1"/> Back to Deployments
        </Button>
        <div className="ml-2 text-lg font-semibold text-white">{host.name} <span className="text-slate-400 text-sm">{host.address || ""}</span></div>
      </div>

      {loading && <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">Loading stacks…</div>}
      {err && <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">Error: {err}</div>}

      {stacks.map((s, idx) => (
        <Card key={`${host.name}:${s.name}:${idx}`} className="bg-slate-900/50 border-slate-800 rounded-xl">
          <CardHeader className="pb-2 flex flex-row items-center justify-between">
            <div className="space-y-1">
              <CardTitle className="text-xl text-white">{s.name}</CardTitle>
              <div className="flex items-center gap-2">
                {driftBadge(s.drift)}
                <Badge variant="outline" className="border-slate-700 text-slate-300">{s.deployKind || "unknown"}</Badge>
                <Badge variant="outline" className="border-slate-700 text-slate-300">pull: {s.pullPolicy || "—"}</Badge>
                {s.sops ? (
                  <Badge className="bg-indigo-900/40 border-indigo-700/40 text-indigo-200">SOPS</Badge>
                ) : (
                  <Badge variant="outline" className="border-slate-700 text-slate-300">no SOPS</Badge>
                )}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-slate-300">IaC enabled</span>
              <Switch checked={!!s.iacEnabled} disabled/>
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <div className="overflow-x-auto rounded-lg border border-slate-800">
              <table className="w-full text-sm">
                <thead className="bg-slate-900/70 text-slate-300">
                  <tr>
                    <th className="p-3 text-left">Name</th>
                    <th className="p-3 text-left">State</th>
                    <th className="p-3 text-left">Image</th>
                    <th className="p-3 text-left">Created</th>
                    <th className="p-3 text-left">IP Address</th>
                    <th className="p-3 text-left">Published Ports</th>
                    <th className="p-3 text-left">Owner</th>
                  </tr>
                </thead>
                <tbody>
                  {s.rows.map((r, i) => (
                    <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40">
                      <td className="p-3 font-medium text-slate-200">{r.name}</td>
                      <td className="p-3 text-slate-300">{r.state}</td>
                      <td className="p-3 text-slate-300">
                        <div className="flex items-center gap-2">
                          <div className="max-w-[36ch] truncate" title={r.imageRun || ""}>{r.imageRun || "—"}</div>
                          {r.imageIac && (
                            <>
                              <ChevronRight className="h-4 w-4 text-slate-500" />
                              <div className={`max-w-[36ch] truncate ${r.drift ? 'text-amber-300' : 'text-slate-300'}`} title={r.imageIac}>{r.imageIac}</div>
                            </>
                          )}
                        </div>
                      </td>
                      <td className="p-3 text-slate-300">{r.created || "—"}</td>
                      <td className="p-3 text-slate-300">{r.ip || "—"}</td>
                      <td className="p-3 text-slate-300">{r.portsText || "—"}</td>
                      <td className="p-3 text-slate-300">{r.owner || "—"}</td>
                    </tr>
                  ))}
                  {(!s.rows || s.rows.length === 0) && (
                    <tr><td className="p-4 text-slate-500" colSpan={7}>No containers or services.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      ))}

      {/* security section */}
      <Card className="bg-slate-900/40 border-slate-800">
        <CardContent className="py-4 flex flex-wrap items-center gap-3 text-sm text-slate-300">
          <ShieldCheck className="h-4 w-4" /> Security by default:
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">AGE key never persisted</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Decrypt to tmpfs only</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
        </CardContent>
      </Card>
    </div>
  );
}

// ---------- Main App ----------

export default function App() {
  const [query, setQuery] = useState("");
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [hostScanState, setHostScanState] = useState<Record<string, { kind: "ok" | "skipped" | "error"; saved?: number; reason?: string; err?: string }>>({});

  // page routing
  const [page, setPage] = useState<"deployments" | "host">("deployments");
  const [activeHost, setActiveHost] = useState<Host | null>(null);

  useEffect(() => {
    let cancel = false;
    (async () => {
      setLoading(true); setErr(null);
      try {
        const r = await fetch("/api/hosts", { credentials: "include" });
        if (r.status === 401) { window.location.href = "/login"; return; }
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = await r.json();
        const items = Array.isArray(data.items) ? data.items : [];
        const mapped: Host[] = items.map((h: any) => ({ name: h.name, address: h.addr ?? h.address ?? "", groups: h.groups ?? [] }));
        if (!cancel) setHosts(mapped);
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load hosts");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
  }, []);

  const filteredHosts = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter(h => [h.name, h.address || "", ...(h.groups || [])].join(" ").toLowerCase().includes(q));
  }, [hosts, query]);

  const metrics = useMemo(() => {
    const hostCount = filteredHosts.length;
    return { stacks: "—", containers: "—", drift: 0, errors: 0, hostCount };
  }, [filteredHosts]);

  async function handleSyncAll() {
    if (scanning) return;
    setScanning(true);
    try {
      // Scan IaC first, then runtime hosts
      await fetch("/api/iac/scan", { method: "POST", credentials: "include" });
      const res = await fetch("/api/scan/all", { method: "POST", credentials: "include" });
      if (res.status === 401) { window.location.href = "/login"; return; }
      const data = await res.json();
      const map: Record<string, { kind: "ok" | "skipped" | "error"; saved?: number; reason?: string; err?: string }> = {};
      for (const r of data.results || []) {
        if (r.skipped) map[r.host] = { kind: "skipped", reason: r.reason };
        else if (r.err) map[r.host] = { kind: "error", err: r.err };
        else map[r.host] = { kind: "ok", saved: r.saved ?? 0 };
      }
      setHostScanState(prev => ({ ...prev, ...map }));
    } finally {
      setScanning(false);
    }
  }

  function openHost(name: string) {
    const h = hosts.find(x => x.name === name) || { name };
    setActiveHost(h as Host);
    setPage("host");
  }

  return (
    <div className="min-h-screen flex">
      <LeftNav page={page} onGoDeployments={() => setPage("deployments")} />

      <div className="flex-1 min-w-0">
        {/* Top bar */}
        <div className="bg-slate-950 border-b border-slate-800">
          <div className="max-w-7xl mx-auto px-4 py-3 flex items-center gap-3">
            <Button variant="ghost" size="icon"><PanelLeft className="h-5 w-5 text-slate-300" /></Button>
            <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none flex items-center gap-2">
              DD<span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">UI</span>
              <Badge variant="outline">Community</Badge>
            </div>
            <Separator orientation="vertical" className="mx-2 h-8 bg-slate-800" />
            <div className="ml-auto flex items-center gap-2">
              <Button onClick={handleSyncAll} disabled={scanning} className="bg-[#310937] hover:bg-[#2a0830] text-white">
                <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "animate-spin" : ""}`} />
                {scanning ? "Syncing…" : "Sync"}
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
          {/* Metrics */}
          <div className="grid md:grid-cols-4 gap-4">
            <MetricCard title="Hosts" value={metrics.hostCount} icon={Boxes} accent />
            <MetricCard title="Stacks" value={metrics.stacks} icon={Layers} accent />
            <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
            <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
          </div>

          {/* Conditional views */}
          {page === 'deployments' && (
            <div className="space-y-4">
              <Card className="bg-slate-900/40 border-slate-800">
                <CardContent className="py-4">
                  <div className="flex flex-col md:flex-row gap-3 md:items-center md:justify-between">
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
                    {loading && (
                      <tr><td className="p-4 text-slate-500" colSpan={5}>Loading hosts…</td></tr>
                    )}
                    {err && !loading && (
                      <tr><td className="p-4 text-rose-300" colSpan={5}>{err}</td></tr>
                    )}
                    {!loading && filteredHosts.map((h) => (
                      <tr key={h.name} className="border-t border-slate-800 hover:bg-slate-900/40">
                        <td className="p-3 font-medium text-slate-200">
                          <button className="hover:underline" onClick={() => openHost(h.name)}>{h.name}</button>
                        </td>
                        <td className="p-3 text-slate-300">{h.address || "—"}</td>
                        <td className="p-3 text-slate-300">{(h.groups || []).length ? (h.groups || []).join(", ") : "—"}</td>
                        <td className="p-3">
                          <Button size="sm" variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={async () => {
                            if (scanning) return;
                            setScanning(true);
                            try {
                              await fetch(`/api/scan/host/${encodeURIComponent(h.name)}`, { method: 'POST', credentials: 'include' });
                              setHostScanState(prev => ({ ...prev, [h.name]: { kind: 'ok' } }));
                            } finally {
                              setScanning(false);
                            }
                          }} disabled={scanning}>
                            <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "opacity-60" : ""}`} />
                            Scan
                          </Button>
                        </td>
                        <td className="p-3"><StatusPill result={hostScanState[h.name]} /></td>
                      </tr>
                    ))}
                    {!loading && filteredHosts.length === 0 && (
                      <tr><td className="p-6 text-center text-slate-500" colSpan={5}>No hosts.</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {page === 'host' && activeHost && (
            <HostStacksView host={activeHost} onBack={() => setPage('deployments')} />
          )}

          {/* Footer */}
          <div className="pt-6 pb-10 text-center text-xs text-slate-500">
            © 2025 PrecisionPlanIT &amp; SoFMeRight (Kai)
          </div>
        </main>
      </div>
    </div>
  );
}
