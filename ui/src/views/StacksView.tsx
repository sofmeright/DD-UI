// ui/src/views/StacksView.tsx
import { useEffect, useMemo, useState } from "react";
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
import { ApiContainer, Host, IacService, IacStack, MergedRow, MergedStack } from "@/types";
import { formatDT, formatPortsLines } from "@/utils/format";

export default function StacksView({
  hosts,
  onOpenStack,
}: {
  hosts: Host[];
  onOpenStack: (host: Host, stackName: string) => void;
}) {
  const [selectedHost, setSelectedHost] = useState<Host | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [stacks, setStacks] = useState<MergedStack[]>([]);
  const [hostQuery, setHostQuery] = useState("");

  // Live logs & console wiring
  const [streamLogs, setStreamLogs] = useState<{ ctr: string } | null>(null);
  const [consoleTarget, setConsoleTarget] = useState<{ ctr: string; cmd?: string } | null>(null);

  // Lightweight info modal (used for one-shot text like Stats)
  const [infoModal, setInfoModal] = useState<{ title: string; text: string } | null>(null);

  // Set default host to first available
  useEffect(() => {
    if (hosts.length > 0 && !selectedHost) {
      setSelectedHost(hosts[0]);
    }
  }, [hosts, selectedHost]);

  function matchRow(r: MergedRow, q: string) {
    if (!q) return true;
    const hay = [r.name, r.state, r.stack, r.imageRun, r.imageIac, r.ip, r.portsText, r.owner]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return hay.includes(q.toLowerCase());
  }

  async function doCtrAction(ctr: string, action: string) {
    if (!selectedHost) return;
    try {
      await fetch(
        `/api/containers/hosts/${encodeURIComponent(selectedHost.name)}/${encodeURIComponent(ctr)}/action`,
        {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action }),
        }
      );
      loadStacksForHost(selectedHost.name);
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

  // Open live logs (EventSource)
  function openLiveLogs(ctr: string) {
    setStreamLogs({ ctr });
  }

  const loadStacksForHost = async (hostName: string) => {
    setLoading(true);
    setErr(null);
    try {
      const [rc, ri] = await Promise.all([
        fetch(`/api/containers/hosts/${encodeURIComponent(hostName)}`, { credentials: "include" }),
        fetch(`/api/iac/hosts/${encodeURIComponent(hostName)}`, { credentials: "include" }),
      ]);
      if (rc.status === 401 || ri.status === 401) {
        window.location.replace("/auth/login");
        return;
      }
      const contJson = await rc.json();
      const iacJson = await ri.json();
      const runtime: ApiContainer[] = (contJson.containers || []) as ApiContainer[];
      const iacStacks: IacStack[] = (iacJson.stacks || []) as IacStack[];

      // Fetch per-stack effective Auto DevOps using hierarchical endpoints
      const effMap: Record<string, boolean> = {};
      await Promise.all(
        (iacStacks || [])
          .map(async (s) => {
            try {
              const r = await fetch(`/api/iac/hosts/${encodeURIComponent(hostName)}/stacks/${encodeURIComponent(s.name)}`, { credentials: "include" });
              if (!r.ok) return;
              const j = await r.json();
              if (j?.stack?.name) {
                effMap[j.stack.name] = !!j.stack.effective_auto_devops;
              }
            } catch {
              /* ignore per-stack errors */
            }
          })
      );

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

        function desiredImageFor(c: ApiContainer): string | undefined {
          if (!is || services.length === 0) return undefined;
          const svc = services.find(
            (x) =>
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
            const exists = rows.some((r) => r.name === (svc.container_name || svc.service_name));
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
          stackDrift = rows.some((r) => r.drift) ? "drift" : "in_sync";
        }

        const effectiveAuto = is && effMap[is.name] ? true : false;

        merged.push({
          name: sname,
          drift: stackDrift,
          iacEnabled: effectiveAuto, // <- switch reflects EFFECTIVE auto-devops
          pullPolicy: hasIac ? is?.pull_policy : undefined,
          sops: hasIac ? is?.sops_status === "all" : false,
          deployKind: hasIac ? is?.deploy_kind || "compose" : sname === "(none)" ? "unmanaged" : "unmanaged",
          rows,
          hasIac,
          hasContent,
        });
      }

      setStacks(merged);
    } catch (e: any) {
      setErr(e?.message || "Failed to load host stacks");
    } finally {
      setLoading(false);
    }
  };

  // Load stacks when host changes
  useEffect(() => {
    if (!selectedHost) return;
    loadStacksForHost(selectedHost.name);
  }, [selectedHost]);

  async function createStackFlow() {
    if (!selectedHost) return;
    const existing = new Set(stacks.map((s) => s.name));
    let name = prompt("New stack name:");
    if (!name) return;
    name = name.trim();
    if (!name) return;
    if (existing.has(name)) {
      alert("A stack with that name already exists.");
      return;
    }
    try {
      const r = await fetch(`/api/iac/hosts/${encodeURIComponent(selectedHost.name)}/stacks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          stack_name: name,
          iac_enabled: false,
        }),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
      onOpenStack(selectedHost, name);
    } catch (e: any) {
      alert(e?.message || "Failed to create stack");
    }
  }

  // PATCH the stack Auto DevOps OVERRIDE (decoupled from iac_enabled) using hierarchical endpoint
  async function setAutoDevOps(hostName: string, stackName: string, enabled: boolean) {
    const r = await fetch(`/api/iac/hosts/${encodeURIComponent(hostName)}/stacks/${encodeURIComponent(stackName)}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_devops: enabled }),
    });
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  }

  function handleToggleAuto(sIndex: number, enabled: boolean) {
    const s = stacks[sIndex];
    if (!selectedHost || !s.hasIac || !s.hasContent) {
      if (enabled) {
        alert(
          "This stack needs compose files or services before Auto DevOps can be enabled. Add content first."
        );
      }
      return;
    }
    setStacks((prev) => prev.map((row, i) => (i === sIndex ? { ...row, iacEnabled: enabled } : row)));
    setAutoDevOps(selectedHost.name, s.name, enabled).catch((err) => {
      alert(`Failed to update Auto DevOps: ${err?.message || err}`);
      setStacks((prev) =>
        prev.map((row, i) => (i === sIndex ? { ...row, iacEnabled: !enabled } : row))
      );
    });
  }

  async function deleteStackAt(index: number) {
    const s = stacks[index];
    if (!selectedHost || !s.hasIac) return;
    if (
      !confirm(
        `Delete IaC for stack "${s.name}"? This removes IaC files/metadata but not runtime containers.`
      )
    )
      return;
    const r = await fetch(`/api/iac/hosts/${encodeURIComponent(selectedHost.name)}/stacks/${encodeURIComponent(s.name)}`, { method: "DELETE", credentials: "include" });
    if (!r.ok) {
      alert(`Failed to delete: ${r.status} ${r.statusText}`);
      return;
    }
    setStacks((prev) =>
      prev.map((row, i) =>
        i === index
          ? {
              ...row,
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

  // Close infoModal with Escape for accessibility
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
      {streamLogs && selectedHost && (
        <LiveLogsModal
          host={selectedHost.name}
          container={streamLogs.ctr}
          onClose={() => setStreamLogs(null)}
        />
      )}

      {/* Console Modal */}
      {consoleTarget && selectedHost && (
        <ConsoleModal
          host={selectedHost.name}
          container={consoleTarget.ctr}
          onClose={() => setConsoleTarget(null)}
        />
      )}

      {/* Lightweight Info Modal (used for Stats or simple text output) */}
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
              <Button
                size="sm"
                variant="outline"
                className="border-slate-700"
                onClick={() => setInfoModal(null)}
              >
                Close
              </Button>
            </div>
            <pre className="text-xs text-slate-300 bg-slate-900 border border-slate-800 rounded p-3 max-h-[60vh] overflow-auto whitespace-pre-wrap">
              {infoModal.text}
            </pre>
          </div>
        </div>
      )}

      {/* Host Selection and Controls */}
      <div className="flex items-center gap-2">
        <div className="text-lg font-semibold text-white">
          Stacks on:
        </div>
        <select
          value={selectedHost?.name || ""}
          onChange={(e) => {
            const host = hosts.find(h => h.name === e.target.value);
            if (host) setSelectedHost(host);
          }}
          className="px-3 py-2 bg-slate-900/50 border border-slate-800 text-slate-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-[#310937]"
        >
          <option value="">Select a host</option>
          {hosts.map((host) => (
            <option key={host.name} value={host.name}>
              {host.name} {host.address && `(${host.address})`}
            </option>
          ))}
        </select>
        
        {selectedHost && (
          <div className="ml-auto flex items-center gap-2">
            <Button onClick={() => loadStacksForHost(selectedHost.name)} className="bg-[#310937] hover:bg-[#2a0830] text-white">
              <RefreshCw className="h-4 w-4 mr-1" /> Sync
            </Button>
            <Button onClick={createStackFlow} variant="outline" className="border-slate-700 text-slate-200">
              <Eye className="hidden" /> {/* placeholder to avoid import shake issues */}
              New Stack
            </Button>
            <div className="relative w-72">
              <Input
                value={hostQuery}
                onChange={(e) => setHostQuery(e.target.value)}
                placeholder={`Search ${selectedHost.name}…`}
                className="pl-3 bg-slate-900/50 border-slate-800 text-slate-200 placeholder:text-slate-500"
              />
            </div>
          </div>
        )}
      </div>

      {!selectedHost ? (
        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-8 text-center">
            <p className="text-slate-400">Select a host to view stacks</p>
          </CardContent>
        </Card>
      ) : (
        <>
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
            <Card key={`${selectedHost.name}:${s.name}`} className="bg-slate-900/50 border-slate-800 rounded-xl">
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <div className="space-y-1">
                  <CardTitle className="text-xl text-white">
                    <button className="hover:underline" onClick={() => onOpenStack(selectedHost, s.name)}>
                      {s.name}
                    </button>
                  </CardTitle>
                  <div className="flex items-center gap-2">
                    {DriftBadge(s.drift)}
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
                        <Badge variant="outline" className="border-slate-700 text-slate-300">
                          no SOPS
                        </Badge>
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
                    disabled={!s.hasIac || !s.hasContent}
                  />
                  {s.hasIac && (
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
                            st.includes("running") ||
                            st.includes("up") ||
                            st.includes("healthy") ||
                            st.includes("restarting");
                          const isPaused = st.includes("paused");
                          return (
                            <tr
                              key={r.name}
                              className="border-t border-slate-800 hover:bg-slate-900/40 align-top"
                            >
                              <td className="px-2 py-1.5 font-medium text-slate-200 truncate">{r.name}</td>
                              <td className="px-2 py-1.5 text-slate-300">
                                <StatePill state={r.state} />
                              </td>
                              <td className="px-2 py-1.5 text-slate-300">
                                <div className="flex items-center gap-2">
                                  <div className="max-w-[24rem] truncate" title={r.imageRun || ""}>
                                    {r.imageRun || "—"}
                                  </div>
                                  {r.imageIac && (
                                    <>
                                      <ChevronRight className="h-4 w-4 text-slate-500" />
                                      <div
                                        className={`max-w-[24rem] truncate ${
                                          r.drift ? "text-amber-300" : "text-slate-300"
                                        }`}
                                        title={r.imageIac}
                                      >
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
                                  {!isRunning && !isPaused && (
                                    <ActionBtn
                                      title="Start"
                                      icon={Play}
                                      onClick={() => doCtrAction(r.name, "start")}
                                    />
                                  )}
                                  {isRunning && (
                                    <ActionBtn
                                      title="Stop"
                                      icon={Pause}
                                      onClick={() => doCtrAction(r.name, "stop")}
                                    />
                                  )}
                                  {(isRunning || isPaused) && (
                                    <ActionBtn
                                      title="Restart"
                                      icon={RotateCw}
                                      onClick={() => doCtrAction(r.name, "restart")}
                                    />
                                  )}
                                  {isRunning && !isPaused && (
                                    <ActionBtn
                                      title="Pause"
                                      icon={Pause}
                                      onClick={() => doCtrAction(r.name, "pause")}
                                    />
                                  )}
                                  {isPaused && (
                                    <ActionBtn
                                      title="Resume"
                                      icon={PlayCircle}
                                      onClick={() => doCtrAction(r.name, "unpause")}
                                    />
                                  )}

                                  <span className="mx-1 h-4 w-px bg-slate-700/60" />

                                  <ActionBtn
                                    title="Live logs"
                                    icon={FileText}
                                    onClick={() => openLiveLogs(r.name)}
                                  />
                                  <ActionBtn
                                    title="Inspect"
                                    icon={Bug}
                                    onClick={() => onOpenStack(selectedHost, s.name)}
                                  />
                                  <ActionBtn
                                    title="Stats"
                                    icon={Activity}
                                    onClick={async () => {
                                      try {
                                        const r2 = await fetch(
                                          `/api/containers/hosts/${encodeURIComponent(selectedHost.name)}/${encodeURIComponent(r.name)}/stats`,
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

                                  <ActionBtn
                                    title="Kill"
                                    icon={ZapOff}
                                    onClick={() => doCtrAction(r.name, "kill")}
                                  />
                                  <ActionBtn
                                    title="Remove"
                                    icon={Trash2}
                                    onClick={() => doCtrAction(r.name, "remove")}
                                  />

                                  <ActionBtn
                                    title="Console"
                                    icon={Terminal}
                                    onClick={() => setConsoleTarget({ ctr: r.name /* shell picker in modal */ })}
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
                <div className="pt-2 text-xs text-slate-400">
                  Tip: click the stack title to open the full compare & editor view.
                </div>
              </CardContent>
            </Card>
          ))}

          <Card className="bg-slate-900/40 border-slate-800">
            <CardContent className="py-4 flex flex-wrap items-center gap-3 text-sm text-slate-300">
              Security by default:
              <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">
                AGE key never persisted
              </span>
              <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">
                Decrypt to tmpfs only
              </span>
              <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
              <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}