import React, { useEffect, useMemo, useState } from "react";
import {
  ArrowLeft, RefreshCw, Plus, Search, ChevronRight, FileText, Bug,
  Activity, ZapOff, Trash2, Terminal, Play, Square, Pause, PlayCircle, RotateCw,
  Eye, EyeOff, ShieldCheck
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";

/* ===== Types ===== */
type Host = { name: string; address?: string; groups?: string[] };
type ApiContainer = {
  name: string; image: string; state: string; status: string; owner?: string;
  ports?: any; labels?: Record<string, string>; updated_at?: string; created_ts?: string;
  ip_addr?: string; compose_project?: string; compose_service?: string; stack?: string | null;
};
type IacEnvFile = { path: string; sops: boolean };
type IacService = {
  id: number; stack_id: number; service_name: string; container_name?: string; image?: string;
  labels: Record<string, string>; env_keys: string[]; env_files: IacEnvFile[];
  ports: any[]; volumes: any[]; deploy: Record<string, any>;
};
type IacStack = {
  id: number; name: string; scope_kind: string; scope_name: string;
  deploy_kind: "compose" | "script" | "unmanaged" | string;
  pull_policy?: string; sops_status: "all" | "partial" | "none" | string;
  iac_enabled: boolean; rel_path: string; compose?: string; services: IacService[] | null | undefined;
};

/* ===== Small UI bits ===== */
function StatePill({ state, health }: { state?: string; health?: string }) {
  const s = (state || "").toLowerCase();
  const h = (health || "").toLowerCase();
  let classes = "border-slate-700 bg-slate-900 text-slate-300";
  let text = state || "unknown";
  if (h === "healthy") { classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200"; text = "healthy"; }
  else if (s.includes("running") || s.includes("up")) { classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200"; }
  else if (s.includes("restarting")) { classes = "border-amber-700/60 bg-amber-900/40 text-amber-200"; }
  else if (s.includes("paused")) { classes = "border-sky-700/60 bg-sky-900/40 text-sky-200"; }
  else if (s.includes("exited") || s.includes("dead")) { classes = "border-rose-700/60 bg-rose-900/40 text-rose-200"; }
  return <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border ${classes}`}>{text}</span>;
}
function driftBadge(d: "in_sync" | "drift" | "unknown") {
  if (d === "in_sync") return <Badge className="bg-emerald-900/40 border-emerald-700/40 text-emerald-200">In sync</Badge>;
  if (d === "drift") return <Badge variant="destructive">Drift</Badge>;
  return <Badge variant="outline" className="border-slate-700 text-slate-300">Unknown</Badge>;
}
function formatDT(s?: string) {
  if (!s) return "—";
  const d = new Date(s); return isNaN(d.getTime()) ? s : d.toLocaleString();
}
function formatPortsLines(ports: any): string[] {
  const arr: any[] = Array.isArray(ports) ? ports : (ports && Array.isArray(ports.ports)) ? ports.ports : [];
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
function ActionBtn({ title, onClick, icon: Icon, disabled=false }:{
  title: string; onClick: ()=>void; icon: any; disabled?: boolean;
}) {
  return (
    <Button size="icon" variant="ghost" className="h-6 w-6 shrink-0" title={title} onClick={onClick} disabled={disabled}>
      <Icon className="h-3.5 w-3.5 text-slate-200" />
    </Button>
  );
}

/* ===== Main view ===== */
export function HostStacksView({
  host, onBack, onSync, onOpenStack,
}: {
  host: Host;
  onBack: () => void;
  onSync: () => void;
  onOpenStack: (stackName: string, iacId?: number) => void;
}) {
  type MergedRow = {
    name: string; state: string; stack: string; imageRun?: string; imageIac?: string;
    created?: string; ip?: string; portsText?: string; owner?: string; drift?: boolean;
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
    hasContent?: boolean;
  };

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
    } catch {
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
          const hasContent = !!is && (!!is.compose || services.length > 0);

          const rows: MergedRow[] = [];
          const desiredImageFor = (c: ApiContainer): string | undefined => {
            if (!is || services.length === 0) return undefined;
            const svc = services.find(x =>
              (c.compose_service && x.service_name === c.compose_service) ||
              (x.container_name && x.container_name === c.name)
            );
            return svc?.image || undefined;
          };
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
          if (hasIac) stackDrift = rows.some(r => r.drift) ? "drift" : "in_sync";

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
            hasContent,
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
      if (enabled) alert("This stack needs compose files or services before Auto DevOps can be enabled. Add content first.");
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
                <Button size="icon" variant="ghost" title="Delete IaC for this stack" onClick={() => deleteStackAt(idx)}>
                  <Trash2 className="h-4 w-4 text-rose-300" />
                </Button>
              )}
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <div className="overflow-x-auto rounded-lg border border-slate-800">
              <table className="w-full text-xs table-fixed">
                <thead className="bg-slate-900/70 text-slate-300">
                  <tr className="whitespace-nowrap">
                    <th className="px-2 py-2 text-left w-56">Name</th>
                    <th className="px-2 py-2 text-left w-36">State</th>
                    <th className="px-2 py-2 text-left w-[24rem]">Image</th>
                    <th className="px-2 py-2 text-left w-40">Created</th>
                    <th className="px-2 py-2 text-left w-36">IP</th>
                    <th className="px-2 py-2 text-left w-56">Published Ports</th>
                    <th className="px-2 py-2 text-left w-32">Owner</th>
                    <th className="px-2 py-2 text-left w-[18rem]">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {(s.rows.filter(r => matchRow(r, hostQuery))).map((r, i) => {
                    const st = (r.state || "").toLowerCase();
                    const isRunning = st.includes("running") || st.includes("up") || st.includes("healthy") || st.includes("restarting");
                    const isPaused = st.includes("paused");
                    return (
                      <tr key={i} className="border-t border-slate-800 hover:bg-slate-900/40 align-top">
                        <td className="px-2 py-1.5 font-medium text-slate-200 truncate">{r.name}</td>
                        <td className="px-2 py-1.5 text-slate-300"><StatePill state={r.state} /></td>
                        <td className="px-2 py-1.5 text-slate-300">
                          <div className="flex items-center gap-2">
                            <div className="max-w-[24rem] truncate" title={r.imageRun || ""}>{r.imageRun || "—"}</div>
                            {r.imageIac && (
                              <>
                                <ChevronRight className="h-4 w-4 text-slate-500" />
                                <div className={`max-w-[24rem] truncate ${r.drift ? "text-amber-300" : "text-slate-300"}`} title={r.imageIac}>
                                  {r.imageIac}
                                </div>
                              </>
                            )}
                          </div>
                        </td>
                        <td className="px-2 py-1.5 text-slate-300">{r.created || "—"}</td>
                        <td className="px-2 py-1.5 text-slate-300">{r.ip || "—"}</td>
                        <td className="px-2 py-1.5 text-slate-300 align-top">
                          <div className="max-w-56 whitespace-pre-line leading-tight">
                            {r.portsText || "—"}
                          </div>
                        </td>
                        <td className="px-2 py-1.5 text-slate-300">{r.owner || "—"}</td>
                        <td className="px-2 py-1">
                          <div className="flex items-center gap-1 overflow-x-auto whitespace-nowrap py-0.5">
                            {!isRunning && !isPaused && (<ActionBtn title="Start" icon={Play} onClick={() => doCtrAction(r.name, "start")} />)}
                            {isRunning && (<ActionBtn title="Stop" icon={Square} onClick={() => doCtrAction(r.name, "stop")} />)}
                            {(isRunning || isPaused) && (<ActionBtn title="Restart" icon={RotateCw} onClick={() => doCtrAction(r.name, "restart")} />)}
                            {isRunning && !isPaused && (<ActionBtn title="Pause" icon={Pause} onClick={() => doCtrAction(r.name, "pause")} />)}
                            {isPaused && (<ActionBtn title="Resume" icon={PlayCircle} onClick={() => doCtrAction(r.name, "unpause")} />)}

                            <span className="mx-1 h-4 w-px bg-slate-700/60" />

                            <ActionBtn title="Logs" icon={FileText} onClick={() => openLogs(r.name)} />
                            <ActionBtn title="Inspect" icon={Bug} onClick={() => onOpenStack(s.name, s.iacId)} />
                            <ActionBtn
                              title="Stats"
                              icon={Activity}
                              onClick={async () => {
                                try {
                                  const r2 = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(r.name)}/stats`, { credentials: "include" });
                                  const txt = await r2.text();
                                  setLogModal({ ctr: `${r.name} (stats)`, text: txt });
                                } catch {
                                  setLogModal({ ctr: `${r.name} (stats)`, text: "(failed to load stats)" });
                                }
                              }}
                            />

                            <span className="mx-1 h-4 w-px bg-slate-700/60" />

                            <ActionBtn title="Kill" icon={ZapOff} onClick={() => doCtrAction(r.name, "kill")} />
                            <ActionBtn title="Remove" icon={Trash2} onClick={() => doCtrAction(r.name, "remove")} />
                            <ActionBtn title="Console (soon)" icon={Terminal} onClick={() => {}} disabled />
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                  {(!s.rows || s.rows.filter(r => matchRow(r, hostQuery)).length === 0) && (
                    <tr><td className="p-3 text-slate-500" colSpan={8}>No containers or services.</td></tr>
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
