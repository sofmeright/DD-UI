// ui/src/App.tsx
import React, { useEffect, useMemo, useState } from "react";
import {
  Boxes, Layers, AlertTriangle, XCircle, Search, RefreshCw, ArrowLeft,
  ChevronRight, ShieldCheck, Eye, EyeOff, FileText, Trash2, Plus, Save
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";

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
  created_ts?: string;
  ip_addr?: string;
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
  services: IacService[] | null; // can be null on brand-new stacks
};

type IacFileMeta = {
  role: string;
  rel_path: string;
  sops: boolean;
  sha256_hex: string;
  size_bytes: number;
  updated_at: string;
};

type InspectOut = {
  id: string;
  name: string;
  image: string;
  state: string;
  health?: string;
  created: string;
  cmd?: string[];
  entrypoint?: string[];
  env?: Record<string, string>;
  labels?: Record<string, string>;
  restart_policy?: string;
  ports?: { published?: string; target?: string; protocol?: string }[];
  volumes?: { source?: string; target?: string; mode?: string; rw?: boolean }[];
  networks?: string[];
};

type SessionResp = {
  user: null | {
    sub: string;
    email: string;
    name: string;
    picture?: string;
  };
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

/* Portainer-style oblong status pill */
function HealthPill({ state, health }: { state?: string; health?: string }) {
  const s = (state || "").toLowerCase();
  const h = (health || "").toLowerCase();
  let cls = "bg-slate-900/40 border-slate-700 text-slate-300";
  let text = (h || s || "unknown").split(" ")[0];

  if (h === "healthy") {
    cls = "bg-emerald-900/35 border-emerald-700/60 text-emerald-200";
    text = "healthy";
  } else if (h === "unhealthy") {
    cls = "bg-rose-900/35 border-rose-700/60 text-rose-200";
    text = "unhealthy";
  } else if (s.includes("running") || s.includes("up")) {
    cls = "bg-emerald-900/25 border-emerald-700/50 text-emerald-200";
    text = "running";
  } else if (s.includes("restarting")) {
    cls = "bg-amber-900/30 border-amber-700/50 text-amber-200";
    text = "restarting";
  } else if (s.includes("paused")) {
    cls = "bg-sky-900/30 border-sky-700/50 text-sky-200";
    text = "paused";
  } else if (s.includes("exited") || s.includes("dead")) {
    cls = "bg-rose-900/30 border-rose-700/50 text-rose-200";
    text = s.includes("dead") ? "dead" : "exited";
  }

  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold border ${cls}`}>
      {text}
    </span>
  );
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
          <img src="/DDUI-Logo.png" alt="DDUI" className="h-5 w-5 rounded-sm" />
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
  iacId?: number;
};

function safeServices(s?: IacService[] | null): IacService[] {
  return Array.isArray(s) ? s : [];
}

function HostStacksView({
  host, onBack, onSync, onOpenStack,
}: { host: Host; onBack: () => void; onSync: ()=>void; onOpenStack: (stackName: string, iacId?: number)=>void }) {
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
        if (rc.status === 401 || ri.status === 401) { window.location.replace("/auth/login"); return; }
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
            const svcs = safeServices(is.services);
            const svc = svcs.find(x =>
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
            const svcs = safeServices(is.services);
            for (const svc of svcs) {
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

          const stackDrift =
            rows.some(r => r.drift)
              ? "drift"
              : (is ? "in_sync" : (rcs.length ? "unknown" : "unknown"));

          merged.push({
            name: sname,
            drift: stackDrift,
            iacEnabled: !!is?.iac_enabled,
            pullPolicy: is?.pull_policy,
            sops: is ? (is.sops_status === "all") : false,
            deployKind: is?.deploy_kind || (sname === "(none)" ? "unmanaged" : "compose"),
            rows,
            iacId: is?.id,
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

  async function createStack() {
    const name = (window.prompt("New stack name (letters, numbers, dashes only):") || "").trim();
    if (!name) return;
    if (!/^[a-zA-Z0-9_-]+$/.test(name)) { window.alert("Invalid name."); return; }
    if (stacks.some(s => s.name === name)) { window.alert("A stack with that name already exists."); return; }

    try {
      const r = await fetch(`/api/iac/stacks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ scope_kind: "host", scope_name: host.name, stack_name: name }),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      const j = await r.json();
      onOpenStack(name, j.id);
    } catch (e: any) {
      window.alert(e?.message || "Failed to create stack");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={onBack}>
          <ArrowLeft className="h-4 w-4 mr-1" /> Back to Deployments
        </Button>
        <div className="ml-2 text-lg font-semibold text-white">
          {host.name} <span className="text-slate-400 text-sm">{host.address || ""}</span>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Button onClick={onSync} className="bg-[#310937] hover:bg-[#2a0830] text-white">
            <RefreshCw className="h-4 w-4 mr-1" /> Sync
          </Button>
          <Button variant="outline" className="border-slate-700" onClick={createStack}>
            <Plus className="h-4 w-4 mr-1" /> New Stack
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
              <CardTitle className="text-xl text-white">
                <button className="hover:underline" onClick={() => onOpenStack(s.name, s.iacId)}>
                  {s.name}
                </button>
              </CardTitle>
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
                    <th className="p-3 text-left w-64">Name</th>
                    <th className="p-3 text-left w-44">State</th>
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
                      <td className="p-3"><HealthPill state={r.state} /></td>
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
                    <tr><td className="p-4 text-slate-500" colSpan={7}>No containers or services.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
            <div className="pt-2 text-xs text-slate-400">
              Tip: click the stack title to open the full compare & editor view.
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

/* ==================== Stack Detail Compare & Editor ==================== */

function EnvRow({ k, v }: { k: string; v: string }) {
  const [show, setShow] = useState(false);
  const masked = v ? "•".repeat(Math.min(v.length, 24)) : "";
  return (
    <div className="flex items-center justify-between gap-2 py-1">
      <div className="text-slate-300 text-sm">{k}</div>
      <div className="flex items-center gap-2">
        <div className="text-slate-400 text-sm font-mono">{show ? v || "" : masked}</div>
        <Button size="icon" variant="ghost" className="h-7 w-7" onClick={() => setShow(s => !s)} title={show ? "Hide" : "Reveal"}>
          {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}

function KVList({ title, items }: { title: string; items: Record<string, string> | undefined }) {
  const keys = Object.keys(items || {}).sort();
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">{title}</div>
      {keys.length === 0 && <div className="text-sm text-slate-500">No {title.toLowerCase()}.</div>}
      <div className="space-y-1">
        {keys.map(k => (
          <div className="flex items-center justify-between gap-2 text-sm" key={k}>
            <div className="text-slate-300">{k}</div>
            <div className="text-slate-400 font-mono truncate max-w-[22rem]" title={items![k]}>{items![k]}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

function PortsBlock({ ports }: { ports?: InspectOut["ports"] }) {
  const list = ports || [];
  if (!list.length) return <div className="text-sm text-slate-500">No port bindings.</div>;
  return (
    <div className="space-y-1 text-sm">
      {list.map((p, i) => (
        <div key={i} className="text-slate-300">
          {(p.published ? p.published + " → " : "")}{p.target}{p.protocol ? "/" + p.protocol : ""}
        </div>
      ))}
    </div>
  );
}

function VolsBlock({ vols }: { vols?: InspectOut["volumes"] }) {
  const list = vols || [];
  if (!list.length) return <div className="text-sm text-slate-500">No mounts.</div>;
  return (
    <div className="space-y-1 text-sm">
      {list.map((m, i) => (
        <div key={i} className="text-slate-300">
          <span className="font-mono">{m.source}</span> → <span className="font-mono">{m.target}</span>
          {m.mode ? ` (${m.mode}${m.rw === false ? ", ro" : ""})` : (m.rw === false ? " (ro)" : "")}
        </div>
      ))}
    </div>
  );
}

function MiniEditor({
  id, initialPath, stackId, refresh,
}: { id: string; initialPath: string; stackId: number; refresh: ()=>void }) {
  const [path, setPath] = useState(initialPath);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [sops, setSops] = useState(false);
  const [decryptView, setDecryptView] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // ⬇️ allow switching between file presets while editor is open
  useEffect(() => {
    setPath(initialPath);
    setContent("");
    setSops(false);
    setDecryptView(false);
    setErr(null);
  }, [initialPath]);

  async function loadFile(decrypt: boolean) {
    setLoading(true); setErr(null);
    try {
      const url = `/api/iac/stacks/${stackId}/file?path=${encodeURIComponent(path)}${decrypt ? "&decrypt=1" : ""}`;
      const r = await fetch(url, {
        credentials: "include",
        headers: decrypt ? { "X-Confirm-Reveal": "yes" } : undefined,
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      const txt = await r.text();
      setContent(txt);
    } catch (e: any) {
      setErr(e?.message || "Failed to load");
    } finally {
      setLoading(false);
    }
  }

  async function saveFile() {
    setLoading(true); setErr(null);
    try {
      const r = await fetch(`/api/iac/stacks/${stackId}/file`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path, content, sops }),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      refresh();
    } catch (e: any) {
      setErr(e?.message || "Failed to save");
    } finally {
      setLoading(false);
    }
  }

  return (
    <Card className="bg-slate-900/40 border-slate-800">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm text-slate-200">Editor</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex gap-2">
          <Input value={path} onChange={e => setPath(e.target.value)} placeholder="docker-compose/host/stack/compose.yaml" />
          <Button onClick={() => loadFile(false)} variant="outline" className="border-slate-700">Open</Button>
          <Button onClick={() => { setDecryptView(true); loadFile(true); }} variant="outline" className="border-indigo-700 text-indigo-200">Reveal SOPS</Button>
        </div>
        {err && <div className="text-xs text-rose-300">Error: {err}</div>}
        {decryptView && <div className="text-xs text-amber-300">Warning: Decrypted secrets are visible in your browser until you navigate away.</div>}
        <textarea
          id={id}
          className="w-full min-h-[220px] text-sm bg-slate-950/50 border border-slate-800 rounded p-2 font-mono text-slate-200"
          value={content}
          onChange={e => setContent(e.target.value)}
          placeholder={loading ? "Loading…" : "File content…"}
        />
        <div className="flex items-center justify-between">
          <label className="text-sm text-slate-300 inline-flex items-center gap-2">
            <input type="checkbox" checked={sops} onChange={e => setSops(e.target.checked)} />
            Mark as SOPS file
          </label>
          <Button onClick={saveFile} disabled={loading}><Save className="h-4 w-4 mr-1" /> Save</Button>
        </div>
      </CardContent>
    </Card>
  );
}

function StackDetailView({
  host, stackName, iacId, onBack,
}: { host: Host; stackName: string; iacId?: number; onBack: ()=>void }) {
  const [runtime, setRuntime] = useState<ApiContainer[]>([]);
  const [containers, setContainers] = useState<InspectOut[]>([]);
  const [files, setFiles] = useState<IacFileMeta[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [editPath, setEditPath] = useState<string | null>(null);

  // keep an internal id so we can create IaC without bouncing the page
  const [localIacId, setLocalIacId] = useState<number | undefined>(iacId);
  useEffect(() => { setLocalIacId(iacId); }, [iacId]);

  async function refreshFiles() {
    if (!localIacId) return;
    const r = await fetch(`/api/iac/stacks/${localIacId}/files`, { credentials: "include" });
    if (!r.ok) return;
    const j = await r.json();
    setFiles(j.files || []);
  }

  useEffect(() => {
    let cancel = false;
    (async () => {
      setLoading(true); setErr(null);
      try {
        const rc = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers`, { credentials: "include" });
        if (rc.status === 401) { window.location.replace("/auth/login"); return; }
        const contJson = await rc.json();
        const runtimeAll: ApiContainer[] = (contJson.items || []) as ApiContainer[];
        const my = runtimeAll.filter(c => (c.compose_project || c.stack || "(none)") === stackName);
        if (!cancel) setRuntime(my);

        const ins: InspectOut[] = [];
        for (const c of my) {
          const r = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(c.name)}/inspect`, { credentials: "include" });
          if (!r.ok) continue;
          ins.push(await r.json());
        }
        if (!cancel) setContainers(ins);

        if (localIacId) await refreshFiles();
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load stack");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host.name, stackName, localIacId]);

  async function ensureIacStack() {
    if (localIacId) return;
    try {
      const r = await fetch(`/api/iac/stacks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ scope_kind: "host", scope_name: host.name, stack_name: stackName }),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      const j = await r.json();
      setLocalIacId(j.id);
      setEditPath(`docker-compose/${host.name}/${stackName}/compose.yaml`);
      await refreshFiles();
    } catch (e: any) {
      setErr(e?.message || "Failed to create IaC stack");
    }
  }

  async function deleteFile(path: string) {
    if (!localIacId) return;
    const r = await fetch(`/api/iac/stacks/${localIacId}/file?path=${encodeURIComponent(path)}`, {
      method: "DELETE",
      credentials: "include",
    });
    if (r.ok) refreshFiles();
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={onBack}>
          <ArrowLeft className="h-4 w-4 mr-1" /> Back to {host.name}
        </Button>
        <div className="ml-2 text-lg font-semibold text-white">
          Stack: {stackName}
        </div>
        <div className="ml-auto">
          {!localIacId ? (
            <Button onClick={ensureIacStack} className="bg-[#310937] hover:bg-[#2a0830] text-white">
              <Plus className="h-4 w-4 mr-1" /> Create in IaC
            </Button>
          ) : (
            <Button onClick={refreshFiles} variant="outline" className="border-slate-700">
              <RefreshCw className="h-4 w-4 mr-1" /> Refresh
            </Button>
          )}
        </div>
      </div>

      {loading && <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">Loading…</div>}
      {err && <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">Error: {err}</div>}

      <div className="grid lg:grid-cols-2 gap-4">
        {/* Left: Runtime detail per container (scrolls) */}
        <div className="space-y-4">
          <Card className="bg-slate-900/50 border-slate-800">
            <CardHeader className="pb-2">
              <CardTitle className="text-slate-200 text-lg">Runtime</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {containers.length === 0 && (
                <div className="text-sm text-slate-500">
                  No containers are currently running for this stack on {host.name}.
                </div>
              )}
              {containers.map((c, i) => (
                <div key={i} className="rounded-lg border border-slate-800 p-3">
                  <div className="flex items-center justify-between">
                    <div className="font-medium text-slate-200">{c.name}</div>
                    <HealthPill state={c.state} health={c.health} />
                  </div>
                  <div className="mt-2 grid md:grid-cols-2 gap-3 text-sm">
                    <KVList title="Image" items={{ Image: c.image }} />
                    <div>
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Restart policy</div>
                      <div className="text-slate-300">{c.restart_policy || "—"}</div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">CMD</div>
                      <div className="text-slate-300 font-mono">{(c.cmd || []).join(" ") || "—"}</div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">ENTRYPOINT</div>
                      <div className="text-slate-300 font-mono">{(c.entrypoint || []).join(" ") || "—"}</div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Ports</div>
                      <PortsBlock ports={c.ports} />
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Networks</div>
                      <div className="text-slate-300 text-sm">{(c.networks || []).join(", ") || "—"}</div>
                    </div>
                  </div>
                  <div className="mt-3 grid md:grid-cols-2 gap-3">
                    <div>
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Environment</div>
                      {(!c.env || Object.keys(c.env).length === 0) && <div className="text-sm text-slate-500">No environment variables.</div>}
                      <div className="space-y-1">
                        {Object.entries(c.env || {}).map(([k, v]) => (
                          <EnvRow key={k} k={k} v={v} />
                        ))}
                      </div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Labels</div>
                      <KVList title="Labels" items={c.labels || {}} />
                    </div>
                  </div>
                  <div className="mt-3">
                    <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Volumes</div>
                    <VolsBlock vols={c.volumes} />
                  </div>
                </div>
              ))}
            </CardContent>
          </Card>
        </div>

        {/* Right: IaC Files / Editor (sticky) */}
        <div className="space-y-4 lg:sticky lg:top-4 self-start">
          <Card className="bg-slate-900/50 border-slate-800">
            <CardHeader className="pb-2">
              <CardTitle className="text-slate-200 text-lg">IaC Files</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {!localIacId && (
                <div className="text-sm text-amber-300">
                  This stack is not yet declared in IaC. Create it to start managing compose/env/scripts here.
                </div>
              )}
              {localIacId && (
                <div className="max-h-[78vh] overflow-auto pr-1">
                  <div className="flex items-center justify-between mb-3">
                    <div className="text-slate-300 text-sm">{files.length} file(s)</div>
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        onClick={() => setEditPath(`docker-compose/${host.name}/${stackName}/compose.yaml`)}
                      >
                        <FileText className="h-4 w-4 mr-1" /> New compose
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        className="border-slate-700"
                        onClick={() => setEditPath(`docker-compose/${host.name}/${stackName}/.env`)}
                      >
                        <Plus className="h-4 w-4 mr-1" /> New env
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        className="border-slate-700"
                        onClick={() => setEditPath(`docker-compose/${host.name}/${stackName}/deploy.sh`)}
                      >
                        <Plus className="h-4 w-4 mr-1" /> New script
                      </Button>
                    </div>
                  </div>

                  <div className="rounded-lg border border-slate-800 overflow-hidden">
                    <table className="w-full text-sm">
                      <thead className="bg-slate-900/70 text-slate-300">
                        <tr>
                          <th className="p-2 text-left">Path</th>
                          <th className="p-2 text-left">Role</th>
                          <th className="p-2 text-left">SOPS</th>
                          <th className="p-2 text-left">Size</th>
                          <th className="p-2 text-left">Updated</th>
                          <th className="p-2 text-left">Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {files.map((f, i) => (
                          <tr key={i} className="border-t border-slate-800">
                            <td className="p-2 text-slate-200 font-mono">{f.rel_path}</td>
                            <td className="p-2 text-slate-300">{f.role}</td>
                            <td className="p-2">{f.sops ? <Badge className="bg-indigo-900/40 border-indigo-700/40 text-indigo-200">SOPS</Badge> : "—"}</td>
                            <td className="p-2 text-slate-300">{f.size_bytes}</td>
                            <td className="p-2 text-slate-300">{formatDT(f.updated_at)}</td>
                            <td className="p-2">
                              <div className="flex items-center gap-2">
                                <Button size="sm" variant="outline" className="border-slate-700" onClick={() => setEditPath(f.rel_path)}>
                                  Edit
                                </Button>
                                <Button size="icon" variant="ghost" onClick={() => deleteFile(f.rel_path)} title="Delete">
                                  <Trash2 className="h-4 w-4 text-rose-300" />
                                </Button>
                              </div>
                            </td>
                          </tr>
                        ))}
                        {files.length === 0 && (
                          <tr><td className="p-3 text-slate-500" colSpan={6}>No files yet. Use the buttons above to add compose/env/script.</td></tr>
                        )}
                      </tbody>
                    </table>
                  </div>

                  {editPath && (
                    <MiniEditor
                      key={editPath} // ⬅️ re-mount when switching preset
                      id="stack-editor"
                      initialPath={editPath}
                      stackId={localIacId!}
                      refresh={() => { setEditPath(null); refreshFiles(); }}
                    />
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Explain when nothing exists */}
      {!loading && containers.length === 0 && !localIacId && (
        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-4 text-sm text-slate-300">
            This stack has no running containers on <b>{host.name}</b> and is not declared in IaC yet.
            Use “Create in IaC” to define the desired state (compose/env/scripts) and bring it under management.
          </CardContent>
        </Card>
      )}
    </div>
  );
}

/* ==================== Login Gate (unauthenticated) ==================== */

function LoginGate() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-950">
      <Card className="w-full max-w-sm bg-slate-900/60 border-slate-800">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <img src="/DDUI-Logo.png" alt="DDUI" className="h-5 w-5 rounded-sm" />
            <span className="font-black uppercase tracking-tight leading-none text-slate-200 select-none">
              <span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">DDUI</span>
            </span>
            <Badge variant="outline">Community</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-slate-300 text-sm">
            You’re signed out. Continue to your identity provider to sign in.
          </p>
          <Button
            className="w-full bg-[#310937] hover:bg-[#2a0830] text-white"
            onClick={() => { window.location.replace("/auth/login"); }}
          >
            Continue to Sign in
          </Button>
          <p className="text-xs text-slate-500">
            If you get stuck, ensure your OIDC <code>RedirectURL</code> points back to
            <code> /auth/callback</code> and that cookies aren’t blocked.
          </p>
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

  const [metricsCache, setMetricsCache] = useState<
    Record<string, { stacks: number; containers: number; drift: number; errors: number }>
  >({});

  const [page, setPage] = useState<"deployments" | "host" | "stack">("deployments");
  const [activeHost, setActiveHost] = useState<Host | null>(null);
  const [activeStack, setActiveStack] = useState<{ name: string; iacId?: number } | null>(null);

  // Session gate
  const [sessionChecked, setSessionChecked] = useState(false);
  const [authed, setAuthed] = useState<boolean>(false);

  useEffect(() => {
    let cancel = false;
    (async () => {
      try {
        const r = await fetch("/api/session", { credentials: "include" });
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = (await r.json()) as SessionResp;
        if (!cancel) {
          setAuthed(!!data.user);
        }
      } catch {
        window.location.replace("/auth/login");
        return;
      } finally {
        if (!cancel) setSessionChecked(true);
      }
    })();
    return () => { cancel = true; };
  }, []);

  useEffect(() => {
    if (!authed) return;
    let cancel = false;
    (async () => {
      setLoading(true); setErr(null);
      try {
        const r = await fetch("/api/hosts", { credentials: "include" });
        if (r.status === 401) { window.location.replace("/auth/login"); return; }
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = await r.json();
        const items = Array.isArray(data.items) ? data.items : [];
        const mapped: Host[] = items.map((h: any) => ({
          name: h.name, address: h.addr ?? h.address ?? "", groups: h.groups ?? []
        }));
        setHosts(mapped);
      } catch (e: any) {
        setErr(e?.message || "Failed to load hosts");
      } finally {
        setLoading(false);
      }
    })();
  }, [authed]);

  const filteredHosts = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter(h => [h.name, h.address || "", ...(h.groups || [])].join(" ").toLowerCase().includes(q));
  }, [hosts, query]);

  const hostKey = useMemo(() => hosts.map(h => h.name).sort().join("|"), [hosts]);
  useEffect(() => { setMetricsCache({}); }, [hostKey]);

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
        const svcs = Array.isArray(is.services) ? is.services! : [];
        const svc = svcs.find(x =>
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
        const svcs = Array.isArray(is.services) ? is.services! : [];
        for (const svc of svcs) {
          const match = rcs.some(c =>
            (c.compose_service && svc.service_name === c.compose_service) ||
            (svc.container_name && c.name === svc.container_name)
          );
          if (!match) { stackDrift = true; break; }
        }
      }
      const svcs = Array.isArray(is?.services) ? is!.services! : [];
      if (!rcs.length && is && svcs.length > 0) stackDrift = true;
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
          if (rc.status === 401 || ri.status === 401) { window.location.replace("/auth/login"); return; }
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
    if (!authed || !hosts.length) return;
    refreshMetricsForHosts(hosts.map(h => h.name));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authed, hosts]);

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
      if (res.status === 401) { window.location.replace("/auth/login"); return; }
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
    setActiveStack(null);
    setPage("host");
    refreshMetricsForHosts([name]);
  }

  function openStack(name: string, iacId?: number) {
    if (!activeHost) return;
    setActiveStack({ name, iacId });
    setPage("stack");
  }

  // Show login gate if not authenticated
  if (sessionChecked && !authed) {
    return <LoginGate />;
  }
  if (!sessionChecked) {
    return <div className="min-h-screen bg-slate-950" />;
  }

  return (
    <div className="min-h-screen flex">
      <LeftNav page={page} onGoDeployments={() => setPage("deployments")} />

      {/* Full-width main content */}
      <div className="flex-1 min-w-0">
        <main className="px-6 py-6 space-y-6">
          {/* Metrics */}
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
                        <td className="p-3">
                          {/* per-host status could render here in a follow-up */}
                        </td>
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
            <HostStacksView
              host={activeHost}
              onBack={() => setPage('deployments')}
              onSync={handleScanAll}
              onOpenStack={openStack}
            />
          )}

          {page === 'stack' && activeHost && activeStack && (
            <StackDetailView
              host={activeHost}
              stackName={activeStack.name}
              iacId={activeStack.iacId}
              onBack={() => setPage('host')}
            />
          )}

          <div className="pt-6 pb-10 text-center text-xs text-slate-500">
            © 2025 PrecisionPlanIT &amp; SoFMeRight (Kai)
          </div>
        </main>
      </div>
    </div>
  );
}
