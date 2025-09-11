// ui/src/views/HostsView.tsx
import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import MetricCard from "@/components/MetricCard";
import { Boxes, Layers, AlertTriangle, XCircle, RefreshCw } from "lucide-react";
import { Host } from "@/types";

export default function HostsView({
  metrics, hosts, filteredHosts, loading, err, scanning, onScanAll, onFilter, onOpenHost, refreshMetricsForHosts,
}: {
  metrics: { hosts: number; stacks: number; containers: number; drift: number; errors: number };
  hosts: Host[];
  filteredHosts: Host[];
  loading: boolean;
  err: string | null;
  scanning: boolean;
  onScanAll: () => Promise<void> | void;
  onFilter: (v: string) => void;
  onOpenHost: (host: Host) => void;
  refreshMetricsForHosts: () => void;
}) {
  const [filterQuery, setFilterQuery] = useState("");

  const handleFilterChange = (value: string) => {
    setFilterQuery(value);
    onFilter(value);
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex-1 overflow-auto p-6">
        <div className="max-w-7xl mx-auto space-y-6">
          <div>
            <h1 className="text-3xl font-bold text-slate-200">Hosts</h1>
            <p className="text-slate-400 mt-1">Manage and monitor your infrastructure hosts</p>
          </div>

          {/* Metrics Cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
            <MetricCard title="Hosts" value={metrics.hosts} icon={Boxes} />
            <MetricCard title="Stacks" value={metrics.stacks} icon={Layers} />
            <MetricCard title="Containers" value={metrics.containers} icon={Boxes} />
            <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
            <MetricCard title="Errors" value={<span className="text-red-400">{metrics.errors}</span>} icon={XCircle} />
          </div>

          {/* Controls */}
          <div className="flex flex-col sm:flex-row gap-4 items-start sm:items-center justify-between">
            <div className="flex-1 max-w-md">
              <Input
                placeholder="Filter hosts..."
                value={filterQuery}
                onChange={(e) => handleFilterChange(e.target.value)}
                className="bg-slate-900 border-slate-700 text-slate-200"
              />
            </div>
            <div className="flex gap-2">
              <Button
                onClick={refreshMetricsForHosts}
                variant="outline"
                size="sm"
                className="border-slate-700 text-slate-300 hover:bg-slate-800"
              >
                <RefreshCw className="h-4 w-4 mr-2" />
                Refresh
              </Button>
              <Button
                onClick={onScanAll}
                disabled={scanning}
                size="sm"
                className="bg-brand hover:bg-brand/80"
              >
                {scanning ? <RefreshCw className="h-4 w-4 mr-2 animate-spin" /> : <RefreshCw className="h-4 w-4 mr-2" />}
                {scanning ? "Scanning..." : "Scan All"}
              </Button>
            </div>
          </div>

          {/* Error Display */}
          {err && (
            <Card className="border-red-500/50 bg-red-500/10">
              <CardContent className="p-4">
                <p className="text-red-400">Error: {err}</p>
              </CardContent>
            </Card>
          )}

          {/* Hosts Grid */}
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <RefreshCw className="h-8 w-8 animate-spin text-slate-400" />
              <span className="ml-2 text-slate-400">Loading hosts...</span>
            </div>
          ) : (
            <div className="grid gap-4">
              {filteredHosts.map((host) => (
                <Card
                  key={host.name}
                  className="border-slate-800 bg-slate-900/50 hover:bg-slate-800/50 transition-colors"
                >
                  <CardContent className="p-4">
                    <div className="flex items-center justify-between cursor-pointer" onClick={() => onOpenHost(host)}>
                      <div className="flex items-center gap-3">
                        <div className="w-3 h-3 bg-green-500 rounded-full"></div>
                        <div>
                          <h3 className="font-semibold text-slate-200">{host.name}</h3>
                          {host.address && (
                            <p className="text-sm text-slate-400">{host.address}</p>
                          )}
                        </div>
                      </div>
                      <div className="text-right text-sm text-slate-400">
                        Click to view stacks
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {!loading && filteredHosts.length === 0 && (
            <Card className="border-slate-800 bg-slate-900/50">
              <CardContent className="p-8 text-center">
                <p className="text-slate-400">No hosts found</p>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}