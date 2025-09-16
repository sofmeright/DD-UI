import { useEffect, useMemo, useState } from "react";
import MetricCard from "@/components/MetricCard";
import { Boxes, Layers, AlertTriangle, XCircle, Server } from "lucide-react";
import { Host, ApiContainer, IacStack } from "@/types";
import { computeHostMetrics } from "@/utils/metrics";

export default function DashboardView({ hosts }: { hosts: Host[] }) {
  const [metricsCache, setMetricsCache] = useState<Record<string, { stacks: number; containers: number; drift: number; errors: number }>>({});
  
  useEffect(() => {
    if (!hosts.length) return;
    
    async function fetchMetrics() {
      const limit = 4; 
      let idx = 0;
      const workers = Array.from({ length: Math.min(limit, hosts.length) }, () => (async () => {
        while (true) {
          const i = idx++; 
          if (i >= hosts.length) break;
          const name = hosts[i].name;
          try {
            const [rc, ri] = await Promise.all([
              fetch(`/api/containers/hosts/${encodeURIComponent(name)}`, { credentials: "include" }),
              fetch(`/api/iac/hosts/${encodeURIComponent(name)}`, { credentials: "include" }),
            ]);
            if (rc.status === 401 || ri.status === 401) { 
              window.location.replace("/auth/login"); 
              return; 
            }
            const contJson = await rc.json();
            const iacJson = await ri.json();
            const runtime: ApiContainer[] = (contJson.containers || []) as ApiContainer[];
            const iacStacks: IacStack[] = (iacJson.stacks || []) as IacStack[];
            
            // Count actual drift from backend's drift_detected field
            let driftCount = 0;
            if (Array.isArray(iacJson.stacks)) {
              driftCount = iacJson.stacks.filter((s: any) => s.drift_detected === true).length;
            }
            
            // Use backend drift count instead of frontend calculation
            const m = computeHostMetrics(runtime, iacStacks);
            m.drift = driftCount; // Override with backend's drift detection
            setMetricsCache(prev => ({ ...prev, [name]: m }));
          } catch {
            // ignore per-host metrics errors
          }
        }
      })());
      await Promise.all(workers);
    }
    
    fetchMetrics();
  }, [hosts]);

  const metrics = useMemo(() => {
    let stacks = 0, containers = 0, drift = 0, errors = 0;
    for (const h of hosts) {
      const m = metricsCache[h.name];
      if (!m) continue;
      stacks += m.stacks; 
      containers += m.containers; 
      drift += m.drift; 
      errors += m.errors;
    }
    return { hosts: hosts.length, stacks, containers, drift, errors };
  }, [hosts, metricsCache]);

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-4">
        <div className="text-lg font-semibold text-white">Dashboard</div>
      </div>

      {/* Full-size metric cards */}
      <div className="grid md:grid-cols-5 gap-4">
        <MetricCard title="Hosts" value={metrics.hosts} icon={Server} />
        <MetricCard title="Stacks" value={metrics.stacks} icon={Boxes} />
        <MetricCard title="Containers" value={metrics.containers} icon={Layers} />
        <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
        <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
      </div>

      {/* Welcome message and project description */}
      <div className="flex items-center justify-center min-h-[300px]">
        <div className="text-center space-y-4 max-w-3xl">
          <div className="text-4xl">🚀</div>
          <div className="text-2xl font-semibold text-white">Welcome to DD-UI!</div>
          <div className="text-slate-300 leading-relaxed">
            DD-UI is a project that desires to bring the spirit of GitOps and projects like ArgoCD & FluxCD to Docker. 
          </div>
          <div className="text-slate-400 leading-relaxed">
            We feature an automated adoption process that helps any user who is comfortable with docker compose to implement 
            a basic but effective and minimally configurable CD pipeline based on a static path structure and ansible 
            inventory files featuring SOPS encryption for simple & easily manageable secrets contained in .env and even 
            compose can be encrypted and deploys decrypted at runtime.
          </div>
        </div>
      </div>
    </div>
  );
}