// ui/src/App.tsx
import React, { useEffect, useMemo, useState } from "react";
import {
  Boxes, Layers, AlertTriangle, XCircle, Search,
  RefreshCw, ArrowLeft, ChevronRight, ShieldCheck
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";

/* ==================== Types ==================== */

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
  created_ts?: string;   // from API if present
  ip_addr?: string;      // from API if present
  compose_project?: string;
  compose_service?: string;
  stack?: string | null; // legacy
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

/* ==================== Small UI bits ==================== */

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

function StatusPill({ result }: {
  result?: { kind: "ok" | "skipped" | "error"; saved?: number; reason?: string; err?: string }
}) {
  if (!result) return null;
  const base = "px-2 py-0.5 rounded text-xs border";
  if (result.kind === "ok") {
    return <span className={`${base} border-emerald-700/50 bg-emerald-900/30 text-emerald-200`}>
      OK{typeof result.saved === "number" ? ` • saved ${result.saved}` : ""}
    </span>;
  }
  if (result.kind === "skipped") {
    return <span className={`${base} border-amber-700/50 bg-amber-900/30 text-amber-200`}>
      Skipped{result.reason ? ` • ${result.reason}` : ""}
    </span>;
  }
  return <span className={`${base} border-rose-700/50 bg-rose-900/30 text-rose-200`}>
    Error{result.err ? ` • ${result.err}` : ""}
  </span>;
}

function driftBadge(d: "in_sync" | "drift" | "unknown") {
  if (d === "in_sync") return <Badge className="bg-emerald-900/40 border-emerald-700/40 text-emerald-200">In sync</Badge>;
  if (d === "drift") return <Badge variant="destructive">Drift</Badge>;
  return <Badge variant="outline" className="border-slate-700 text-slate-300">Unknown</Badge>;
}

function formatDT(s?: string) {
  if (!s) return "—";
  const d = new Date(s);
  if (isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

/* Ports → one mapping per line */
function formatPortsLines(ports: any): string[] {
  const arr: any[] =
    Array.isArray(ports) ? ports :
      (ports && Array.isArray(ports.ports)) ? ports.ports : [];
  const lines: string[] = [];
  for (const p of arr) {
    const ip = p.IP || p.Ip || p.ip || "";
    const pub = p.PublicPort ?? p.publicPort;
    const priv = p.PrivatePort ?? p.privatePort;
    const typ = (p.Type ?? p.type ?? "").toString().toLowerCase() || "tcp";
    if (priv) {
      const left = pub ? `${ip ? ip + ":" : ""}${pub}` : "";
      lines.push(`${left ? left + " → " : ""}${priv}/${typ}`);
    }
  }
  return lines;
}

/* ==================== Layout: Left Nav ==================== */

function LeftNav({ page, onGoDeployments }: { page: string; onGoDeployments: () => void }) {
  return (
    <div className="hidden md:flex md:flex-col w-60 shrink-0 border-r border-slate-800 bg-slate-950/60">
      {/* Brand moved into the side nav */}
      <div className="px-4 py-4 border-b border-slate-800">
        <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none flex items-center gap-2">
          <span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">DDUI</span>
          <Badge variant="outline">Community</Badge>
        </div>
      </div>

      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">Resources</div>
      <nav className="px-2 pb-4 space-y-1">
        <button
          className={`w-full text-left px-3 py-2 rounded-lg text-sm transition border ${
            page === 'deployments'
              ? 'bg-slate-800/60 border-slate-700 text-white'
              : 'hover:bg-slate-900/40 border-transparent text-slate-300'
          }`}
          onClick={onGoDeployments}
        >
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
        <form method="post" action="/logout">
          <Button type="submit" variant="ghost" className="px-3 text-slate-300 hover:bg-slate-900/60">Logout</Button>
        </form>
      </nav>
    </div>
  );
}

/* ==================== Host Stacks (merge runtime + IaC) ==================== */

type MergedRow = {
  name: string;
  state: string;
  stack: string;
  imageRun?: string;
  imageIac?: string;
  created?: string;
  ip?: string;
  portsText?: string; // newline-separated lines
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

function HostStacksView({ host, onBack, onSync }: { host: Host; onBack: () => void; onSync: ()=>void }) {
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [stacks, setStacks] = useState<MergedStack[]>([]);
  const [hostQuery, setHostQuery] = useState("");

  function matchRow(r: MergedRow, q: string) {
    if (!q) return true;
    const hay = [
      r.name, r.state, r.stack, r.imageRun, r.imageIac, r.ip, r.portsText, r.owner
    ].filter(Boolean).join(" ").toLowerCase();
    return hay.includes(q.toLowerCase());
  }

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

        const rtByStack = new Map<string, ApiContainer[]>();
        for (const c of runtime) {
          const key = (c.compose_project || c.stack || "(none)").trim() || "(none)";
          if (!rtByStack.has(key)) rtByStack.set(key, []);
          rtByStack.get(key)!.push(c);
        }

        const iacByStack = new Map<string, IacStack>();
        for (const s of iacStacks) iacByStack.set(s.name, s);

        const names = new Set<string>([...rtByStack.keys(), ...iacByStack.keys()]);
        const merged: MergedStack[] = [];

        for (const sname of Array.from(names).sort()) {
          const rcs = rtByStack.get(sname) || [];
          const is = iacByStack.get(sname);
          const rows: MergedRow[] = [];

          function desiredImageFor(c: ApiContainer): string | undefined {
            if (!is) return undefined;
            const svc = is.services.find(x =>
              (c.compose_service && x.service_name === c.compose_service) ||
              (x.container_name && x.container_name === c.name)
            );
            return svc?.image || undefined;
          }

          for (const c of rcs) {
            const portsLines = formatPortsLines((c as any).ports);
            const portsText = portsLines.join("\n");
            const desired = desiredImageFor(c);
            const drift = !!(desired && desired.trim() && desired.trim() !== (c.image || "").trim());
            rows.push({
              name: c.name,
              state: c.state,
              stack: sname,
              imageRun: c.image,
              imageIac: desired,
              created: formatDT(c.created_ts),
              ip: c.ip_addr,
              portsText,
              owner: c.owner || "—",
              drift,
            });
          }

          if (is) {
            for (const svc of is.services) {
              const exists = rows.some(r => r.name === (svc.container_name || svc.service_name));
              if (!exists) {
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
            sops: is ? (is.sops_status === "all") : false,
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
          <ArrowLeft className="h-4 w-4 mr-1" /> Back to Deployments
        </Button>
        <div className="ml-2 text-lg font-semibold text-white">
          {host.name} <span className="text-slate-400 text-sm">{host.address || ""}</span>
        </div>
        {/* Sync button directly left of the host search */}
        <div className="ml-auto flex items-center gap-2">
          <Button onClick={onSync} className="bg-[#310937] hover:bg-[#2a0830] text-white">
            <RefreshCw className="h-4 w-4 mr-1" /> Sync
          </Button>
          <div className="relative w-72">
            <Search className="h-4 w-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <Input
              value={hostQuery}
              onChange={(e) => setHostQuery(e.target.value)}
              placeholder={`Search ${host.name}…`}
              className="pl-9 bg-slate-900/50 border-slate-800 text-slate-200 placeholder:text-slate-500"
            />
          </div>
        </div>
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
              <Switch checked={!!s.iacEnabled} disabled />
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <div className="overflow-x-auto rounded-lg border border-slate-800">
              <table className="w-full text-sm table-fixed">
                <thead className="bg-slate-900/70 text-slate-300">
                  <tr>
                    <th className="p-3 text-left w-56">Name</th>
                    <th className="p-3 text-left w-24">State</th>
                    <th className="p-3 text-left w-48">Stack</th>
                    <th className="p-3 text-left w-[28rem]">Image</th>
                    <th className="p-3 text-left w-44">Created</th>
                    <th className="p-3 text-left w-40">IP Address</th>
                    <th className="p-3 text-left w-64">Published Ports</th>
                    <th className="p-3 text-left w-40">Owner</th>
                  </tr>
                </thead>
                <tbody>
                  {(s.rows.filter(r => matchRow(r, hostQuery))).map((r, i) => (
                    <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40">
                      <td className="p-3 font-medium text-slate-200 truncate">{r.name}</td>
                      <td className="p-3 text-slate-300">{r.state}</td>
                      <td className="p-3 text-slate-300 truncate">{r.stack}</td>
                      <td className="p-3 text-slate-300">
                        <div className="flex items-center gap-2">
                          <div className="max-w-[28rem] truncate" title={r.imageRun || ""}>{r.imageRun || "—"}</div>
                          {r.imageIac && (
                            <>
                              <ChevronRight className="h-4 w-4 text-slate-500" />
                              <div
                                className={`max-w-[28rem] truncate ${r.drift ? "text-amber-300" : "text-slate-300"}`}
                                title={r.imageIac}
                              >
                                {r.imageIac}
                              </div>
                            </>
                          )}
                        </div>
                      </td>
                      <td className="p-3 text-slate-300">{r.created || "—"}</td>
                      <td className="p-3 text-slate-300">{r.ip || "—"}</td>
                      <td className="p-3 text-slate-300 align-top w-64">
                        <div className="max-w-64 whitespace-pre-line leading-tight">
                          {r.portsText || "—"}
                        </div>
                      </td>
                      <td className="p-3 text-slate-300">{r.owner || "—"}</td>
                    </tr>
                  ))}
                  {(!s.rows || s.rows.filter(r => matchRow(r, hostQuery)).length === 0) && (
                    <tr><td className="p-4 text-slate-500" colSpan={8}>No containers or services.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      ))}

      {/* Security footnote */}
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

/* ==================== Main App ==================== */

export default function App() {
  const [query, setQuery] = useState("");
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [hostScanState, setHostScanState] = useState<Record<string, {
    kind: "ok" | "skipped" | "error"; saved?: number; reason?: string; err?: string
  }>>({});

  // Host -> { stacks, containers, drift, errors }
  const [metricsCache, setMetricsCache] = useState<
    Record<string, { stacks: number; containers: number; drift: number; errors: number }>
  >({});

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
        const mapped: Host[] = items.map((h: any) => ({
          name: h.name, address: h.addr ?? h.address ?? "", groups: h.groups ?? []
        }));
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

  // "healthy" states; everything else counts as error for the top card
  const OK_STATES = new Set(["running", "created", "restarting", "healthy", "up"]);
  function isBadState(state?: string) {
    const s = (state || "").toLowerCase();
    if (!s) return false;
    for (const ok of OK_STATES) if (s.includes(ok)) return false;
    return true;
  }

  function computeHostMetrics(runtime: ApiContainer[], iac: IacStack[]) {
    const rtByStack = new Map<string, ApiContainer[]>();
    for (const c of runtime) {
      const key = (c.compose_project || c.stack || "(none)").trim() || "(none)";
      if (!rtByStack.has(key)) rtByStack.set(key, []);
      rtByStack.get(key)!.push(c);
    }
    const iacByName = new Map<string, IacStack>();
    for (const s of iac) iacByName.set(s.name, s);

    const names = new Set<string>([...rtByStack.keys(), ...iacByName.keys()]);

    let stacks = 0;
    let containers = runtime.length;
    let drift = 0;
    let errors = 0;

    for (const c of runtime) if (isBadState(c.state)) errors++;

    for (const sname of names) {
      stacks++;
      const rcs = rtByStack.get(sname) || [];
      const is = iacByName.get(sname);
      let stackDrift = false;

      const desiredImageFor = (c: ApiContainer): string | undefined => {
        if (!is) return undefined;
        const svc = is.services.find(x =>
          (c.compose_service && x.service_name === c.compose_service) ||
          (x.container_name && x.container_name === c.name)
        );
        return svc?.image || undefined;
      };

      for (const c of rcs) {
        const desired = desiredImageFor(c);
        if (desired && desired.trim() && desired.trim() !== (c.image || "").trim()) {
          stackDrift = true; break;
        }
      }
      if (!stackDrift && is) {
        for (const svc of is.services) {
          const match = rcs.some(c =>
            (c.compose_service && svc.service_name === c.compose_service) ||
            (svc.container_name && c.name === svc.container_name)
          );
          if (!match) { stackDrift = true; break; }
        }
      }
      if (!rcs.length && is && is.services.length > 0) stackDrift = true;
      if (stackDrift) drift++;
    }

    return { stacks, containers, drift, errors };
  }

  async function refreshMetricsForHosts(hostNames: string[]) {
    if (!hostNames.length) return;
    const limit = 4;
    let idx = 0;
    const workers = Array.from({ length: Math.min(limit, hostNames.length) }, () => (async () => {
      while (true) {
        const i = idx++; if (i >= hostNames.length) break;
        const name = hostNames[i];
        try {
          const [rc, ri] = await Promise.all([
            fetch(`/api/hosts/${encodeURIComponent(name)}/containers`, { credentials: "include" }),
            fetch(`/api/hosts/${encodeURIComponent(name)}/iac`, { credentials: "include" }),
          ]);
          if (rc.status === 401 || ri.status === 401) { window.location.href = "/login"; return; }
          const contJson = await rc.json();
          const iacJson = await ri.json();
          const runtime: ApiContainer[] = (contJson.items || []) as ApiContainer[];
          const iacStacks: IacStack[] = (iacJson.stacks || []) as IacStack[];
          const m = computeHostMetrics(runtime, iacStacks);
          setMetricsCache(prev => ({ ...prev, [name]: m }));
        } catch {
          // ignore per-host metrics errors
        }
      }
    })());
    await Promise.all(workers);
  }

  useEffect(() => {
    if (!hosts.length) return;
    refreshMetricsForHosts(hosts.map(h => h.name));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hosts]);

  const metrics = useMemo(() => {
    let stacks = 0, containers = 0, drift = 0, errors = 0;
    for (const h of filteredHosts) {
      const m = metricsCache[h.name];
      if (!m) continue;
      stacks += m.stacks;
      containers += m.containers;
      drift += m.drift;
      errors += m.errors;
    }
    return { hosts: filteredHosts.length, stacks, containers, drift, errors };
  }, [filteredHosts, metricsCache]);

  async function handleScanAll() {
    if (scanning) return;
    setScanning(true);
    try {
      await fetch("/api/iac/scan", { method: "POST", credentials: "include" }).catch(()=>{});
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
      await refreshMetricsForHosts(hosts.map(h => h.name));
    } finally {
      setScanning(false);
    }
  }

  function openHost(name: string) {
    const h = hosts.find(x => x.name === name) || { name };
    setActiveHost(h as Host);
    setPage("host");
    refreshMetricsForHosts([name]);
  }

  return (
    <div className="min-h-screen flex">
      <LeftNav page={page} onGoDeployments={() => setPage("deployments")} />

      {/* Full-width main content (no right-side top bar) */}
      <div className="flex-1 min-w-0">
        <main className="px-6 py-6 space-y-6">
          {/* Metrics (full width grid) */}
          <div className="grid md:grid-cols-5 gap-4">
            <MetricCard title="Hosts" value={metrics.hosts} icon={Boxes} accent />
            <MetricCard title="Stacks" value={metrics.stacks} icon={Boxes} />
            <MetricCard title="Containers" value={metrics.containers} icon={Layers} />
            <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
            <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
          </div>

          {/* Deployments (hosts list) */}
          {page === 'deployments' && (
            <div className="space-y-4">
              <Card className="bg-slate-900/40 border-slate-800">
                <CardContent className="py-4">
                  {/* Sync button directly left of global search */}
                  <div className="flex items-center gap-2">
                    <Button onClick={handleScanAll} disabled={scanning} className="bg-[#310937] hover:bg-[#2a0830] text-white">
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
                          <button className="hover:underline" onClick={() => openHost(h.name)}>
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
                            onClick={async () => {
                              if (scanning) return;
                              setScanning(true);
                              try {
                                await fetch(`/api/scan/host/${encodeURIComponent(h.name)}`, { method: "POST", credentials: "include" });
                                setHostScanState(prev => ({ ...prev, [h.name]: { kind: "ok" } }));
                                await refreshMetricsForHosts([h.name]);
                              } finally {
                                setScanning(false);
                              }
                            }}
                            disabled={scanning}
                          >
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
            <HostStacksView host={activeHost} onBack={() => setPage('deployments')} onSync={handleScanAll} />
          )}

          <div className="pt-6 pb-10 text-center text-xs text-slate-500">
            © 2025 PrecisionPlanIT &amp; SoFMeRight (Kai)
          </div>
        </main>
      </div>
    </div>
  );
}
