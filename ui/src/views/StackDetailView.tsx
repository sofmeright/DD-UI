// ui/src/views/StackDetailView.tsx
import { useEffect, useRef, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { ArrowLeft, ChevronRight, Eye, EyeOff, RefreshCw, Trash2, Rocket, Square } from "lucide-react";
import Fact from "@/components/Fact";
import MiniEditor from "@/editors/MiniEditor";
import StatePill from "@/components/StatePill";
import { ApiContainer, Host, IacFileMeta, InspectOut } from "@/types";
import { formatDT } from "@/utils/format";

/* ---------- Shared row primitives (uniform font/spacing/columns) ---------- */

function RowShell({ children, index }: { children: React.ReactNode; index: number }) {
  const zebra = index % 2 === 0 ? "bg-slate-900/30" : "bg-slate-900/10";
  return <div className={`grid grid-cols-12 items-center gap-2 px-2 py-1.5 rounded ${zebra}`}>{children}</div>;
}

// Readonly, scroll-on-focus input (truncates visually, scrolls on selection)
function ValueBox({ value, title }: { value: string; title?: string }) {
  return (
    <input
      readOnly
      value={value}
      title={title ?? value}
      onFocus={(e) => e.currentTarget.select()}
      className="w-full max-w-full bg-slate-900/40 border border-slate-700 text-slate-300 font-mono text-sm px-2 py-1 rounded outline-none focus:ring-1 focus:ring-slate-600 overflow-auto"
    />
  );
}

/* ---------- Collapsible wrapper with right-side actions ---------- */

function CollapsibleSection({
  title,
  count,
  rightAction,
  children,
  defaultOpen = false,
}: {
  title: string;
  count?: number;
  rightAction?: React.ReactNode;
  children: React.ReactNode;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="border border-slate-800 rounded-lg">
      <button
        type="button"
        className="w-full flex items-center justify-between px-3 py-2 hover:bg-slate-900/40"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
      >
        <div className="flex items-center gap-2">
          <ChevronRight className={`h-4 w-4 text-slate-400 transition-transform ${open ? "rotate-90" : ""}`} />
          <div className="text-xs uppercase tracking-wide text-slate-400">{title}</div>
        </div>
        <div className="flex items-center gap-2">
          {rightAction}
          {typeof count === "number" && <Badge variant="outline" className="border-slate-700 text-slate-300">{count}</Badge>}
        </div>
      </button>
      {open && <div className="px-3 pb-3 pt-1">{children}</div>}
    </div>
  );
}

/* ---------- Sections (Env, Labels, Networks, Ports, Volumes) ---------- */

function EnvRow({
  k,
  v,
  forceShow,
  index,
}: {
  k: string;
  v: string;
  forceShow?: boolean;
  index: number;
}) {
  const [show, setShow] = useState(false);
  const showEff = !!forceShow || show;
  const masked = v ? "•".repeat(Math.min(v.length, 24)) : "";
  const display = showEff ? v || "" : masked;

  return (
    <RowShell index={index}>
      <div className="col-span-4 text-slate-300 text-sm font-medium break-words">{k}</div>
      <div className="col-span-7">
        <ValueBox value={display} title={showEff ? v : undefined} />
      </div>
      <div className="col-span-1 flex justify-end">
        <Button
          size="icon"
          variant="ghost"
          className="h-7 w-7"
          onClick={() => setShow((s) => !s)}
          title={showEff ? "Hide" : "Reveal"}
        >
          {showEff ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </Button>
      </div>
    </RowShell>
  );
}

function LabelRow({ k, v, index }: { k: string; v: string; index: number }) {
  return (
    <RowShell index={index}>
      <div className="col-span-4 text-slate-300 text-sm font-medium break-words">{k}</div>
      <div className="col-span-8">
        <ValueBox value={v} />
      </div>
    </RowShell>
  );
}

function PortsBlock({ ports }: { ports?: InspectOut["ports"] }) {
  const list = ports || [];
  if (!list.length) return <div className="text-sm text-slate-500">No port bindings.</div>;
  return (
    <div className="space-y-1 text-sm">
      {list.map((p, i) => (
        <div key={i} className="text-slate-300">
          {(p.published ? p.published + " → " : "")}
          {p.target}
          {p.protocol ? "/" + p.protocol : ""}
        </div>
      ))}
    </div>
  );
}

function VolsBlock({ vols }: { vols?: InspectOut["volumes"] }) {
  const list = vols || [];
  if (!list.length) return <div className="text-sm text-slate-500">No mounts.</div>;
  return (
    <div className="space-y-1">
      {list.map((m, i) => {
        const zebra = i % 2 === 0 ? "bg-slate-900/30" : "bg-slate-900/10";
        return (
          <div key={i} className={`grid grid-cols-3 gap-3 items-center px-2 py-1.5 rounded ${zebra}`}>
            <div className="text-slate-300 text-sm font-mono truncate" title={m.source}>{m.source}</div>
            <div className="text-slate-400 text-sm text-center">mapped →</div>
            <div className="text-slate-300 text-sm font-mono truncate" title={m.target}>
              {m.target}
              {m.mode ? ` (${m.mode}${m.rw === false ? ", ro" : ""})` : (m.rw === false ? " (ro)" : "")}
            </div>
          </div>
        );
      })}
    </div>
  );
}

type NetRow = { name: string; ip?: string; gateway?: string; mac?: string };

function normalizeNetworks(c: InspectOut): NetRow[] {
  const anyC: any = c as any;

  // Preferred shapes
  if (Array.isArray(anyC.networks_detail)) {
    return anyC.networks_detail.map((n: any) => ({
      name: n.name ?? "",
      ip: n.ip ?? n.ip_address ?? n.ipv4 ?? "",
      gateway: n.gateway ?? "",
      mac: n.mac ?? n.mac_address ?? "",
    }));
  }
  if (anyC.networks_map && typeof anyC.networks_map === "object") {
    return Object.entries(anyC.networks_map).map(([name, n]: any) => ({
      name,
      ip: n?.ip ?? n?.ip_address ?? n?.IPAddress ?? "",
      gateway: n?.gateway ?? n?.Gateway ?? "",
      mac: n?.mac ?? n?.mac_address ?? n?.MacAddress ?? "",
    }));
  }

  // Fallback: list of names only
  if (Array.isArray(c.networks)) {
    return (c.networks as string[]).map((name) => ({ name }));
  }
  return [];
}

function NetworksBlock({ c }: { c: InspectOut }) {
  const nets = normalizeNetworks(c);
  if (!nets.length) return <div className="text-sm text-slate-500">No networks.</div>;

  return (
    <div className="space-y-1">
      {/* Header */}
      <div className="grid grid-cols-4 gap-3 px-2 py-1.5">
        <div className="text-xs text-slate-400">Name</div>
        <div className="text-xs text-slate-400">IP Address</div>
        <div className="text-xs text-slate-400">Gateway</div>
        <div className="text-xs text-slate-400">MAC Address</div>
      </div>
      {nets.map((n, i) => {
        const zebra = i % 2 === 0 ? "bg-slate-900/30" : "bg-slate-900/10";
        return (
          <div key={`${n.name}-${i}`} className={`grid grid-cols-4 gap-3 px-2 py-1.5 rounded ${zebra}`}>
            <div className="text-slate-300 text-sm truncate" title={n.name}>{n.name || "—"}</div>
            <div className="text-slate-300 text-sm truncate" title={n.ip}>{n.ip || "—"}</div>
            <div className="text-slate-300 text-sm truncate" title={n.gateway}>{n.gateway || "—"}</div>
            <div className="text-slate-300 text-sm truncate font-mono" title={n.mac}>{n.mac || "—"}</div>
          </div>
        );
      })}
    </div>
  );
}

/* ---------- Container Card (per-container state, status pill, sections) ---------- */

function ContainerCard({
  c,
}: {
  c: InspectOut;
}) {
  const [revealEnvAll, setRevealEnvAll] = useState(false);

  const envEntries = Object.entries(c.env || {});
  const labelEntries = Object.entries(c.labels || {}).sort(([a], [b]) => a.localeCompare(b));
  const envCount = envEntries.length;
  const labelCount = labelEntries.length;
  const volCount = (c.volumes || []).length;
  const netsCount = normalizeNetworks(c).length;

  const statusText = (c as any).state || (c as any).status || "unknown";

  return (
    <div className="rounded-lg border border-slate-800 p-3 space-y-3">
      {/* Header with name and status pill */}
      <div className="flex items-center justify-between">
        <div className="font-medium text-slate-200 text-sm">{c.name}</div>
        <div><StatePill state={statusText} /></div>
      </div>

      {/* Top details: Image, Ports */}
      <div className="space-y-1">
        <RowShell index={0}>
          <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">Image</div>
          <div className="col-span-9 text-slate-300 text-sm font-mono truncate" title={c.image || ""}>{c.image || "—"}</div>
        </RowShell>
        <RowShell index={1}>
          <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">Ports</div>
          <div className="col-span-9"><PortsBlock ports={c.ports} /></div>
        </RowShell>
      </div>

      {/* GENERAL collapsible */}
      <CollapsibleSection title="General" defaultOpen={false}>
        <div className="space-y-1">
          <RowShell index={0}>
            <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">Created</div>
            <div className="col-span-9 text-slate-300 text-sm">
              {(c as any).created || (c as any).created_at
                ? formatDT((c as any).created || (c as any).created_at)
                : "—"}
            </div>
          </RowShell>
          <RowShell index={1}>
            <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">Start time</div>
            <div className="col-span-9 text-slate-300 text-sm">
              {(c as any).started || (c as any).started_at || (c as any).start_time
                ? formatDT((c as any).started || (c as any).started_at || (c as any).start_time)
                : "—"}
            </div>
          </RowShell>
          <RowShell index={3}>
            <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">Restart policy</div>
            <div className="col-span-9 text-slate-300 text-sm">{c.restart_policy || "—"}</div>
          </RowShell>
          <RowShell index={4}>
            <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">CMD</div>
            <div className="col-span-9">
              <ValueBox value={(c.cmd || []).join(" ") || "—"} />
            </div>
          </RowShell>
          <RowShell index={5}>
            <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">ENTRYPOINT</div>
            <div className="col-span-9">
              <ValueBox value={(c.entrypoint || []).join(" ") || "—"} />
            </div>
          </RowShell>
          <RowShell index={6}>
            <div className="col-span-3 text-slate-400 text-xs uppercase tracking-wide">ID</div>
            <div
              className="col-span-9 text-slate-300 text-sm font-mono truncate"
              title={(c as any).id || (c as any).container_id || ""}
            >
              {(c as any).id || (c as any).container_id || "—"}
            </div>
          </RowShell>
        </div>
      </CollapsibleSection>

      {/* Environment with per-container bulk reveal toggle */}
      <CollapsibleSection
        title="Environment Variables"
        count={envCount}
        rightAction={
          envCount > 0 ? (
            <Button
              size="icon"
              variant="ghost"
              className="h-7 w-7"
              onClick={(e) => {
                e.stopPropagation();
                setRevealEnvAll((v) => !v);
              }}
              title={revealEnvAll ? "Hide all" : "Reveal all"}
            >
              {revealEnvAll ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </Button>
          ) : null
        }
      >
        {envCount === 0 ? (
          <div className="text-sm text-slate-500">No environment variables.</div>
        ) : (
          <div className="space-y-1">
            {envEntries.map(([k, v], idx) => (
              <EnvRow key={k} k={k} v={v} forceShow={revealEnvAll} index={idx} />
            ))}
          </div>
        )}
      </CollapsibleSection>

      {/* Labels */}
      <CollapsibleSection title="Labels" count={labelCount}>
        {labelCount === 0 ? (
          <div className="text-sm text-slate-500">No labels.</div>
        ) : (
          <div className="space-y-1">
            {labelEntries.map(([k, v], idx) => (
              <LabelRow key={k} k={k} v={v} index={idx} />
            ))}
          </div>
        )}
      </CollapsibleSection>

      {/* Networks */}
      <CollapsibleSection title="Networks" count={netsCount}>
        <NetworksBlock c={c} />
      </CollapsibleSection>

      {/* Volumes */}
      <CollapsibleSection title="Volumes" count={volCount}>
        <VolsBlock vols={c.volumes} />
      </CollapsibleSection>
    </div>
  );
}

/* ---------- Page ---------- */

export default function StackDetailView({
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

  const [deploying, setDeploying] = useState<boolean>(false);
  const [watching, setWatching] = useState<boolean>(false);
  const watchTimer = useRef<number | null>(null);
  const watchUntil = useRef<number>(0);

  useEffect(() => { setAutoDevOps(false); }, [stackName]);

  async function refreshFiles() {
    if (!stackIacId) return;
    const r = await fetch(`/api/iac/stacks/${stackIacId}/files`, { credentials: "include" });
    if (!r.ok) return;
    const j = await r.json();
    setFiles(j.files || []);
  }

  // Dedicated runtime refresh (containers + inspect)
  async function refreshRuntime() {
    try {
      const rc = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers`, { credentials: "include" });
      if (rc.status === 401) { window.location.replace("/auth/login"); return; }
      const contJson = await rc.json();
      const runtimeAll: ApiContainer[] = (contJson.items || []) as ApiContainer[];
      const my = runtimeAll.filter(c => (c.compose_project || c.stack || "(none)") === stackName);
      setRuntime(my);

      const ins: InspectOut[] = [];
      for (const c of my) {
        const r = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(c.name)}/inspect`, { credentials: "include" });
        if (!r.ok) continue;
        ins.push(await r.json());
      }
      setContainers(ins);
    } catch (e) {
      // soft-fail
      // eslint-disable-next-line no-console
      console.warn("refreshRuntime failed", e);
    }
  }

  // Load runtime + files on mount/changes
  useEffect(() => {
    let cancel = false;
    (async () => {
      setLoading(true); setErr(null);
      try {
        await refreshRuntime();
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

  // Load EFFECTIVE Auto DevOps
  useEffect(() => {
    let cancel = false;
    (async () => {
      if (!stackIacId) { setAutoDevOps(false); return; }
      try {
        const r = await fetch(`/api/iac/stacks/${stackIacId}`, { credentials: "include" });
        if (!r.ok) return;
        const j = await r.json();
        if (!cancel) setAutoDevOps(!!j?.stack?.effective_auto_devops);
      } catch { /* ignore */ }
    })();
    return () => { cancel = true; };
  }, [stackIacId]);

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
    const r = await fetch(`/api/iac/stacks/${stackIacId}`, { method: "DELETE", credentials: "include" });
    if (!r.ok) { alert(`Failed to delete: ${r.status} ${r.statusText}`); return; }
    setStackIacId(undefined);
    setFiles([]);
    setEditPath(null);
  }

  // Toggle stack Auto DevOps OVERRIDE
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
    if (checked && files.length === 0) {
      alert("This stack needs compose files or services before Auto DevOps can be enabled. Add content first.");
      return;
    }
    setAutoDevOps(checked);
    await fetch(`/api/iac/stacks/${id}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_devops: checked }),
    });
  }

  // Begin/stop short-lived watch that polls runtime so user sees changes roll in.
  function beginWatch(durationMs = 120_000, intervalMs = 3_000) {
    // clear any existing
    if (watchTimer.current) {
      window.clearTimeout(watchTimer.current);
      watchTimer.current = null;
    }
    setWatching(true);
    watchUntil.current = Date.now() + durationMs;

    const tick = async () => {
      await refreshRuntime();
      if (Date.now() < watchUntil.current) {
        watchTimer.current = window.setTimeout(tick, intervalMs);
      } else {
        setWatching(false);
        watchTimer.current = null;
      }
    };
    // immediate refresh and schedule
    tick();
  }

  function stopWatch() {
    if (watchTimer.current) {
      window.clearTimeout(watchTimer.current);
      watchTimer.current = null;
    }
    setWatching(false);
  }

  useEffect(() => {
    return () => {
      // cleanup on unmount
      if (watchTimer.current) {
        window.clearTimeout(watchTimer.current);
        watchTimer.current = null;
      }
    };
  }, []);

  async function deployNow() {
    if (!stackIacId) { alert("Create the stack (save a file) before deploying."); return; }
    if (files.length === 0) { alert("This stack has no files to deploy. Add a compose file or scripts first."); return; }
    setDeploying(true);
    try {
      // Manual deploy (server treats manual as default when auto param is absent)
      const r = await fetch(`/api/iac/stacks/${stackIacId}/deploy`, { method: "POST", credentials: "include" });
      const txt = await r.text();
      if (!r.ok) { alert(`Deploy failed: ${r.status} ${txt}`); return; }

      // Kick off a watch so the Active Containers panel keeps refreshing for a bit.
      beginWatch(120_000, 3_000);
    } catch (e: any) {
      alert(`Deploy failed: ${e?.message || e}`);
    } finally {
      setDeploying(false);
    }
  }

  const hasContent = files.some(f => f.role === "compose") || files.length > 0;

  // "New file" helpers: open blank editor with base dir if default exists; otherwise prefill default full path.
  function openNewFile(baseDir: string, defaultName: string) {
    const base = `docker-compose/${host.name}/${stackName}/`;
    const defaultPath = `${base}${defaultName}`;
    const exists = files.some(f => f.rel_path === defaultPath);
    setEditPath(exists ? baseDir : defaultPath);
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button variant="outline" className="border-slate-700 text-slate-200 hover:bg-slate-800" onClick={onBack}>
          <ArrowLeft className="h-4 w-4 mr-1" /> Back to {host.name}
        </Button>
        <div className="ml-2 text-lg font-semibold text-white">Stack: {stackName}</div>
        <div className="ml-auto flex items-center gap-3">
          <span className="text-sm text-slate-300">Auto DevOps</span>
          <Switch checked={autoDevOps} onCheckedChange={(v) => toggleAutoDevOps(!!v)} />
          {stackIacId ? (
            <Button
              onClick={async () => { await refreshRuntime(); await refreshFiles(); }}
              variant="outline"
              className="border-slate-700"
              title="Refresh files and active containers"
            >
              <RefreshCw className="h-4 w-4 mr-1" /> Refresh
            </Button>
          ) : null}
          {watching && (
            <span className="ml-1 inline-flex items-center gap-2 text-xs text-emerald-300">
              <RefreshCw className="h-3 w-3 animate-spin" />
              Watching updates…
              <Button size="xs" variant="ghost" className="h-6 px-2 text-emerald-200" onClick={stopWatch} title="Stop watching">
                <Square className="h-3 w-3 mr-1" /> Stop
              </Button>
            </span>
          )}
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
            </CardHeader>
            <CardContent className="space-y-3">
              {containers.length === 0 && (
                <div className="text-sm text-slate-500">
                  No containers are currently running for this stack on {host.name}.
                </div>
              )}
              {containers.map((c, i) => (
                <ContainerCard key={i} c={c} />
              ))}
            </CardContent>
          </Card>
        </div>

        {/* Right: IaC Files / Editor */}
        <div className="space-y-4 lg:sticky lg:top-4 lg:h-[calc(100vh-140px)] lg:z-10">
          <Card className="bg-slate-900/50 border-slate-800 h-full flex flex-col">
            <CardHeader className="pb-2 shrink-0 flex flex-row items-center justify-between">
              <CardTitle className="text-slate-200 text-lg">IaC Files</CardTitle>
              {/* (Deploy button removed from header earlier to avoid redundancy) */}
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
                    onClick={() => openNewFile(`docker-compose/${host.name}/${stackName}/`, "docker-compose.yaml")}
                  >
                    New compose
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="border-slate-700"
                    onClick={() => openNewFile(`docker-compose/${host.name}/${stackName}/`, ".env")}
                  >
                    New env
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="border-slate-700"
                    onClick={() => openNewFile(`docker-compose/${host.name}/${stackName}/`, "deploy.sh")}
                  >
                    New script
                  </Button>

                  {/* Delete IaC sits left of Deploy */}
                  {stackIacId ? (
                    <Button onClick={deleteStack} variant="outline" className="border-rose-700 text-rose-200">
                      <Trash2 className="h-4 w-4 mr-1" /> Delete IaC
                    </Button>
                  ) : null}

                  {/* Deploy sits to the right of New Script (and Delete IaC) */}
                  {hasContent && (
                    <Button
                      onClick={deployNow}
                      disabled={deploying}
                      size="sm"
                      className="bg-emerald-800 hover:bg-emerald-900 text-white"
                      title="Deploy this stack"
                    >
                      <Rocket className={`h-4 w-4 mr-1 ${deploying ? "animate-spin" : ""}`} />
                      {deploying ? "Deploying..." : "Deploy"}
                    </Button>
                  )}
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
                            <Button size="sm" variant="outline" className="border-slate-700" onClick={() => setEditPath(f.rel_path)}>Edit</Button>
                            <Button
                              size="icon"
                              variant="ghost"
                              onClick={async () => {
                                if (!stackIacId) return;
                                const r = await fetch(`/api/iac/stacks/${stackIacId}/file?path=${encodeURIComponent(f.rel_path)}`, { method: "DELETE", credentials: "include" });
                                if (r.ok) refreshFiles();
                              }}
                              title="Delete"
                            >
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
                    fileMeta={files.find(f => f.rel_path === editPath)}
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
