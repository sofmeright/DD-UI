// ui/src/views/HostsStacksView.tsx
import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
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
import { ApiContainer, Host, IacStack, MergedRow, MergedStack } from "@/types";
import { formatDT, formatPortsLines } from "@/utils/format";

type EnhancedStackWire = {
  name: string;
  drift_detected: boolean;
  drift_reason?: string;
  containers?: Array<{ name: string; service?: string; config_hash?: string }>;
  rendered_services?: Array<{ service_name: string; container_name?: string; image?: string; hash?: string }>;
};

export default function HostStacksView({
  host,
  onSync,
  onOpenStack,
}: {
  host: Host;
  onSync: () => void;
  onOpenStack: (stackName: string, iacId?: number) => void;
}) {
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
        `/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(ctr)}/action`,
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
    } catch {
      alert("Action failed");
    }
  }

  function openLiveLogs(ctr: string) {
    setStreamLogs({ ctr });
  }

  // Local sanitizer to *map* runtime compose_project back to user stack names for grouping
  function sanitizeProject(s: string): string {
    const lowered = s.trim().toLowerCase().replaceAll(" ", "_");
    return lowered.replace(/[^a-z0-9_-]/g, "_").replace(/^[_-]+|[_-]+$/g, "") || "default";
  }

  useEffect(() => {
    let cancel = false;
    (async () => {
      setLoading(true);
      setErr(null);
      try {
        const [rc, ri] = await Promise.all([
          fetch(`/api/hosts/${encodeURIComponent(host.name)}/containers`, { credentials: "include" }),
          fetch(`/api/hosts/${encodeURIComponent(host.name)}/iac`, { credentials: "include" }),
        ]);
        if (rc.status === 401 || ri.status === 401) {
          window.location.replace("/auth/login");
          return;
        }
        const contJson = await rc.json();
        const iacJson = await ri.json();
        const runtime: ApiContainer[] = (contJson.items || []) as ApiContainer[];
        const iacStacks: IacStack[] = (iacJson.stacks || []) as IacStack[];

        // Enhanced drift/runtime (drift + config-hash + *rendered services*)
        const enhancedByName = new Map<
          string,
          {
            drift_detected: boolean;
            drift_reason?: string;
            containers: Array<{ name: string; service?: string; config_hash?: string }>;
            rendered: Array<{ service_name: string; container_name?: string; image?: string; hash?: string }>;
          }
        >();
        try {
          const re = await fetch(`/api/hosts/${encodeURIComponent(host.name)}/enhanced-iac`, {
            credentials: "include",
          });
          if (re.ok) {
            const ej = await re.json();
            const items = Array.isArray(ej?.stacks) ? (ej.stacks as EnhancedStackWire[]) : [];
            for (const s of items) {
              const nm = (s?.name || "").toString();
              if (!nm) continue;
              enhancedByName.set(nm, {
                drift_detected: !!s.drift_detected,
                drift_reason: s.drift_reason || "",
                containers: Array.isArray(s.containers) ? s.containers : [],
                rendered: Array.isArray(s.rendered_services) ? s.rendered_services : [],
              });
            }
          }
        } catch {
          /* ignore; UI still renders with base info */
        }

        // Effective Auto DevOps per stack (for the toggle only)
        const effMap: Record<number, boolean> = {};
        await Promise.all(
          (iacStacks || [])
            .filter((s) => typeof (s as any)?.id === "number")
            .map(async (s) => {
              try {
                const r = await fetch(`/api/iac/stacks/${(s as any).id}`, { credentials: "include" });
                if (!r.ok) return;
                const j = await r.json();
                if (j?.stack?.id) {
                  effMap[j.stack.id] = !!j.stack.effective_auto_devops;
                }
              } catch {
                /* ignore */
              }
            })
        );

        // Build a map of sanitized->raw IaC stack names so we can regroup runtime under **user** names
        const rawBySanitized = new Map<string, string>();
        for (const s of iacStacks) rawBySanitized.set(sanitizeProject(s.name), s.name);

        // Group runtime by the *user* stack name when possible
        const rtByStack = new Map<string, ApiContainer[]>();
        for (const c of runtime) {
          const label = (c.compose_project || "").trim();
          const mapped = label ? rawBySanitized.get(sanitizeProject(label)) : undefined;
          const key = mapped || (c.stack || "(none)").trim() || "(none)";
          if (!rtByStack.has(key)) rtByStack.set(key, []);
          rtByStack.get(key)!.push(c);
        }

        const iacByStack = new Map<string, IacStack>();
        for (const s of iacStacks) iacByStack.set(s.name, s);

        // Fast-lookup maps for enhanced data
        const cfgHashByCtr = new Map<string, string>();
        const renderedByStack = new Map<
          string,
          Array<{ service_name: string; container_name?: string; image?: string; hash?: string }>
        >();
        for (const [sname, e] of enhancedByName.entries()) {
          for (const c of e.containers || []) {
            if (c?.name && c?.config_hash) cfgHashByCtr.set(c.name, c.config_hash);
          }
          if (e.rendered?.length) renderedByStack.set(sname, e.rendered);
        }

        const names = new Set<string>([...rtByStack.keys(), ...iacByStack.keys()]);
        const merged: MergedStack[] = [];

        for (const sname of Array.from(names).sort()) {
          const rcs = rtByStack.get(sname) || [];
          const is = iacByStack.get(sname);
          const hasIac = !!is && (!!is.compose || (Array.isArray((is as any).services) && (is as any).services.length > 0));
          const hasContent = !!is && (!!is.compose || (Array.isArray((is as any).services) && (is as any).services.length > 0));

          const rows: MergedRow[] = [];

          for (const c of rcs) {
            const portsLines = formatPortsLines((c as any).ports);
            const portsText = portsLines.join("\n");
            rows.push({
              name: c.name,
              state: c.state,
              stack: sname,
              imageRun: c.image,
              imageIac: undefined, // no image-compare drift
              created: formatDT(c.created_ts),
              ip: c.ip_addr,
              portsText,
              owner: c.owner || "—",
              drift: false,
              // @ts-ignore: carry config-hash tag
              configHash: cfgHashByCtr.get(c.name) || undefined,
            } as any);
          }

          // Prefer **rendered** services to compute "missing" rows (post-SOPS, post-interpolation)
          const rendered = renderedByStack.get(sname) || [];
          if (rendered.length) {
            for (const svc of rendered) {
              const exists = (rcs || []).some(
                (c) =>
                  (c.compose_service && c.compose_service === svc.service_name) ||
                  (!!svc.container_name && c.name === svc.container_name)
              );
              if (!exists) {
                rows.push({
                  name: svc.container_name || svc.service_name,
                  state: "missing",
                  stack: sname,
                  imageRun: undefined,
                  imageIac: svc.image || "—",
                  created: "—",
                  ip: "—",
                  portsText: "—",
                  owner: "—",
                  drift: true,
                });
              }
            }
          }

          const enh = enhancedByName.get(sname);
          let stackDrift: "in_sync" | "drift" | "unknown" = "unknown";
          if (enh) {
            stackDrift = enh.drift_detected ? "drift" : "in_sync";
          } else if (hasIac) {
            stackDrift = "unknown";
          }

          const effectiveAuto =
            is && typeof (is as any).id === "number" && effMap[(is as any).id] ? true : false;

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

          // @ts-ignore drift reason tooltip
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

  // --- Name validation helpers (warn-only; Compose will normalize), storage keeps exact name ---
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
      const r = await fetch(`/api/iac/stacks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          scope_kind: "host",
          scope_name: host.name,
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

  // PATCH the stack Auto DevOps OVERRIDE (decoupled from iac_enabled)
  async function setAutoDevOps(id: number, enabled: boolean) {
    const r = await fetch(`/api/iac/stacks/${id}`, {
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
    setAutoDevOps(s.iacId!, enabled).catch((err) => {
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
    const r = await fetch(`/api/iac/stacks/${s.iacId}`, { method: "DELETE", credentials: "include" });
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
      {streamLogs && (
        <LiveLogsModal host={host.name} container={streamLogs.ctr} onClose={() => setStreamLogs(null)} />
      )}

      {consoleTarget && (
        <ConsoleModal host={host.name} container={consoleTarget.ctr} onClose={() => setConsoleTarget(null)} />
      )}

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

      <div className="flex items-center gap-2">
        <div className="text-lg font-semibold text-white">
          {host.name} <span className="text-slate-400 text-sm">{host.address || ""}</span>
        </div>
        <div className="ml-auto flex items-center gap-2">
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

      {stacks.map((s, idx) => (
        <Card key={`${host.name}:${s.name}`} className="bg-slate-900/50 border-slate-800 rounded-xl">
          <CardHeader className="pb-2 flex flex-row items-center justify-between">
            <div className="space-y-1">
              <CardTitle className="text-xl text-white">
                <button className="hover:underline" onClick={() => onOpenStack(s.name, s.iacId)}>
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
                      const cfgShort = (r as any).configHash ? String((r as any).configHash).slice(0, 12) : "";

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
                              {cfgShort && (
                                <span
                                  className="text-[10px] px-1.5 py-0.5 rounded border border-slate-700 text-slate-400"
                                  title={(r as any).configHash}
                                >
                                  cfg {cfgShort}
                                </span>
                              )}
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
                              <ActionBtn title="Inspect" icon={Bug} onClick={() => onOpenStack(s.name, s.iacId)} />
                              <ActionBtn
                                title="Stats"
                                icon={Activity}
                                onClick={async () => {
                                  try {
                                    const r2 = await fetch(
                                      `/api/hosts/${encodeURIComponent(host.name)}/containers/${encodeURIComponent(r.name)}/stats`,
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
