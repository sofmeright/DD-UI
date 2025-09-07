// ui/src/App.tsx
import React, { useEffect, useMemo, useState } from "react";
import {
  Boxes, Layers, AlertTriangle, XCircle, Search, RefreshCw, ArrowLeft,
  ChevronRight, ShieldCheck, Eye, EyeOff, FileText, Trash2, Plus, Save,
  Play, Square, Pause, PlayCircle, RotateCw, ZapOff, Terminal, Activity, Bug,
  Shield, ShieldOff, ChevronUp, ChevronDown
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
  stack?: string | null;
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
  iac_enabled: boolean; // Auto DevOps
  rel_path: string;
  compose?: string;
  services: IacService[] | null | undefined;
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

/* Portainer-style state pill */
function StatePill({ state, health }: { state?: string; health?: string }) {
  const s = (state || "").toLowerCase();
  const h = (health || "").toLowerCase();
  let classes = "border-slate-700 bg-slate-900 text-slate-300";
  let text = state || "unknown";
  if (h === "healthy") {
    classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200";
    text = "healthy";
  } else if (s.includes("running") || s.includes("up")) {
    classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200";
  } else if (s.includes("restarting")) {
    classes = "border-amber-700/60 bg-amber-900/40 text-amber-200";
  } else if (s.includes("paused")) {
    classes = "border-sky-700/60 bg-sky-900/40 text-sky-200";
  } else if (s.includes("exited") || s.includes("dead")) {
    classes = "border-rose-700/60 bg-rose-900/40 text-rose-200";
  }
  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border ${classes}`}>
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

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <div className="shrink-0 text-xs uppercase tracking-wide text-slate-400 w-28">{label}</div>
      <div className="text-slate-300 min-w-0 break-words">{value}</div>
    </div>
  );
}

/* ==================== Left Nav ==================== */

function LeftNav({
  page, onGoDeployments, onGoImages, onGoNetworks, onGoVolumes,
}: { page: string; onGoDeployments: () => void; onGoImages: ()=>void; onGoNetworks: ()=>void; onGoVolumes: ()=>void }) {
  const item = (id: string, label: string, onClick: ()=>void) => (
    <button
      className={`w-full text-left px-3 py-2 rounded-lg text-sm transition border ${
        page === id
          ? 'bg-slate-800/60 border-slate-700 text-white'
          : 'hover:bg-slate-900/40 border-transparent text-slate-300'
      }`}
      onClick={onClick}
    >
      {label}
    </button>
  );
  return (
    <div className="hidden md:flex md:flex-col w-60 shrink-0 border-r border-slate-800 bg-slate-950/60">
      <div className="px-4 py-4 border-b border-slate-800">
        <div className="flex items-center gap-3">
          <img src="/DDUI-Logo.png" alt="DDUI" className="h-16 w-16 rounded-md" />
          <div className="flex flex-col">
            <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none text-lg">
              <span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">DDUI</span>
            </div>
            <Badge variant="outline" className="mt-1 w-fit">Community</Badge>
          </div>
        </div>
      </div>

      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">Resources</div>
      <nav className="px-2 pb-4 space-y-1">
        {item('deployments', 'Deployments', onGoDeployments)}
        {item('images', 'Images', onGoImages)}
        {item('networks', 'Networks', onGoNetworks)}
        {item('volumes', 'Volumes', onGoVolumes)}
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

/* ==================== Host Stacks ==================== */

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
  hasIac: boolean;
  hasContent?: boolean; // NEW: Track if stack has any compose files or services
};

function ActionBtn({
  title, onClick, icon: Icon, disabled=false
}: { title: string; onClick: ()=>void; icon: any; disabled?: boolean }) {
  return (
    <Button size="icon" variant="ghost" className="h-7 w-7" title={title} onClick={onClick} disabled={disabled}>
      <Icon className="h-3 w-3 text-slate-200" />
    </Button>
  );
}

function HostStacksView({
  host, onBack, onSync, onOpenStack,
}: { host: Host; onBack: () => void; onSync: ()=>void; onOpenStack: (stackName: string, iacId?: number)=>void }) {
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [stacks, setStacks] = useState<MergedStack[]>([]);
  const [hostQuery, setHostQuery] = useState("");
  const [logModal, setLogModal] = useState<{ ctr: string; text: string } | null>(null);

  function matchRow(r: MergedRow, q: string) {
    if (!q) return true;
    const hay = [
      r.name, r.state, r.stack, r.imageRun, r.imageIac, r.ip, r.portsText, r.owner
    ].filter(Boolean).join(" ").toLowerCase();
    return hay.includes(q.toLowerCase());
  }

  async function doCtrAction(ctr: string, action: string) {
    try {
      await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(ctr)}/action`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action }),
      });
      onSync();
      // quick optimistic UI: mark restarting/paused/etc locally
      setStacks(prev => prev.map(s => ({
        ...s,
        rows: s.rows.map(r => r.name === ctr
          ? { ...r, state: action === "pause" ? "paused" :
                          action === "unpause" ? "running" :
                          action === "stop" ? "exited" :
                          action === "kill" ? "dead" :
                          action === "remove" ? "removed" :
                          action === "start" ? "running" :
                          action === "restart" ? "restarting" : r.state }
          : r)
      })));
    } catch (e) {
      alert("Action failed");
    }
  }

  async function openLogs(ctr: string) {
    try {
      const r = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(ctr)}/logs?tail=200`, { credentials: "include" });
      const txt = await r.text();
      setLogModal({ ctr, text: txt || "(no logs)" });
    } catch {
      setLogModal({ ctr, text: "(failed to load logs)" });
    }
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
          const services: IacService[] = Array.isArray(is?.services) ? (is!.services as IacService[]) : [];
          const hasIac = !!is && (services.length > 0 || !!is.compose);
          
          // NEW: Check if stack has actual content (compose file or services)
          const hasContent = !!is && (!!is.compose || services.length > 0);

          const rows: MergedRow[] = [];

          function desiredImageFor(c: ApiContainer): string | undefined {
            if (!is || services.length === 0) return undefined;
            const svc = services.find(x =>
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
            for (const svc of services) {
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

          let stackDrift: "in_sync" | "drift" | "unknown" = "unknown";
          if (hasIac) {
            stackDrift = rows.some(r => r.drift) ? "drift" : "in_sync";
          }

          merged.push({
            name: sname,
            drift: stackDrift,
            iacEnabled: !!is?.iac_enabled,
            pullPolicy: hasIac ? is?.pull_policy : undefined,
            sops: hasIac ? (is?.sops_status === "all") : false,
            deployKind: hasIac ? (is?.deploy_kind || "compose") : (sname === "(none)" ? "unmanaged" : "unmanaged"),
            rows,
            iacId: is?.id,
            hasIac,
            hasContent, // NEW
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
  }, [host.name, onSync]);

  async function createStackFlow() {
    const existing = new Set(stacks.map(s => s.name));
    let name = prompt("New stack name:");
    if (!name) return;
    name = name.trim();
    if (!name) return;
    if (existing.has(name)) { alert("A stack with that name already exists."); return; }
    try {
      const r = await fetch(`/api/iac/stacks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ scope_kind: "host", scope_name: host.name, stack_name: name, iac_enabled: false }),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      const j = await r.json();
      onOpenStack(name, j.id);
    } catch (e: any) {
      alert(e?.message || "Failed to create stack");
    }
  }

  async function setAutoDevOps(id: number, enabled: boolean) {
    const r = await fetch(`/api/iac/stacks/${id}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ iac_enabled: enabled }),
    });
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  }

  function handleToggleAuto(sIndex: number, enabled: boolean) {
    const s = stacks[sIndex];
    if (!s.iacId || !s.hasContent) {
      if (enabled) {
        alert("This stack needs compose files or services before Auto DevOps can be enabled. Add content first.");
      }
      return;
    }
    setStacks(prev => prev.map((row, i) => i === sIndex ? { ...row, iacEnabled: enabled } : row));
    setAutoDevOps(s.iacId!, enabled).catch(err => {
      alert(`Failed to update Auto DevOps: ${err?.message || err}`);
      setStacks(prev => prev.map((row, i) => i === sIndex ? { ...row, iacEnabled: !enabled } : row));
    });
  }

  async function deleteStackAt(index: number) {
    const s = stacks[index];
    if (!s.iacId) return;
    if (!confirm(`Delete IaC for stack "${s.name}"? This removes IaC files/metadata but not runtime containers.`)) return;
    const r = await fetch(`/api/iac/stacks/${s.iacId}`, { method: "DELETE", credentials: "include" });
    if (!r.ok) { alert(`Failed to delete: ${r.status} ${r.statusText}`); return; }
    setStacks(prev => prev.map((row, i) => i === index
      ? { ...row, iacId: undefined, hasIac: false, iacEnabled: false, pullPolicy: undefined, sops: false, drift: "unknown", hasContent: false }
      : row
    ));
  }

  return (
    <div className="space-y-4">
      {/* Logs modal */}
      {logModal && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={() => setLogModal(null)}>
          <div className="bg-slate-950 border border-slate-800 rounded-xl w-full max-w-3xl p-4" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between mb-2">
              <div className="text-slate-200 font-semibold">Logs: {logModal.ctr}</div>
              <Button size="sm" variant="outline" className="border-slate-700" onClick={() => setLogModal(null)}>Close</Button>
            </div>
            <pre className="text-xs text-slate-300 bg-slate-900 border border-slate-800 rounded p-3 max-h-[60vh] overflow-auto whitespace-pre-wrap">
{logModal.text}
            </pre>
          </div>
        </div>
      )}

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
          <Button onClick={createStackFlow} variant="outline" className="border-slate-700 text-slate-200">
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
                <Badge variant="outline" className="border-slate-700 text-slate-300">pull: {s.hasIac ? (s.pullPolicy || "—") : "—"}</Badge>
                {s.hasIac ? (
                  s.sops ? (
                    <Badge className="bg-indigo-900/40 border-indigo-700/40 text-indigo-200">SOPS</Badge>
                  ) : (
                    <Badge variant="outline" className="border-slate-700 text-slate-300">no SOPS</Badge>
                  )
                ) : (
                  <Badge variant="outline" className="border-slate-700 text-slate-300">No IaC</Badge>
                )}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-slate-300">Auto DevOps</span>
              <Switch
                checked={!!s.iacEnabled}
                onCheckedChange={(v) => handleToggleAuto(idx, !!v)}
                disabled={!s.iacId || !s.hasContent}
              />
              {s.iacId && (
                <Button
                  size="icon"
                  variant="ghost"
                  title="Delete IaC for this stack"
                  onClick={() => deleteStackAt(idx)}
                >
                  <Trash2 className="h-4 w-4 text-rose-300" />
                </Button>
              )}
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
                    <th className="p-3 text-left w-[30rem]">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {(s.rows.filter(r => matchRow(r, hostQuery))).map((r, i) => (
                    <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40">
                      <td className="p-3 font-medium text-slate-200 truncate">{r.name}</td>
                      <td className="p-3 text-slate-300"><StatePill state={r.state} /></td>
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
                      <td className="p-2">
                        <div className="grid grid-cols-4 gap-1 w-fit">
                          <ActionBtn title="Play" icon={Play} onClick={() => doCtrAction(r.name, "start")} />
                          <ActionBtn title="Stop" icon={Square} onClick={() => doCtrAction(r.name, "stop")} />
                          <ActionBtn title="Kill" icon={ZapOff} onClick={() => doCtrAction(r.name, "kill")} />
                          <ActionBtn title="Restart" icon={RotateCw} onClick={() => doCtrAction(r.name, "restart")} />
                          <ActionBtn title="Pause" icon={Pause} onClick={() => doCtrAction(r.name, "pause")} />
                          <ActionBtn title="Resume" icon={PlayCircle} onClick={() => doCtrAction(r.name, "unpause")} />
                          <ActionBtn title="Remove" icon={Trash2} onClick={() => doCtrAction(r.name, "remove")} />
                          <div className="col-span-1" />
                          <ActionBtn title="Logs" icon={FileText} onClick={() => openLogs(r.name)} />
                          <ActionBtn title="Inspect" icon={Bug} onClick={() => onOpenStack(s.name, s.iacId)} />
                          <ActionBtn title="Stats" icon={Activity} onClick={async () => {
                            try {
                              const r2 = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(r.name)}/stats`, { credentials: "include" });
                              const txt = await r2.text();
                              setLogModal({ ctr: `${r.name} (stats)`, text: txt });
                            } catch {
                              setLogModal({ ctr: `${r.name} (stats)`, text: "(failed to load stats)" });
                            }
                          }} />
                          <ActionBtn title="Console" icon={Terminal} onClick={() => alert("Console attach: coming soon")} disabled />
                        </div>
                      </td>
                    </tr>
                  ))}
                  {(!s.rows || s.rows.filter(r => matchRow(r, hostQuery)).length === 0) && (
                    <tr><td className="p-4 text-slate-500" colSpan={8}>No containers or services.</td></tr>
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

/* ==================== Stack Detail & Editor ==================== */

/* Updated: supports overflow-safe values and bulk reveal */
function EnvRow({ k, v, forceShow }: { k: string; v: string; forceShow?: boolean }) {
  const [show, setShow] = useState(false);
  const showEff = forceShow || show;
  const masked = v ? "•".repeat(Math.min(v.length, 24)) : "";
  return (
    <div className="flex items-start gap-3 py-1">
      <div className="text-slate-300 text-sm w-44 shrink-0">{k}</div>
      <div className="flex items-start gap-2 grow min-w-0">
        <div
          className="text-slate-400 text-sm font-mono break-all whitespace-pre-wrap leading-tight max-h-24 overflow-auto pr-1"
        >
          {showEff ? v || "" : masked}
        </div>
        <Button
          size="icon"
          variant="ghost"
          className="h-7 w-7 shrink-0"
          onClick={() => setShow(s => !s)}
          title={showEff ? "Hide" : "Reveal"}
        >
          {showEff ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </Button>
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

type MiniEditorProps = {
  id: string;
  initialPath: string;
  stackId?: number;
  ensureStack: () => Promise<number>; // lazy create before first save
  refresh: () => void;
};

function MiniEditor({
  id, initialPath, stackId, ensureStack, refresh,
}: MiniEditorProps) {
  const [path, setPath] = useState(initialPath);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [sops, setSops] = useState(false);
  const [decryptView, setDecryptView] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => { setPath(initialPath); setContent(""); setErr(null); setDecryptView(false); }, [initialPath]);

  useEffect(() => {
    let cancel = false;
    (async () => {
      if (!stackId) return;
      setLoading(true); setErr(null);
      try {
        const url = `/api/iac/stacks/${stackId}/file?path=${encodeURIComponent(path)}`;
        const r = await fetch(url, { credentials: "include" });
        if (!r.ok) {
          if (r.status !== 404) throw new Error(`${r.status} ${r.statusText}`);
          setContent("");
        } else {
          const txt = await r.text();
          if (!cancel) setContent(txt);
        }
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
  }, [stackId, path]);

  async function saveFile() {
    setLoading(true); setErr(null);
    try {
      const idToUse = stackId ?? await ensureStack();
      const r = await fetch(`/api/iac/stacks/${idToUse}/file`, {
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

  async function revealSops() {
    if (!stackId) { setErr("Create the stack by saving a file first."); return; }
    setDecryptView(true);
    setLoading(true); setErr(null);
    try {
      const url = `/api/iac/stacks/${stackId}/file?path=${encodeURIComponent(path)}&decrypt=1`;
      const r = await fetch(url, {
        credentials: "include",
        headers: { "X-Confirm-Reveal": "yes" },
      });
      const txt = await r.text();
      if (!r.ok) {
        throw new Error(txt || `${r.status} ${r.statusText}`);
      }
      setContent(txt);
    } catch (e: any) {
      setErr(e?.message || "Failed to decrypt");
    } finally {
      setLoading(false);
    }
  }

  async function encryptSops() {
    if (!stackId) { setErr("Create the stack by saving a file first."); return; }
    if (sops) { setErr("File is already marked as SOPS."); return; }
    if (!confirm("Encrypt this file with SOPS? This action cannot be undone locally.")) return;
    
    setLoading(true); setErr(null);
    try {
      setSops(true);
      await saveFile(); // This will trigger SOPS encryption on the server
    } catch (e: any) {
      setSops(false);
      setErr(e?.message || "Failed to encrypt");
    } finally {
      setLoading(false);
    }
  }

  const isSopsFile = path.toLowerCase().includes('_secret.env') || path.toLowerCase().includes('_private.env') || sops;

  return (
    <Card className="bg-slate-900/40 border-slate-800 h-full flex flex-col">
      <CardHeader className="pb-2 shrink-0">
        <CardTitle className="text-sm text-slate-200">Editor</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 min-h-0 flex flex-col gap-3">
        <div className="flex gap-2 shrink-0">
          <Input value={path} onChange={e => setPath(e.target.value)} placeholder="docker-compose/host/stack/compose.yaml" className="flex-1" />
          <div className="flex gap-1">
            {isSopsFile && (
              <Button onClick={revealSops} variant="outline" className="border-indigo-700 text-indigo-200" title="Decrypt and reveal SOPS content">
                <Shield className="h-4 w-4 mr-1" />SOPS Reveal
              </Button>
            )}
            {!sops && !isSopsFile && (
              <Button onClick={encryptSops} variant="outline" className="border-amber-700 text-amber-200" title="Encrypt this file with SOPS">
                <ShieldOff className="h-4 w-4 mr-1" />SOPS Encrypt
              </Button>
            )}
          </div>
        </div>
        {err && <div className="text-xs text-rose-300 shrink-0">Error: {err}</div>}
        {decryptView && <div className="text-xs text-amber-300 shrink-0">Warning: Decrypted secrets are visible in your browser until you navigate away.</div>}
        <textarea
          id={id}
          className="w-full flex-1 min-h-0 text-sm bg-slate-950/50 border border-slate-800 rounded p-2 font-mono text-slate-200"
          value={content}
          onChange={e => setContent(e.target.value)}
          placeholder={loading ? "Loading…" : "File content…"}
        />
        <div className="flex items-center justify-between shrink-0">
          <label className="text-sm text-slate-300 inline-flex items-center gap-2">
            <input type="checkbox" checked={sops} onChange={e => setSops(e.target.checked)} />
            Mark as SOPS file
          </label>
          <Button onClick={saveFile} disabled={loading}><Save className="h-4 w-4 mr-1" /> Save</Button>
        </div>
        <div className="text-xs text-slate-500 -mt-2">
          Files ending with <code>_private.env</code> or <code>_secret.env</code> will auto-encrypt with SOPS (if the server has a key).
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
  const [stackIacId, setStackIacId] = useState<number | undefined>(iacId);
  const [autoDevOps, setAutoDevOps] = useState<boolean>(false);
  const [revealEnvAll, setRevealEnvAll] = useState<boolean>(false);
  const [deploying, setDeploying] = useState<boolean>(false);

  useEffect(() => { setAutoDevOps(false); }, [stackName]);

  async function refreshFiles() {
    if (!stackIacId) return;
    const r = await fetch(`/api/iac/stacks/${stackIacId}/files`, { credentials: "include" });
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

        if (stackIacId) await refreshFiles();
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load stack");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host.name, stackName, stackIacId]);

  async function ensureStack() {
    if (stackIacId) return stackIacId;
    const r = await fetch(`/api/iac/stacks`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ scope_kind: "host", scope_name: host.name, stack_name: stackName, iac_enabled: false }),
    });
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    const j = await r.json();
    if (j.id) setStackIacId(j.id);
    return j.id;
  }

  async function deleteStack() {
    if (!stackIacId) return;
    if (!confirm(`Delete IaC stack "${stackName}"? This only deletes IaC metadata/files, not runtime containers.`)) return;
    const r = await fetch(`/api/iac/stacks/${stackIacId}`, {
      method: "DELETE",
      credentials: "include",
    });
    if (!r.ok) {
      alert(`Failed to delete: ${r.status} ${r.statusText}`);
      return;
    }
    setStackIacId(undefined);
    setFiles([]);
    setEditPath(null);
  }

  async function toggleAutoDevOps(checked: boolean) {
    let id = stackIacId;
    if (!id) {
      try {
        id = await ensureStack();
        setStackIacId(id);
      } catch (e: any) {
        alert(e?.message || "Unable to create stack for Auto DevOps");
        return;
      }
    }
  
    // Check if stack has content before enabling
    if (checked && files.length === 0) {
      alert("This stack needs compose files or services before Auto DevOps can be enabled. Add content first.");
      return;
    }
  
    setAutoDevOps(checked);
    const r = await fetch(`/api/iac/stacks/${id}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ iac_enabled: checked }),
    });
  }

  async function deployNow() {
    if (!stackIacId) {
      alert("Create the stack (save a file) before deploying.");
      return;
    }
    if (files.length === 0) {
      alert("This stack has no files to deploy. Add a compose file or scripts first.");
      return;
    }
    
    setDeploying(true);
    try {
      const r = await fetch(`/api/iac/stacks/${stackIacId}/deploy`, { method: "POST", credentials: "include" });
      if (!r.ok) {
        const txt = await r.text();
        alert(`Deploy failed: ${r.status} ${txt}`);
        return;
      }
      alert("Deploy requested. Check host for activity.");
    } catch (e: any) {
      alert(`Deploy failed: ${e?.message || e}`);
    } finally {
      setDeploying(false);
    }
  }

  const hasContent = files.some(f => f.role === 'compose') || files.length > 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={onBack}>
          <ArrowLeft className="h-4 w-4 mr-1" /> Back to {host.name}
        </Button>
        <div className="ml-2 text-lg font-semibold text-white">
          Stack: {stackName}
        </div>
        <div className="ml-auto flex items-center gap-3">
          <Button 
            onClick={deployNow} 
            disabled={deploying || !hasContent}
            className="bg-emerald-800 hover:bg-emerald-900 text-white disabled:opacity-50"
          >
            <RotateCw className={`h-4 w-4 mr-1 ${deploying ? 'animate-spin' : ''}`} />
            {deploying ? 'Deploying...' : 'Deploy'}
          </Button>
          <span className="text-sm text-slate-300">Auto DevOps</span>
          <Switch 
            checked={autoDevOps} 
            onCheckedChange={(v) => toggleAutoDevOps(!!v)}
          />
          {stackIacId ? (
            <>
              <Button onClick={refreshFiles} variant="outline" className="border-slate-700">
                <RefreshCw className="h-4 w-4 mr-1" /> Refresh
              </Button>
              <Button onClick={deleteStack} variant="outline" className="border-rose-700 text-rose-200">
                <Trash2 className="h-4 w-4 mr-1" /> Delete IaC
              </Button>
            </>
          ) : null}
        </div>
      </div>

      {loading && <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">Loading…</div>}
      {err && <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">Error: {err}</div>}

      <div className="grid lg:grid-cols-2 gap-4">
        {/* Left: Active Containers */}
        <div className="space-y-4">
          <Card className="bg-slate-900/50 border-slate-800">
            <CardHeader className="pb-2 flex items-center justify-between">
              <CardTitle className="text-slate-200 text-lg">Active Containers</CardTitle>
              <Button
                size="sm"
                variant="outline"
                className="border-slate-700"
                onClick={() => setRevealEnvAll(v => !v)}
                title={revealEnvAll ? "Hide all env" : "Reveal all env"}
              >
                {revealEnvAll ? <EyeOff className="h-4 w-4 mr-1" /> : <Eye className="h-4 w-4 mr-1" />}
                {revealEnvAll ? "Hide env" : "Reveal env"}
              </Button>
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
                  </div>

                  {/* Facts with aligned center divider */}
                  <div className="mt-2 grid md:grid-cols-2 gap-3">
                    <div className="space-y-2 md:pr-3 md:border-r md:border-slate-800">
                      <Fact label="CMD" value={<span className="font-mono">{(c.cmd || []).join(" ") || "—"}</span>} />
                      <Fact label="ENTRYPOINT" value={<span className="font-mono">{(c.entrypoint || []).join(" ") || "—"}</span>} />
                      <Fact label="Image" value={<span className="font-mono">{c.image || "—"}</span>} />
                    </div>
                    <div className="space-y-2 md:pl-3 md:border-l md:border-slate-800">
                      <Fact label="Networks" value={(c.networks || []).join(", ") || "—"} />
                      <Fact label="Ports" value={<PortsBlock ports={c.ports} />} />
                      <Fact label="Restart policy" value={c.restart_policy || "—"} />
                    </div>
                  </div>

                  <div className="mt-4 grid md:grid-cols-2 gap-3">
                    <div className="md:pr-3 md:border-r md:border-slate-800">
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Environment</div>
                      {(!c.env || Object.keys(c.env).length === 0) && <div className="text-sm text-slate-500">No environment variables.</div>}
                      <div className="space-y-1">
                        {Object.entries(c.env || {}).map(([k, v]) => (
                          <EnvRow key={k} k={k} v={v} forceShow={revealEnvAll} />
                        ))}
                      </div>
                    </div>
                    <div className="md:pl-3 md:border-l md:border-slate-800">
                      <div className="text-xs uppercase tracking-wide text-slate-400 mb-2">Labels</div>
                      {(!c.labels || Object.keys(c.labels).length === 0) && <div className="text-sm text-slate-500">No labels.</div>}
                      {(Object.entries(c.labels || {}).sort(([a],[b]) => a.localeCompare(b))).map(([k,v]) => (
                        <div key={k} className="flex items-center justify-between gap-2 text-sm">
                          <div className="text-slate-300">{k}</div>
                          <div className="text-slate-400 font-mono truncate max-w-[22rem]" title={v}>{v}</div>
                        </div>
                      ))}
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

        {/* Right: IaC Files / Editor */}
        <div className="space-y-4 lg:sticky lg:top-4 lg:h-[calc(100vh-140px)] lg:z-10">
          <Card className="bg-slate-900/50 border-slate-800 h-full flex flex-col">
            <CardHeader className="pb-2 shrink-0 flex flex-row items-center justify-between">
              <CardTitle className="text-slate-200 text-lg">IaC Files</CardTitle>
              {stackIacId && hasContent && (
                <Button 
                  onClick={deployNow} 
                  disabled={deploying}
                  size="sm"
                  className="bg-emerald-800 hover:bg-emerald-900 text-white"
                >
                  <RotateCw className={`h-4 w-4 mr-1 ${deploying ? 'animate-spin' : ''}`} />
                  Deploy
                </Button>
              )}
            </CardHeader>
            <CardContent className="flex-1 min-h-0 flex flex-col gap-3">
              {!stackIacId && (
                <div className="text-sm text-amber-300 shrink-0">
                  No IaC yet. Use the buttons below — the <b>first Save</b> will create the IaC stack automatically.
                </div>
              )}

              <div className="flex items-center justify-between shrink-0">
                <div className="text-slate-300 text-sm">{files.length} file(s)</div>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    onClick={() => setEditPath(`docker-compose/${host.name}/${stackName}/docker-compose.yaml`)}
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

              <div className="rounded-lg border border-slate-800 overflow-hidden shrink-0">
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
                            <Button size="icon" variant="ghost" onClick={async () => {
                              if (!stackIacId) return;
                              const r = await fetch(`/api/iac/stacks/${stackIacId}/file?path=${encodeURIComponent(f.rel_path)}`, {
                                method: "DELETE", credentials: "include"
                              });
                              if (r.ok) refreshFiles();
                            }} title="Delete">
                              <Trash2 className="h-4 w-4 text-rose-300" />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                    {files.length === 0 && (
                      <tr><td className="p-3 text-slate-500" colSpan={6}>No files yet. Add compose/env/script above.</td></tr>
                    )}
                  </tbody>
                </table>
              </div>

              {editPath && (
                <div className="flex-1 min-h-0">
                  <MiniEditor
                    key={editPath}
                    id="stack-editor"
                    initialPath={editPath}
                    stackId={stackIacId}
                    ensureStack={ensureStack}
                    refresh={() => { setEditPath(null); refreshFiles(); }}
                  />
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {!loading && containers.length === 0 && !stackIacId && (
        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-4 text-sm text-slate-300">
            This stack has no running containers on <b>{host.name}</b> and is not declared in IaC yet.
            Save a file to create the IaC entry, or just navigate away to leave nothing behind.
          </CardContent>
        </Card>
      )}
    </div>
  );
}

/* ==================== Images/Networks/Volumes with Sorting + Bulk Delete ==================== */

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
  children, 
  sortKey, 
  currentSort, 
  onSort 
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
      <button 
        className="flex items-center gap-1 hover:text-white transition"
        onClick={() => onSort(sortKey)}
      >
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

function ImagesView({ hosts }: { hosts: Host[] }) {
  const [hostName, setHostName] = useState(hosts[0]?.name || "");
  const [rows, setRows] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [sort, setSort] = useState<{ key: string; direction: 'asc' | 'desc' }>({ key: 'repo', direction: 'asc' });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const allSelected = rows.length > 0 && selected.size === rows.length;

  useEffect(() => {
    if (!hostName) return;
    (async () => {
      setLoading(true);
      try {
        const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images`, { credentials: "include" });
        const j = await r.json();
        setRows(j.items || []);
        setSelected(new Set());
      } finally {
        setLoading(false);
      }
    })();
  }, [hostName]);

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

  const toggleOne = (id: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };
  const toggleAll = () => {
    if (allSelected) {
      setSelected(new Set());
    } else {
      setSelected(new Set(rows.map((r:any) => r.id)));
    }
  };

  async function bulkDelete(force = true) {
    if (selected.size === 0) return;
    if (!confirm(`Delete ${selected.size} image(s) from ${hostName}?`)) return;
    const ids = Array.from(selected);
    const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images/delete`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ids, force }),
    });
    if (!r.ok) {
      const txt = await r.text();
      alert(`Delete failed: ${txt}`);
      return;
    }
    const r2 = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/images`, { credentials: "include" });
    const j2 = await r2.json();
    setRows(j2.items || []);
    setSelected(new Set());
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-lg font-semibold text-white">Images</div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            className="border-rose-700 text-rose-200 disabled:opacity-50"
            disabled={selected.size === 0}
            onClick={() => bulkDelete(true)}
            title="Force delete selected images"
          >
            <Trash2 className="h-4 w-4 mr-1" /> Delete selected
          </Button>
          <HostPicker hosts={hosts} activeHost={hostName} setActiveHost={setHostName} />
        </div>
      </div>
      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-2 text-left w-10">
                <input type="checkbox" checked={allSelected} onChange={toggleAll} />
              </th>
              <SortableHeader sortKey="repo" currentSort={sort} onSort={handleSort}>Repository</SortableHeader>
              <SortableHeader sortKey="tag" currentSort={sort} onSort={handleSort}>Tag</SortableHeader>
              <SortableHeader sortKey="id" currentSort={sort} onSort={handleSort}>Image ID (sha256)</SortableHeader>
              <SortableHeader sortKey="size" currentSort={sort} onSort={handleSort}>Size</SortableHeader>
              <SortableHeader sortKey="created" currentSort={sort} onSort={handleSort}>Created</SortableHeader>
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td className="p-3 text-slate-500" colSpan={6}>Loading…</td></tr>}
            {(!loading && sortedRows.length === 0) && <tr><td className="p-3 text-slate-500" colSpan={6}>No images.</td></tr>}
            {sortedRows.map((im, i) => (
              <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40">
                <td className="p-2">
                  <input type="checkbox" checked={selected.has(im.id)} onChange={() => toggleOne(im.id)} />
                </td>
                <td className="p-2 text-slate-300">{im.repo || "<none>"}</td>
                <td className="p-2 text-slate-300">{im.tag || "none"}</td>
                <td className="p-2 text-slate-300 font-mono text-xs break-all">{im.id}</td>
                <td className="p-2 text-slate-300">{im.size || "—"}</td>
                <td className="p-2 text-slate-300">{im.created || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function NetworksView({ hosts }: { hosts: Host[] }) {
  const [hostName, setHostName] = useState(hosts[0]?.name || "");
  const [rows, setRows] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [sort, setSort] = useState<{ key: string; direction: 'asc' | 'desc' }>({ key: 'name', direction: 'asc' });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const allSelected = rows.length > 0 && selected.size === rows.length;

  useEffect(() => {
    if (!hostName) return;
    (async () => {
      setLoading(true);
      try {
        const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/networks`, { credentials: "include" });
        const j = await r.json();
        setRows(j.items || []);
        setSelected(new Set());
      } finally {
        setLoading(false);
      }
    })();
  }, [hostName]);

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

  const toggleOne = (name: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name); else next.add(name);
      return next;
    });
  };
  const toggleAll = () => {
    if (allSelected) setSelected(new Set());
    else setSelected(new Set(rows.map((r:any) => r.name)));
  };

  async function bulkDelete() {
    if (selected.size === 0) return;
    if (!confirm(`Delete ${selected.size} network(s) from ${hostName}?`)) return;
    const names = Array.from(selected);
    const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/networks/delete`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ names }),
    });
    if (!r.ok) {
      const txt = await r.text();
      alert(`Delete failed: ${txt}`);
      return;
    }
    const r2 = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/networks`, { credentials: "include" });
    const j2 = await r2.json();
    setRows(j2.items || []);
    setSelected(new Set());
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-lg font-semibold text-white">Networks</div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            className="border-rose-700 text-rose-200 disabled:opacity-50"
            disabled={selected.size === 0}
            onClick={bulkDelete}
          >
            <Trash2 className="h-4 w-4 mr-1" /> Delete selected
          </Button>
          <HostPicker hosts={hosts} activeHost={hostName} setActiveHost={setHostName} />
        </div>
      </div>
      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-2 text-left w-10">
                <input type="checkbox" checked={allSelected} onChange={toggleAll} />
              </th>
              <SortableHeader sortKey="name" currentSort={sort} onSort={handleSort}>Name</SortableHeader>
              <SortableHeader sortKey="driver" currentSort={sort} onSort={handleSort}>Driver</SortableHeader>
              <SortableHeader sortKey="scope" currentSort={sort} onSort={handleSort}>Scope</SortableHeader>
              <SortableHeader sortKey="id" currentSort={sort} onSort={handleSort}>ID</SortableHeader>
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td className="p-3 text-slate-500" colSpan={5}>Loading…</td></tr>}
            {(!loading && sortedRows.length === 0) && <tr><td className="p-3 text-slate-500" colSpan={5}>No networks.</td></tr>}
            {sortedRows.map((n, i) => (
              <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40">
                <td className="p-2">
                  <input type="checkbox" checked={selected.has(n.name)} onChange={() => toggleOne(n.name)} />
                </td>
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

function VolumesView({ hosts }: { hosts: Host[] }) {
  const [hostName, setHostName] = useState(hosts[0]?.name || "");
  const [rows, setRows] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [sort, setSort] = useState<{ key: string; direction: 'asc' | 'desc' }>({ key: 'name', direction: 'asc' });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const allSelected = rows.length > 0 && selected.size === rows.length;

  useEffect(() => {
    if (!hostName) return;
    (async () => {
      setLoading(true);
      try {
        const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/volumes`, { credentials: "include" });
        const j = await r.json();
        setRows(j.items || []);
        setSelected(new Set());
      } finally {
        setLoading(false);
      }
    })();
  }, [hostName]);

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

  const toggleOne = (name: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name); else next.add(name);
      return next;
    });
  };
  const toggleAll = () => {
    if (allSelected) setSelected(new Set());
    else setSelected(new Set(rows.map((r:any) => r.name)));
  };

  async function bulkDelete(force = true) {
    if (selected.size === 0) return;
    if (!confirm(`Delete ${selected.size} volume(s) from ${hostName}?`)) return;
    const names = Array.from(selected);
    const r = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/volumes/delete`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ names, force }),
    });
    if (!r.ok) {
      const txt = await r.text();
      alert(`Delete failed: ${txt}`);
      return;
    }
    const r2 = await fetch(`/api/hosts/${encodeURIComponent(hostName)}/volumes`, { credentials: "include" });
    const j2 = await r2.json();
    setRows(j2.items || []);
    setSelected(new Set());
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-lg font-semibold text-white">Volumes</div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            className="border-rose-700 text-rose-200 disabled:opacity-50"
            disabled={selected.size === 0}
            onClick={() => bulkDelete(true)}
          >
            <Trash2 className="h-4 w-4 mr-1" /> Delete selected
          </Button>
          <HostPicker hosts={hosts} activeHost={hostName} setActiveHost={setHostName} />
        </div>
      </div>
      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-2 text-left w-10">
                <input type="checkbox" checked={allSelected} onChange={toggleAll} />
              </th>
              <SortableHeader sortKey="name" currentSort={sort} onSort={handleSort}>Name</SortableHeader>
              <SortableHeader sortKey="driver" currentSort={sort} onSort={handleSort}>Driver</SortableHeader>
              <SortableHeader sortKey="mountpoint" currentSort={sort} onSort={handleSort}>Mountpoint</SortableHeader>
              <SortableHeader sortKey="created" currentSort={sort} onSort={handleSort}>Created</SortableHeader>
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td className="p-3 text-slate-500" colSpan={5}>Loading…</td></tr>}
            {(!loading && sortedRows.length === 0) && <tr><td className="p-3 text-slate-500" colSpan={5}>No volumes.</td></tr>}
            {sortedRows.map((v, i) => (
              <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40">
                <td className="p-2">
                  <input type="checkbox" checked={selected.has(v.name)} onChange={() => toggleOne(v.name)} />
                </td>
                <td className="p-2 text-slate-300">{v.name}</td>
                <td className="p-2 text-slate-300">{v.driver}</td>
                <td className="p-2 text-slate-300 font-mono text-xs">{v.mountpoint}</td>
                <td className="p-2 text-slate-300">{v.created || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/* ==================== Login Gate ==================== */

function LoginGate() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-950">
      <Card className="w-full max-w-sm bg-slate-900/60 border-slate-800">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span className="font-black uppercase tracking-tight leading-none text-slate-200 select-none">
              <span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">DDUI</span>
            </span>
            <Badge variant="outline">Community</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-slate-300 text-sm">You're signed out. Continue to your identity provider to sign in.</p>
          <Button
            className="w-full bg-[#310937] hover:bg-[#2a0830] text-white"
            onClick={() => { window.location.replace("/auth/login"); }}
          >
            Continue to Sign in
          </Button>
          <p className="text-xs text-slate-500">
            If you get stuck, ensure your OIDC <code>RedirectURL</code> points back to
            <code> /auth/callback</code> and that cookies aren't blocked.
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

  const [page, setPage] = useState<"deployments" | "host" | "stack" | "images" | "networks" | "volumes">("deployments");
  const [activeHost, setActiveHost] = useState<Host | null>(null);
  const [activeStack, setActiveStack] = useState<{ name: string; iacId?: number } | null>(null);

  const [sessionChecked, setSessionChecked] = useState(false);
  const [authed, setAuthed] = useState<boolean>(false);

  useEffect(() => {
    let cancel = false;
    (async () => {
      try {
        const r = await fetch("/api/session", { credentials: "include" });
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = (await r.json()) as SessionResp;
        if (!cancel) setAuthed(!!data.user);
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
      const services: IacService[] = Array.isArray(is?.services) ? (is!.services as IacService[]) : [];
      const hasIac = !!is && (services.length > 0 || !!is.compose);
      let stackDrift = false;

      const desiredImageFor = (c: ApiContainer): string | undefined => {
        if (!is || services.length === 0) return undefined;
        const svc = services.find(x =>
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
      if (!stackDrift && is && services.length > 0) {
        for (const svc of services) {
          const match = rcs.some(c =>
            (c.compose_service && svc.service_name === c.compose_service) ||
            (svc.container_name && c.name === svc.container_name)
          );
          if (!match) { stackDrift = true; break; }
        }
      }
      if (!rcs.length && hasIac && services.length > 0) stackDrift = true;
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

  if (sessionChecked && !authed) {
    return <LoginGate />;
  }
  if (!sessionChecked) {
    return <div className="min-h-screen bg-slate-950" />;
  }

  const hostMetrics = activeHost ? (metricsCache[activeHost.name] || { stacks: 0, containers: 0, drift: 0, errors: 0 }) : null;

  return (
    <div className="min-h-screen flex">
      <LeftNav
        page={page}
        onGoDeployments={() => setPage("deployments")}
        onGoImages={() => setPage("images")}
        onGoNetworks={() => setPage("networks")}
        onGoVolumes={() => setPage("volumes")}
      />

      <div className="flex-1 min-w-0">
        <main className="px-6 py-6 space-y-6">
          {page === 'deployments' && (
            <div className="grid md:grid-cols-5 gap-4">
              <MetricCard title="Hosts" value={metrics.hosts} icon={Boxes} accent />
              <MetricCard title="Stacks" value={metrics.stacks} icon={Boxes} />
              <MetricCard title="Containers" value={metrics.containers} icon={Layers} />
              <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
              <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
            </div>
          )}

          {page === 'host' && activeHost && (
            <div className="grid md:grid-cols-4 gap-4">
              <MetricCard title="Stacks" value={hostMetrics!.stacks} icon={Boxes} />
              <MetricCard title="Containers" value={hostMetrics!.containers} icon={Layers} />
              <MetricCard title="Drift" value={<span className="text-amber-400">{hostMetrics!.drift}</span>} icon={AlertTriangle} />
              <MetricCard title="Errors" value={<span className="text-rose-400">{hostMetrics!.errors}</span>} icon={XCircle} />
            </div>
          )}

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
                        <td className="p-3">{/* per-host results summarized in metrics */}</td>
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
              onOpenStack={(name, id) => { setActiveStack({ name, iacId: id }); setPage('stack'); }}
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

          {page === 'images' && <ImagesView hosts={hosts} />}
          {page === 'networks' && <NetworksView hosts={hosts} />}
          {page === 'volumes' && <VolumesView hosts={hosts} />}

          <div className="pt-6 pb-10 text-center text-xs text-slate-500">
            © 2025 PrecisionPlanIT &amp; SoFMeRight (Kai)
          </div>
        </main>
      </div>
    </div>
  );
}
