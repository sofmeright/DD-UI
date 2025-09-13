// ui/src/views/HostsStacksView.tsx
// ui/src/views/HostsStacksView.tsx
import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import HostPicker from "@/components/HostPicker";
import {
  ChevronRight,
  FileText,
  Activity,
  Bug,
  Pause,
  Play,
  PlayCircle,
  RefreshCw,
  RotateCw,
  Terminal,
  Trash2,
  ZapOff,
  Eye,
} from "lucide-react";
import StatePill from "@/components/StatePill";
import DriftBadge from "@/components/DriftBadge";
import ActionBtn from "@/components/ActionBtn";
import LiveLogsModal from "@/components/LiveLogsModal";
import ConsoleModal from "@/components/ConsoleModal";
import { ApiContainer, Host, IacService, IacStack, MergedRow, MergedStack } from "@/types";
import { formatDT, formatPortsLines } from "@/utils/format";
import { debugLog, warnLog } from "@/utils/logging";

function sanitizeLabel(s: string): string {
  // match compose label semantics: lowercase, spaces -> _, only [a-z0-9_-]
  const lowered = (s || "").trim().toLowerCase().replaceAll(" ", "_");
  const stripped = lowered.replace(/[^a-z0-9_-]/g, "_");
  return stripped.replace(/^[-_]+|[-_]+$/g, "") || "default";
}

function isEncryptedOrTemplated(v?: string | null) {
  if (!v) return false;
  return v.startsWith("ENC[") || v.includes("${");
}

function guessServiceFromContainerName(containerName: string, projectLabel: string): string | undefined {
  // strip trailing -N or _N
  let base = containerName.replace(/[-_]\d+$/, "");
  // prefer exact compose_project if available; otherwise use provided projectLabel
  // pattern 1: <project>-<service>
  if (base.startsWith(projectLabel + "-")) return base.slice(projectLabel.length + 1);
  // pattern 2: <project>_<service>
  if (base.startsWith(projectLabel + "_")) return base.slice(projectLabel.length + 1);
  return undefined;
}

