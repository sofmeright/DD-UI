// ui/src/views/StacksView.tsx
import { useEffect, useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
// Using native select for now
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
  Download,
  Upload,
  Users,
  Layers,
  Server,
  AlertTriangle,
  CheckCircle,
  XCircle,
} from "lucide-react";
import DriftBadge from "@/components/DriftBadge";
import ConsoleModal from "@/components/ConsoleModal";
import LiveLogsModal from "@/components/LiveLogsModal";
import { ApiContainer, Host, IacService, IacStack, MergedRow, MergedStack } from "@/types";

export default function StacksView({
  hosts,
  onOpenStack,
}: {
  hosts: Host[];
  onOpenStack: (host: Host, stackName: string, iacId?: number) => void;
}) {
  const [selectedHost, setSelectedHost] = useState<Host | null>(null);
  const [stacks, setStacks] = useState<MergedStack[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [expandedStacks, setExpandedStacks] = useState<Set<string>>(new Set());
  const [consoleModal, setConsoleModal] = useState<{ host: string; container: string } | null>(null);
  const [logsModal, setLogsModal] = useState<{ host: string; container: string } | null>(null);
  const [filterQuery, setFilterQuery] = useState("");

  // Set default host to first available
  useEffect(() => {
    if (hosts.length > 0 && !selectedHost) {
      setSelectedHost(hosts[0]);
    }
  }, [hosts, selectedHost]);

  // Load stacks when host changes
  useEffect(() => {
    if (!selectedHost) return;
    loadStacksForHost(selectedHost.name);
  }, [selectedHost]);

  const loadStacksForHost = async (hostName: string) => {
    setLoading(true);
    setErr(null);
    try {
      // Load containers and IAC data
      const [containersResp, iacResp] = await Promise.all([
        fetch(`/api/hosts/${hostName}/containers`, { credentials: "include" }),
        fetch(`/api/hosts/${hostName}/iac`, { credentials: "include" }),
      ]);

      if (!containersResp.ok) throw new Error(`Containers: HTTP ${containersResp.status}`);
      if (!iacResp.ok) throw new Error(`IAC: HTTP ${iacResp.status}`);

      const containersData = await containersResp.json();
      const iacData = await iacResp.json();

      const containers: ApiContainer[] = containersData.items || [];
      const iacStacks: IacStack[] = iacData.stacks || [];

      // Create merged stacks (same logic as HostStacksView)
      const merged: MergedStack[] = [];
      const stackNames = new Set<string>();

      // Collect all stack names
      containers.forEach(c => c.stack && stackNames.add(c.stack));
      iacStacks.forEach(s => stackNames.add(s.name));

      for (const sname of Array.from(stackNames).sort()) {
        const rcs = containers.filter(c => c.stack === sname);
        const is = iacStacks.find(s => s.name === sname);
        const services = is?.services || [];
        const hasIac = !!is;

        const rows: MergedRow[] = [];

        // Helper function to get desired image
        const desiredImageFor = (c: ApiContainer) => {
          const svc = services.find(
            s => (s.container_name || s.service_name) === c.name ||
                 s.service_name === c.compose_service
          );
          return svc?.image || undefined;
        };

        // Add running containers
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

        // Add missing containers from IAC
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

        // Determine drift status
        let stackDrift: "in_sync" | "drift" | "unknown" = "unknown";
        if (hasIac) {
          stackDrift = rows.some((r) => r.drift) ? "drift" : "in_sync";
        }

        merged.push({
          name: sname,
          drift: stackDrift,
          iacEnabled: is?.iac_enabled || false,
          pullPolicy: is?.pull_policy,
          sops: is?.sops_status === "all",
          deployKind: is?.deploy_kind || "unmanaged",
          rows,
          iacId: is?.id,
          hasIac,
          hasContent: hasIac,
        });
      }

      setStacks(merged);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const filteredStacks = useMemo(() => {
    if (!filterQuery.trim()) return stacks;
    const q = filterQuery.toLowerCase();
    return stacks.filter(s => 
      s.name.toLowerCase().includes(q) ||
      s.rows.some(r => r.name.toLowerCase().includes(q))
    );
  }, [stacks, filterQuery]);

  const toggleStack = (stackName: string) => {
    const newExpanded = new Set(expandedStacks);
    if (newExpanded.has(stackName)) {
      newExpanded.delete(stackName);
    } else {
      newExpanded.add(stackName);
    }
    setExpandedStacks(newExpanded);
  };

  // Helper functions (same as HostStacksView)
  const formatDT = (ts?: string) => {
    if (!ts) return "—";
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  };

  const formatPortsLines = (ports: any) => {
    if (!Array.isArray(ports)) return [];
    return ports.map((p: any) => {
      if (p.PublicPort && p.PrivatePort) {
        return `${p.PublicPort}:${p.PrivatePort}/${p.Type || "tcp"}`;
      }
      if (p.PrivatePort) {
        return `${p.PrivatePort}/${p.Type || "tcp"}`;
      }
      return JSON.stringify(p);
    });
  };

  const performAction = async (action: string, containerName: string) => {
    if (!selectedHost) return;
    try {
      const response = await fetch(`/api/hosts/${selectedHost.name}/containers/${containerName}/action`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ action }),
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      await loadStacksForHost(selectedHost.name);
    } catch (e) {
      console.error(`Action ${action} failed:`, e);
    }
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex-1 overflow-auto p-6">
        <div className="max-w-7xl mx-auto space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-slate-200">Stacks</h1>
              <p className="text-slate-400 mt-1">Manage application stacks across hosts</p>
            </div>
            
            {/* Host Selection Dropdown */}
            <div className="flex items-center gap-4">
              <div className="text-sm text-slate-400">Host:</div>
              <select
                value={selectedHost?.name || ""}
                onChange={(e) => {
                  const host = hosts.find(h => h.name === e.target.value);
                  if (host) setSelectedHost(host);
                }}
                className="w-64 px-3 py-2 bg-slate-900 border border-slate-700 text-slate-200 rounded-md focus:outline-none focus:ring-2 focus:ring-brand"
              >
                <option value="">Select a host</option>
                {hosts.map((host) => (
                  <option key={host.name} value={host.name}>
                    {host.name} {host.address && `(${host.address})`}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {/* Search */}
          <div className="flex gap-4 items-center">
            <div className="flex-1 max-w-md">
              <Input
                placeholder="Filter stacks..."
                value={filterQuery}
                onChange={(e) => setFilterQuery(e.target.value)}
                className="bg-slate-900 border-slate-700 text-slate-200"
              />
            </div>
            <Button
              onClick={() => selectedHost && loadStacksForHost(selectedHost.name)}
              variant="outline"
              size="sm"
              className="border-slate-700 text-slate-300 hover:bg-slate-800"
              disabled={!selectedHost}
            >
              <RefreshCw className="h-4 w-4 mr-2" />
              Refresh
            </Button>
          </div>

          {/* Error Display */}
          {err && (
            <Card className="border-red-500/50 bg-red-500/10">
              <CardContent className="p-4">
                <p className="text-red-400">Error: {err}</p>
              </CardContent>
            </Card>
          )}

          {/* Loading */}
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <RefreshCw className="h-8 w-8 animate-spin text-slate-400" />
              <span className="ml-2 text-slate-400">Loading stacks...</span>
            </div>
          ) : (
            /* Stacks List */
            <div className="space-y-4">
              {filteredStacks.map((stack) => (
                <Card key={stack.name} className="border-slate-800 bg-slate-900/50">
                  <CardHeader className="pb-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => toggleStack(stack.name)}
                          className="p-1 h-8 w-8"
                        >
                          <ChevronRight
                            className={`h-4 w-4 transition-transform ${
                              expandedStacks.has(stack.name) ? "rotate-90" : ""
                            }`}
                          />
                        </Button>
                        <div>
                          <CardTitle className="text-lg text-slate-200">{stack.name}</CardTitle>
                          <div className="flex items-center gap-2 mt-1">
                            <Badge variant="outline" className="text-xs">
                              {stack.deployKind}
                            </Badge>
                            {DriftBadge(stack.drift)}
                            {stack.sops && (
                              <Badge className="bg-purple-900/40 border-purple-700/40 text-purple-200 text-xs">
                                SOPS
                              </Badge>
                            )}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-slate-400">
                          {stack.rows.length} container{stack.rows.length !== 1 ? 's' : ''}
                        </span>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => selectedHost && onOpenStack(selectedHost, stack.name, stack.iacId)}
                          className="text-slate-300 hover:text-slate-100"
                        >
                          <FileText className="h-4 w-4 mr-1" />
                          Details
                        </Button>
                      </div>
                    </div>
                  </CardHeader>

                  {expandedStacks.has(stack.name) && (
                    <CardContent className="pt-0">
                      <div className="space-y-2">
                        {stack.rows.map((row) => (
                          <div
                            key={row.name}
                            className="flex items-center justify-between p-3 rounded border border-slate-700 bg-slate-800/50"
                          >
                            <div className="flex items-center gap-3">
                              <div className={`w-2 h-2 rounded-full ${
                                row.state === "running" ? "bg-green-500" :
                                row.state === "exited" ? "bg-red-500" :
                                row.state === "missing" ? "bg-yellow-500" :
                                "bg-slate-500"
                              }`} />
                              <div>
                                <div className="font-medium text-slate-200">{row.name}</div>
                                <div className="text-sm text-slate-400">
                                  {row.imageRun && `Running: ${row.imageRun}`}
                                  {row.imageIac && row.imageRun && " | "}
                                  {row.imageIac && `Expected: ${row.imageIac}`}
                                </div>
                              </div>
                            </div>
                            <div className="flex items-center gap-2">
                              <Badge variant={row.state === "running" ? "default" : "outline"}>
                                {row.state}
                              </Badge>
                              {row.drift && (
                                <Badge variant="destructive" className="text-xs">Drift</Badge>
                              )}
                              {row.state === "running" && (
                                <div className="flex gap-1">
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => setLogsModal({ host: selectedHost!.name, container: row.name })}
                                    className="p-1 h-7 w-7"
                                  >
                                    <FileText className="h-3 w-3" />
                                  </Button>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => setConsoleModal({ host: selectedHost!.name, container: row.name })}
                                    className="p-1 h-7 w-7"
                                  >
                                    <Terminal className="h-3 w-3" />
                                  </Button>
                                </div>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    </CardContent>
                  )}
                </Card>
              ))}

              {!loading && filteredStacks.length === 0 && (
                <Card className="border-slate-800 bg-slate-900/50">
                  <CardContent className="p-8 text-center">
                    <p className="text-slate-400">
                      {selectedHost ? "No stacks found on this host" : "Select a host to view stacks"}
                    </p>
                  </CardContent>
                </Card>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Modals */}
      {consoleModal && (
        <ConsoleModal
          host={consoleModal.host}
          container={consoleModal.container}
          onClose={() => setConsoleModal(null)}
        />
      )}
      {logsModal && (
        <LiveLogsModal
          host={logsModal.host}
          container={logsModal.container}
          onClose={() => setLogsModal(null)}
        />
      )}
    </div>
  );
}