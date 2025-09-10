// ui/src/AppRouter.tsx
import React, { useEffect, useMemo, useState } from "react";
import { Routes, Route, useNavigate, useLocation, useParams } from "react-router-dom";
import LeftNav from "@/components/LeftNav";
import LoginGate from "@/components/LoginGate";
import DeploymentsView from "@/views/DeploymentsView";
import HostStacksView from "@/views/HostStacksView";
import StackDetailView from "@/views/StackDetailView";
import ImagesView from "@/views/ImagesView";
import NetworksView from "@/views/NetworksView";
import VolumesView from "@/views/VolumesView";
import { SessionResp, Host, ApiContainer, IacStack } from "@/types";
import { computeHostMetrics } from "@/utils/metrics";

// Layout component that wraps all routes
function AppLayout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  const location = useLocation();
  
  // Determine current page from URL
  const getCurrentPage = () => {
    const path = location.pathname;
    if (path.startsWith('/deployments')) return 'deployments';
    if (path.startsWith('/hosts')) return 'host';
    if (path.startsWith('/stacks')) return 'stack';
    if (path.startsWith('/images')) return 'images';
    if (path.startsWith('/networks')) return 'networks';
    if (path.startsWith('/volumes')) return 'volumes';
    return 'deployments';
  };

  return (
    <div className="h-screen bg-slate-950 flex">
      <LeftNav
        page={getCurrentPage()}
        onGoDeployments={() => navigate('/deployments')}
        onGoImages={() => navigate('/images')}
        onGoNetworks={() => navigate('/networks')}
        onGoVolumes={() => navigate('/volumes')}
      />
      <div className="flex-1 flex flex-col overflow-hidden">
        {children}
      </div>
    </div>
  );
}

// Host view component with router params
function HostView() {
  const { hostName } = useParams<{ hostName: string }>();
  const navigate = useNavigate();
  
  if (!hostName) {
    navigate('/deployments');
    return null;
  }

  return (
    <HostStacksView
      host={{ name: hostName }}
      onBack={() => navigate('/deployments')}
      onSync={() => window.location.reload()}
      onOpenStack={(stackName, iacId) => navigate(`/hosts/${hostName}/stacks/${stackName}${iacId ? `?iacId=${iacId}` : ''}`)}
    />
  );
}

// Stack detail view component with router params
function StackView() {
  const { hostName, stackName } = useParams<{ hostName: string; stackName: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  
  if (!hostName || !stackName) {
    navigate('/deployments');
    return null;
  }

  // Extract iacId from query params
  const searchParams = new URLSearchParams(location.search);
  const iacId = searchParams.get('iacId') ? parseInt(searchParams.get('iacId')!) : undefined;

  return (
    <StackDetailView
      host={{ name: hostName }}
      stackName={stackName}
      iacId={iacId}
      onBack={() => navigate(`/hosts/${hostName}`)}
    />
  );
}

export default function AppRouter() {
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [metricsCache, setMetricsCache] = useState<Record<string, { stacks: number; containers: number; drift: number; errors: number }>>({});
  const [sessionChecked, setSessionChecked] = useState(false);
  const [authed, setAuthed] = useState<boolean>(false);
  const [filterQuery, setFilterQuery] = useState("");

  const navigate = useNavigate();

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
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = await r.json();
        if (!cancel) {
          setHosts(data.items || []);
          
          // Compute metrics for all hosts
          const newCache: Record<string, { stacks: number; containers: number; drift: number; errors: number }> = {};
          for (const host of data.items || []) {
            try {
              const metrics = await computeHostMetrics(host.name);
              newCache[host.name] = metrics;
            } catch (e) {
              newCache[host.name] = { stacks: 0, containers: 0, drift: 0, errors: 1 };
            }
          }
          if (!cancel) setMetricsCache(newCache);
        }
      } catch (e) {
        if (!cancel) setErr(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
  }, [authed]);

  const scanAll = async () => {
    setScanning(true);
    try {
      const r = await fetch("/api/scan/all", { method: "POST", credentials: "include" });
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      // Refresh after scan
      window.location.reload();
    } catch (e) {
      console.error("Scan failed:", e);
    } finally {
      setScanning(false);
    }
  };

  const filteredHosts = useMemo(() => {
    if (!filterQuery.trim()) return hosts;
    const q = filterQuery.toLowerCase();
    return hosts.filter(h => 
      h.name.toLowerCase().includes(q) || 
      (h.address && h.address.toLowerCase().includes(q))
    );
  }, [hosts, filterQuery]);

  const globalMetrics = useMemo(() => {
    let stacks = 0, containers = 0, drift = 0, errors = 0;
    for (const host of filteredHosts) {
      const m = metricsCache[host.name];
      if (!m) continue;
      stacks += m.stacks; containers += m.containers; drift += m.drift; errors += m.errors;
    }
    return { hosts: filteredHosts.length, stacks, containers, drift, errors };
  }, [filteredHosts, metricsCache]);

  if (!sessionChecked) {
    return <div className="h-screen bg-slate-950 flex items-center justify-center">
      <div className="text-slate-400">Checking session...</div>
    </div>;
  }

  if (!authed) {
    return <LoginGate />;
  }

  return (
    <AppLayout>
      <Routes>
        <Route path="/" element={
          <DeploymentsView
            metrics={globalMetrics}
            hosts={hosts}
            filteredHosts={filteredHosts}
            loading={loading}
            err={err}
            scanning={scanning}
            onScanAll={scanAll}
            onFilter={setFilterQuery}
            onOpenHost={(host) => navigate(`/hosts/${host.name}`)}
            refreshMetricsForHosts={() => window.location.reload()}
          />
        } />
        <Route path="/deployments" element={
          <DeploymentsView
            metrics={globalMetrics}
            hosts={hosts}
            filteredHosts={filteredHosts}
            loading={loading}
            err={err}
            scanning={scanning}
            onScanAll={scanAll}
            onFilter={setFilterQuery}
            onOpenHost={(host) => navigate(`/hosts/${host.name}`)}
            refreshMetricsForHosts={() => window.location.reload()}
          />
        } />
        <Route path="/hosts/:hostName" element={<HostView />} />
        <Route path="/hosts/:hostName/stacks/:stackName" element={<StackView />} />
        <Route path="/images" element={<ImagesView hosts={hosts} />} />
        <Route path="/networks" element={<NetworksView hosts={hosts} />} />
        <Route path="/volumes" element={<VolumesView hosts={hosts} />} />
      </Routes>
    </AppLayout>
  );
}