export default function HostStacksView({
  host,
  hosts,
  onSync,
  onOpenStack,
  onHostChange,
}: {
  host: Host;
  hosts: Host[];
  onSync: () => void;
  onOpenStack: (stackName: string) => void;
  onHostChange: (hostName: string) => void;
}) {
  debugLog('[DDUI] HostStacksView component mounted for host:', host.name);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [stacks, setStacks] = useState<MergedStack[]>([]);
  const [hostQuery, setHostQuery] = useState("");

  // Live logs & console wiring
  const [streamLogs, setStreamLogs] = useState<{ ctr: string } | null>(null);
  const [consoleTarget, setConsoleTarget] = useState<{ ctr: string; cmd?: string } | null>(null);

  // Lightweight info modal
  const [infoModal, setInfoModal] = useState<{ title: string; text: string } | null>(null);

  function matchRow(r: MergedRow, q: string) {
    if (!q) return true;
    const hay = [r.name, r.state, r.stack, r.imageRun, r.imageIac, r.ip, r.portsText, r.owner]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return hay.includes(q.toLowerCase());
  }

  async function doCtrAction(ctr: string, action: string) {
    try {
      await fetch(
        `/api/containers/hosts/${encodeURIComponent(host.name)}/${encodeURIComponent(ctr)}/action`,
        {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action }),
        }
      );
      onSync();
      setStacks((prev) =>
        prev.map((s) => ({
          ...s,
          rows: s.rows.map((r) =>
            r.name === ctr
              ? {
                  ...r,
                  state:
                    action === "pause"
                      ? "paused"
                      : action === "unpause"
                      ? "running"
                      : action === "stop"
                      ? "exited"
                      : action === "kill"
                      ? "dead"
                      : action === "remove"
                      ? "removed"
                      : action === "start"
                      ? "running"
                      : action === "restart"
                      ? "restarting"
                      : r.state,
                }
              : r
          ),
        }))
      );
    } catch (e) {
      alert("Action failed");
    }
  }

  function openLiveLogs(ctr: string) {
    setStreamLogs({ ctr });
  }

  useEffect(() => {
    debugLog('[DDUI] HostStacksView useEffect starting for host:', host.name);
    let cancel = false;
    (async () => {
      setLoading(true);
      setErr(null);
      try {
        const [rc, ri] = await Promise.all([
          fetch(`/api/containers/hosts/${encodeURIComponent(host.name)}`, { credentials: "include" }),
          fetch(`/api/iac/hosts/${encodeURIComponent(host.name)}`, { credentials: "include" }),
        ]);
        if (rc.status === 401 || ri.status === 401) {
          window.location.replace("/auth/login");
          return;
        }
        const contJson = await rc.json();
        const iacJson = await ri.json();
        const runtime: ApiContainer[] = (contJson.containers || []) as ApiContainer[];
        const iacStacks: IacStack[] = (iacJson.stacks || []) as IacStack[];

        // Enhanced drift/runtime (source drift, rendered services, & config-hash from backend)
        const enhancedByName = new Map<
        string,
        {
          drift_detected: boolean;
          drift_reason?: string;
          effective_auto_devops?: boolean;
          containers?: Array<{ name: string; config_hash?: string }>;
          rendered_services?: Array<{ service_name: string; container_name?: string; image?: string }>;
        }
        >();
        try {
        debugLog('[DDUI] Fetching enhanced-iac data for host:', host.name);
        const re = await fetch(`/api/iac/hosts/${encodeURIComponent(host.name)}/enhanced`, {
          credentials: "include",
        });
        debugLog('[DDUI] Enhanced-iac response status:', re.status, re.statusText);
        if (re.ok) {
          const ej = await re.json();
          debugLog('[DDUI] Enhanced-iac response data:', ej);
          const items = Array.isArray(ej?.stacks) ? ej.stacks : [];
          debugLog('[DDUI] Enhanced-iac found', items.length, 'stacks');
          for (const s of items) {
            const nm = (s?.name || s?.stack_name || "").toString();
            if (!nm) continue;
            const ctrs = Array.isArray(s?.containers) ? s.containers : [];
            const rsv = Array.isArray(s?.rendered_services) ? s.rendered_services : [];
            enhancedByName.set(nm, {
              drift_detected: !!s?.drift_detected,
              drift_reason: s?.drift_reason || "",
              effective_auto_devops: !!s?.effective_auto_devops,
              containers: ctrs.map((c: any) => ({
                name: (c?.name || "").toString(),
                config_hash: (c?.config_hash || "").toString(),
              })),
              rendered_services: rsv.map((rv: any) => ({
                service_name: (rv?.service_name || "").toString(),
                container_name: (rv?.container_name || "").toString() || undefined,
                image: (rv?.image || "").toString() || undefined,
              })),
            });
          }
          debugLog('[DDUI] Enhanced-iac enhancedByName map populated with', enhancedByName.size, 'entries');
        } else {
          warnLog('[DDUI] Enhanced-iac API failed:', re.status, re.statusText);
        }
        } catch (error) {
        errorLog('[DDUI] Enhanced-iac API error:', error);
        }

        // Auto DevOps information is now included in the enhanced-iac endpoint response

        // Build IaC maps (RAW names)
        const iacByStack = new Map<string, IacStack>();
        for (const s of iacStacks) iacByStack.set(s.name, s);

        // Map label->raw so runtime buckets land under the IaC stack
        const labelToRaw = new Map<string, string>();
        for (const s of iacStacks) {
        labelToRaw.set(sanitizeLabel(s.name), s.name);
        }

        // Bucket runtime by RAW stack name when we can map; else by label
        const rtByStack = new Map<string, ApiContainer[]>();
        for (const c of runtime) {
        const label = (c.compose_project || c.stack || "(none)").trim() || "(none)";
        const key = label !== "(none)" ? labelToRaw.get(sanitizeLabel(label)) || label : label;
        if (!rtByStack.has(key)) rtByStack.set(key, []);
        rtByStack.get(key)!.push(c);
        }

        // config-hash by container name (from enhanced) — keep using this map
        const cfgHashByName = new Map<string, string>();
        for (const [, e] of enhancedByName.entries()) {
        for (const c of e.containers || []) {
          if (c?.name && c?.config_hash) cfgHashByName.set(c.name, c.config_hash);
        }
        }

        // Union of names (normalized)
        const names = new Set<string>([...rtByStack.keys(), ...iacByStack.keys()]);
        const merged: MergedStack[] = [];

        debugLog('Processing stacks:', Array.from(names));

        for (const sname of Array.from(names).sort()) {
        const rcs = rtByStack.get(sname) || [];
        const is = iacByStack.get(sname);

        // Prefer fully-rendered (post-SOPS, post-interpolation) services from enhanced API.
        const enh = enhancedByName.get(sname);
        // Fallback to DB services only if nothing rendered
        const rawSvcs: IacService[] = Array.isArray(is?.services) ? (is!.services as IacService[]) : [];
        
        // Try rendered_services from enhanced API first, then from basic API (since basic now uses enhanced logic)
        const enhancedRenderedServices = enh?.rendered_services || [];
        // Check both camelCase and snake_case for compatibility
        const basicRenderedServices = Array.isArray((is as any)?.rendered_services) 
          ? (is as any).rendered_services 
          : Array.isArray((is as any)?.renderedServices) 
            ? (is as any).renderedServices 
            : [];
        const allRenderedServices = enhancedRenderedServices.length > 0 ? enhancedRenderedServices : basicRenderedServices;
        
        if (sname === 'it-tools') {
          debugLog(`[DDUI] DETAILED it-tools analysis:`, {
            enhanced_data: enh,
            basic_stack_full: is,
            enhanced_rendered_services: enhancedRenderedServices,
            basic_rendered_services: basicRenderedServices,
            all_rendered_services: allRenderedServices,
            raw_services: rawSvcs
          });
        }
        
        debugLog(`[DDUI] Processing stack: ${sname}`, {
          enhanced_count: enhancedRenderedServices.length,
          basic_count: basicRenderedServices.length,
          raw_count: rawSvcs.length,
          using_enhanced: enhancedRenderedServices.length > 0,
          has_rendered_services: (is as any)?.rendered_services !== undefined,
          has_renderedServices: (is as any)?.renderedServices !== undefined,
          basic_stack_data: is ? Object.keys(is) : [],
          enhanced_api_data: enh ? Object.keys(enh) : [],
          basic_rendered_services_sample: basicRenderedServices.slice(0, 1),
          enhanced_rendered_services_sample: enhancedRenderedServices.slice(0, 1)
        });
        
        if (allRenderedServices.length > 0) {
          allRenderedServices.forEach((svc, idx) => {
            const hasEncrypted = svc.image && (svc.image.includes('ENC[') || svc.image.includes('${'));
            debugLog(`[DDUI] ${sname}/${svc.service_name}: ${hasEncrypted ? '🔒' : '✅'} ${svc.image || 'no-image'}`);
          });
        }
        
        const rendered = allRenderedServices.map((rv: any) => ({
          service_name: rv.service_name,
          container_name: rv.container_name,
          image: rv.image,
        }));
        const servicesResolved: Array<{ service_name: string; container_name?: string; image?: string }> =
          rendered.length > 0
            ? rendered
            : rawSvcs.map((x) => ({
                service_name: x.service_name,
                container_name: x.container_name || undefined,
                image: x.image || undefined,
              }));

        const hasIac = !!is && (servicesResolved.length > 0 || !!is.compose);
        const hasContent = !!is && (!!is.compose || servicesResolved.length > 0);

        const rows: MergedRow[] = [];
        const projectLabel = sanitizeLabel(sname);

        function guessServiceFromContainerName(containerName: string, projectLabel: string): string | undefined {
          let base = containerName.replace(/[-_]\d+$/, "");
          if (base.startsWith(projectLabel + "-")) return base.slice(projectLabel.length + 1);
          if (base.startsWith(projectLabel + "_")) return base.slice(projectLabel.length + 1);
          return undefined;
        }

        function desiredImageFor(c: ApiContainer): string | undefined {
          if (servicesResolved.length === 0) return undefined;
          const reported = (c as any).compose_service as string | undefined;
          const guessed = reported || guessServiceFromContainerName(c.name, c.compose_project || projectLabel);
          const svc = servicesResolved.find(
            (x) =>
              (guessed && x.service_name === guessed) ||
              (!!x.container_name && x.container_name === c.name)
          );
          return svc?.image || undefined;
        }

        // Runtime rows
        for (const c of rcs) {
          const portsLines = formatPortsLines((c as any).ports);
          const portsText = portsLines.join("\n");
          const _desired = desiredImageFor(c);

          rows.push({
            name: c.name,
            state: c.state,
            stack: sname,
            imageRun: c.image,
            imageIac: undefined, // we do NOT compare images for drift on existing rows
            created: formatDT(c.created_ts),
            ip: c.ip_addr,
            portsText,
            owner: c.owner || "—",
            // @ts-ignore
            _desiredImage: _desired,
          } as any);
        }

        // Missing service rows (only when not found in runtime)
        for (const svc of servicesResolved) {
          const exists = (rcs || []).some((c) => {
            if (svc.container_name) {
              return c.name === svc.container_name;
            }
            const reported = (c as any).compose_service as string | undefined;
            const guessed = reported || guessServiceFromContainerName(c.name, c.compose_project || projectLabel);
            return guessed === svc.service_name;
          });

          if (!exists) {
            debugLog(`[DDUI] Missing service in ${sname}:`, {
              service_name: svc.service_name,
              container_name: svc.container_name,
              image: svc.image,
              is_encrypted: svc.image?.includes('ENC['),
              using_rendered: rendered.length > 0,
              rendered_count: rendered.length,
              raw_count: rawSvcs.length
            });
            // Skip adding missing service rows if they contain encrypted values
            // This indicates we're using raw services when we should be using rendered
            if (svc.image?.includes('ENC[')) {
              warnLog(`[DDUI] Skipping encrypted missing service ${svc.service_name} - check rendered services`);
              continue;
            }
            rows.push({
              name: svc.container_name || svc.service_name,
              state: "missing",
              stack: sname,
              imageRun: undefined,
              imageIac: svc.image, // <- resolved (post-SOPS, post-interpolation)
              created: "—",
              ip: "—",
              portsText: "—",
              owner: "—",
            });
          }
        }

        let stackDrift: "in_sync" | "drift" | "unknown" = "unknown";
        if (enh) {
          stackDrift = enh.drift_detected ? "drift" : "in_sync";
        } else if (hasIac) {
          stackDrift = "unknown";
        }

        const effectiveAuto = enh?.effective_auto_devops ? true : false;

        const mergedRow: MergedStack = {
          name: sname,
          drift: stackDrift,
          iacEnabled: effectiveAuto,
          pullPolicy: hasIac ? is?.pull_policy : undefined,
          sops: hasIac ? is?.sops_status === "all" : false,
          deployKind: hasIac ? is?.deploy_kind || "compose" : sname === "(none)" ? "unmanaged" : "unmanaged",
          rows,
          iacId: (is as any)?.id,
          hasIac,
          hasContent,
        };

        // @ts-ignore tooltip reason
        if (enh?.drift_reason) (mergedRow as any).driftReason = enh.drift_reason;

        merged.push(mergedRow);
        }

        if (!cancel) setStacks(merged);
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load host stacks");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => {
      cancel = true;
    };
  }, [host.name, onSync]);

  // --- Name validation helpers (warn-only; Compose will normalize) ---
  function dockerSanitizePreview(s: string): string {
    const lowered = s.trim().toLowerCase().replaceAll(" ", "_");
    return lowered.replace(/[^a-z0-9_-]/g, "_") || "default";
  }
  function hasUnsupportedChars(s: string): boolean {
    return /[^A-Za-z0-9 _-]/.test(s);
  }

  async function createStackFlow() {
    const existing = new Set(stacks.map((s) => s.name));
    let name = prompt("New stack name:");
    if (!name) return;
    name = name.trim();
    if (!name) return;

    if (existing.has(name)) {
      alert("A stack with that name already exists.");
      return;
    }

    if (hasUnsupportedChars(name)) {
      const preview = dockerSanitizePreview(name);
      const ok = confirm(
        `Heads up: Docker Compose will normalize this name for the project label.\n` +
          `You entered:  "${name}"\n` +
          `Compose uses: "${preview}"\n\nProceed with "${name}"?`
      );
      if (!ok) return;
    }

    try {
      const r = await fetch(`/api/iac/hosts/${encodeURIComponent(host.name)}/stacks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          stack_name: name, // store EXACT name as entered; Compose sanitizes at deploy
          iac_enabled: false,
        }),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      const j = await r.json();
      onOpenStack(name, j.id);
    } catch (e: any) {
      alert(e?.message || "Failed to create stack");
    }
  }

  async function setAutoDevOps(stackName: string, enabled: boolean) {
    const r = await fetch(`/api/iac/hosts/${encodeURIComponent(host.name)}/stacks/${encodeURIComponent(stackName)}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_devops: enabled }),
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
    setStacks((prev) => prev.map((row, i) => (i === sIndex ? { ...row, iacEnabled: enabled } : row)));
    setAutoDevOps(s.name, enabled).catch((err) => {
      alert(`Failed to update Auto DevOps: ${err?.message || err}`);
      setStacks((prev) =>
        prev.map((row, i) => (i === sIndex ? { ...row, iacEnabled: !enabled } : row))
      );
    });
  }

  async function deleteStackAt(index: number) {
    const s = stacks[index];
    if (!s.iacId) return;
    if (!confirm(`Delete IaC for stack "${s.name}"? This removes IaC files/metadata but not runtime containers.`))
      return;
    const r = await fetch(`/api/iac/hosts/${encodeURIComponent(host.name)}/stacks/${encodeURIComponent(s.name)}`, { method: "DELETE", credentials: "include" });
    if (!r.ok) {
      alert(`Failed to delete: ${r.status} ${r.statusText}`);
      return;
    }
    setStacks((prev) =>
      prev.map((row, i) =>
        i === index
          ? {
              ...row,
              iacId: undefined,
              hasIac: false,
              iacEnabled: false,
              pullPolicy: undefined,
              sops: false,
              drift: "unknown",
              hasContent: false,
            }
          : row
      )
    );
  }

  useEffect(() => {
    if (!infoModal) return;
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === "Escape") setInfoModal(null);
    };
    window.addEventListener("keydown", onEsc);
    return () => window.removeEventListener("keydown", onEsc);
  }, [infoModal]);

  return (
    <div className="space-y-4">
      {/* Streaming Logs Modal */}
      {streamLogs && (
        <LiveLogsModal host={host.name} container={streamLogs.ctr} onClose={() => setStreamLogs(null)} />
      )}

      {/* Console Modal */}
      {consoleTarget && (
        <ConsoleModal host={host.name} container={consoleTarget.ctr} onClose={() => setConsoleTarget(null)} />
      )}

      {/* Info Modal */}
      {infoModal && (
        <div
          className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="info-title"
          onClick={() => setInfoModal(null)}
        >
          <div
            className="bg-slate-950 border border-slate-800 rounded-xl w-full max-w-3xl p-4"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-2">
              <div className="text-slate-200 font-semibold" id="info-title">
                {infoModal.title}
              </div>
              <Button size="sm" variant="outline" className="border-slate-700" onClick={() => setInfoModal(null)}>
                Close
              </Button>
            </div>
            <pre className="text-xs text-slate-300 bg-slate-900 border border-slate-800 rounded p-3 max-h-[60vh] overflow-auto whitespace-pre-wrap">
              {infoModal.text}
            </pre>
          </div>
        </div>
      )}

      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <div className="text-lg font-semibold text-white">Stacks</div>
          <HostPicker hosts={hosts} activeHost={host.name} setActiveHost={onHostChange} />
        </div>
        <div className="flex items-center gap-2">
          <Button onClick={onSync} className="bg-[#310937] hover:bg-[#2a0830] text-white">
            <RefreshCw className="h-4 w-4 mr-1" /> Sync
          </Button>
          <Button onClick={createStackFlow} variant="outline" className="border-slate-700 text-slate-200">
            <Eye className="hidden" />
            New Stack
          </Button>
          <div className="relative w-72">
            <Input
              value={hostQuery}
              onChange={(e) => setHostQuery(e.target.value)}
              placeholder={`Search ${host.name}…`}
              className="pl-3 bg-slate-900/50 border-slate-800 text-slate-200 placeholder:text-slate-500"
            />
          </div>
        </div>
      </div>

      {loading && (
        <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300">
          Loading stacks…
        </div>
      )}
      {err && (
        <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">
          Error: {err}
        </div>
      )}

      {stacks
        .filter((s) => {
          // Hide stack cards if search is active and no containers match
          if (!hostQuery.trim()) return true;
          const matchingRows = s.rows.filter((r) => matchRow(r, hostQuery));
          return matchingRows.length > 0;
        })
        .map((s, idx) => (
        <Card key={`${host.name}:${s.name}`} className="bg-slate-900/50 border-slate-800 rounded-xl">
          <CardHeader className="pb-2 flex flex-row items-center justify-between">
            <div className="space-y-1">
              <CardTitle className="text-xl text-white">
                <button className="hover:underline" onClick={() => onOpenStack(s.name)}>
                  {s.name}
                </button>
              </CardTitle>
              <div className="flex items-center gap-2">
                <span title={(s as any).driftReason || ""}>{DriftBadge(s.drift)}</span>
                <Badge variant="outline" className="border-slate-700 text-slate-300">
                  {s.deployKind || "unknown"}
                </Badge>
                <Badge variant="outline" className="border-slate-700 text-slate-300">
                  pull: {s.hasIac ? s.pullPolicy || "—" : "—"}
                </Badge>
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
              <label htmlFor={`auto-${idx}`} className="text-sm text-slate-300">
                Auto DevOps
              </label>
              <Switch
                id={`auto-${idx}`}
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
                  {s.rows
                    .filter((r) => matchRow(r, hostQuery))
                    .map((r) => {
                      const st = (r.state || "").toLowerCase();
                      const isRunning =
                        st.includes("running") || st.includes("up") || st.includes("healthy") || st.includes("restarting");
                      const isPaused = st.includes("paused");
                      return (
                        <tr key={r.name} className="border-t border-slate-800 hover:bg-slate-900/40 align-top">
                          <td className="px-2 py-1.5 font-medium text-slate-200 truncate">{r.name}</td>
                          <td className="px-2 py-1.5 text-slate-300">
                            <StatePill state={r.state} />
                          </td>
                          <td className="px-2 py-1.5 text-slate-300">
                            <div className="flex items-center gap-2">
                              <div className="max-w-[24rem] truncate" title={r.imageRun || ""}>
                                {r.imageRun || "—"}
                              </div>
                              {r.imageIac && r.state === "missing" && (
                                <>
                                  <ChevronRight className="h-4 w-4 text-slate-500" />
                                  <div className="max-w-[24rem] truncate text-slate-300" title={r.imageIac}>
                                    {r.imageIac}
                                  </div>
                                </>
                              )}
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-slate-300">{r.created || "—"}</td>
                          <td className="px-2 py-1.5 text-slate-300">{r.ip || "—"}</td>
                          <td className="px-2 py-1.5 text-slate-300 align-top">
                            <div className="max-w-56 whitespace-pre-line leading-tight">{r.portsText || "—"}</div>
                          </td>
                          <td className="px-2 py-1.5 text-slate-300">{r.owner || "—"}</td>
                          <td className="px-2 py-1">
                            <div className="flex items-center gap-1 overflow-x-auto whitespace-nowrap py-0.5">
                              {r.state === "missing" ? (
                                // For missing services, only show inspect action
                                <>
                                  <ActionBtn title="Inspect" icon={Bug} onClick={() => onOpenStack(s.name)} />
                                  <span className="text-slate-500 text-xs ml-2">Service not running</span>
                                </>
                              ) : (
                                // Normal container actions for running/stopped containers
                                <>
                                  {!isRunning && !isPaused && (
                                    <ActionBtn title="Start" icon={Play} onClick={() => doCtrAction(r.name, "start")} />
                                  )}
                                  {isRunning && (
                                    <ActionBtn title="Stop" icon={Pause} onClick={() => doCtrAction(r.name, "stop")} />
                                  )}
                                  {(isRunning || isPaused) && (
                                    <ActionBtn
                                      title="Restart"
                                      icon={RotateCw}
                                      onClick={() => doCtrAction(r.name, "restart")}
                                    />
                                  )}
                                  {isRunning && !isPaused && (
                                    <ActionBtn title="Pause" icon={Pause} onClick={() => doCtrAction(r.name, "pause")} />
                                  )}
                                  {isPaused && (
                                    <ActionBtn
                                      title="Resume"
                                      icon={PlayCircle}
                                      onClick={() => doCtrAction(r.name, "unpause")}
                                    />
                                  )}

                                  <span className="mx-1 h-4 w-px bg-slate-700/60" />

                                  <ActionBtn title="Live logs" icon={FileText} onClick={() => openLiveLogs(r.name)} />
                                  <ActionBtn title="Inspect" icon={Bug} onClick={() => onOpenStack(s.name)} />
                                  <ActionBtn
                                    title="Stats"
                                    icon={Activity}
                                    onClick={async () => {
                                      try {
                                        const r2 = await fetch(
                                          `/api/containers/hosts/${encodeURIComponent(host.name)}/${encodeURIComponent(r.name)}/stats`,
                                          { credentials: "include" }
                                        );
                                        const txt = await r2.text();
                                        setInfoModal({ title: `${r.name} (stats)`, text: txt || "(no data)" });
                                      } catch {
                                        setInfoModal({ title: `${r.name} (stats)`, text: "(failed to load stats)" });
                                      }
                                    }}
                                  />

                                  <span className="mx-1 h-4 w-px bg-slate-700/60" />

                                  <ActionBtn title="Kill" icon={ZapOff} onClick={() => doCtrAction(r.name, "kill")} />
                                  <ActionBtn title="Remove" icon={Trash2} onClick={() => doCtrAction(r.name, "remove")} />
                                  <ActionBtn
                                    title="Console"
                                    icon={Terminal}
                                    onClick={() => setConsoleTarget({ ctr: r.name })}
                                    disabled={!isRunning}
                                  />
                                </>
                              )}
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  {(!s.rows || s.rows.filter((r) => matchRow(r, hostQuery)).length === 0) && (
                    <tr>
                      <td className="p-3 text-slate-500" colSpan={8}>
                        No containers or services.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
            <div className="pt-2 text-xs text-slate-400">Tip: click the stack title to open the full compare & editor view.</div>
          </CardContent>
        </Card>
      ))}

      <Card className="bg-slate-900/40 border-slate-800">
        <CardContent className="py-4 flex flex-wrap items-center gap-3 text-sm text-slate-300">
          Security by default:
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">AGE key never persisted</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Decrypt to tmpfs only</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
        </CardContent>
      </Card>
    </div>
  );
}
