// ui/src/App.tsx
import React, { useEffect, useMemo, useState } from "react";
import { Routes, Route, useNavigate, useLocation, useParams } from "react-router-dom";
import LeftNav from "@/components/LeftNav";
import LoginGate from "@/components/LoginGate";
import HostsView from "@/views/HostsView";
import HostStacksView from "@/views/HostStacksView";
import StackDetailView from "@/views/StackDetailView";
import ImagesView from "@/views/ImagesView";
import NetworksView from "@/views/NetworksView";
import VolumesView from "@/views/VolumesView";
import { SessionResp, Host, ApiContainer, IacStack } from "@/types";
import { computeHostMetrics } from "@/utils/metrics";

export default function App() {
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [metricsCache, setMetricsCache] = useState<Record<string, { stacks: number; containers: number; drift: number; errors: number }>>({});
  const [sessionChecked, setSessionChecked] = useState(false);
  const [authed, setAuthed] = useState<boolean>(false);
  const [filterQuery, setFilterQuery] = useState("");
  
  const navigate = useNavigate();
  const location = useLocation();

  // Helper: current host from URL (fallback to first host)
  const currentHostFromPath = () => {
    const m = location.pathname.match(/^\/hosts\/([^/]+)/);
    return (m && decodeURIComponent(m[1])) || hosts[0]?.name || "";
  };
  
  // Determine current page from URL (for LeftNav highlighting)
  const getCurrentPage = () => {
    const path = location.pathname;
    if (path === "/hosts") return "hosts";
    if (path.startsWith("/hosts/")) {
      const seg = path.split("/")[3]; // ['', 'hosts', '<host>', '<section>', ...]
      if (seg === "images") return "images";
      if (seg === "networks") return "networks";
      if (seg === "volumes") return "volumes";
      // default for /hosts/:hostName or /hosts/:hostName/stacks(/...)
      return "stacks";
    }
    return "hosts";
  };

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
        const mapped: Host[] = items.map((h: any) => ({ name: h.name, address: h.addr ?? h.address ?? "", groups: h.groups ?? [] }));
        setHosts(mapped);
      } catch (e: any) {
        setErr(e?.message || "Failed to load hosts");
      } finally {
        setLoading(false);
      }
    })();
  }, [authed]);

  const filteredHosts = useMemo(() => {
    const q = filterQuery.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter(h => [h.name, h.address || "", ...(h.groups || [])].join(" ").toLowerCase().includes(q));
  }, [hosts, filterQuery]);

  const hostKey = useMemo(() => hosts.map(h => h.name).sort().join("|"), [hosts]);
  useEffect(() => { setMetricsCache({}); }, [hostKey]);

  async function refreshMetricsForHosts(hostNames: string[]) {
    if (!hostNames.length) return;
    const limit = 4; let idx = 0;
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
      stacks += m.stacks; containers += m.containers; drift += m.drift; errors += m.errors;
    }
    return { hosts: filteredHosts.length, stacks, containers, drift, errors };
  }, [filteredHosts, metricsCache]);

  async function handleScanAll() {
    if (scanning) return; setScanning(true);
    try {
      await fetch("/api/iac/scan", { method: "POST", credentials: "include" }).catch(()=>{});
      await fetch("/api/scan/all", { method: "POST", credentials: "include" });
      await refreshMetricsForHosts(hosts.map(h => h.name));
    } finally { setScanning(false); }
  }

  if (sessionChecked && !authed) return <LoginGate />;
  if (!sessionChecked) return <div className="min-h-screen bg-slate-950" />;

  // Route components
  const HostStacksPage = () => {
    const { hostName } = useParams<{ hostName: string }>();
    const host = hosts.find(h => h.name === hostName);
    if (!host) {
      navigate('/hosts');
      return null;
    }
    return (
      <HostStacksView
        host={host}
        onSync={() => refreshMetricsForHosts([host.name])}
        onOpenStack={(stackName, iacId) =>
          navigate(`/hosts/${encodeURIComponent(hostName!)}/stacks/${encodeURIComponent(stackName)}${iacId ? `?iacId=${iacId}` : ""}`)
        }
      />
    );
  };

  const StackDetailPage = () => {
    const { hostName, stackName } = useParams<{ hostName: string; stackName: string }>();
    const searchParams = new URLSearchParams(location.search);
    const iacId = searchParams.get('iacId') ? parseInt(searchParams.get('iacId')!) : undefined;
    
    const host = hosts.find(h => h.name === hostName);
    if (!host || !stackName) {
      navigate('/hosts');
      return null;
    }
    
    return (
      <StackDetailView
        host={host}
        stackName={stackName}
        iacId={iacId}
        onBack={() => navigate(`/hosts/${encodeURIComponent(host.name)}/stacks`)}
      />
    );
  };

  return (
    <div className="min-h-screen bg-slate-950 flex">
      <LeftNav
        page={getCurrentPage()}
        onGoHosts={() => navigate('/hosts')}
        onGoStacks={() => {
          const h = currentHostFromPath();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/stacks`);
          else navigate('/hosts');
        }}
        onGoImages={() => {
          const h = currentHostFromPath();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/images`);
          else navigate('/hosts');
        }}
        onGoNetworks={() => {
          const h = currentHostFromPath();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/networks`);
          else navigate('/hosts');
        }}
        onGoVolumes={() => {
          const h = currentHostFromPath();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/volumes`);
          else navigate('/hosts');
        }}
      />

      {/* Right side layout retained */}
      <div className="flex-1 min-w-0 max-h-screen flex flex-col">
        <main className="flex-1 overflow-y-auto overflow-x-hidden p-3 sm:p-4 lg:p-6">
          <Routes>
            <Route path="/hosts" element={
              <HostsView
                metrics={metrics}
                hosts={hosts}
                filteredHosts={filteredHosts}
                loading={loading}
                err={err}
                scanning={scanning}
                onScanAll={handleScanAll}
                onFilter={setFilterQuery}
                onOpenHost={(hostName) => navigate(`/hosts/${encodeURIComponent(hostName)}/stacks`)}
                refreshMetricsForHosts={() => refreshMetricsForHosts(hosts.map(h => h.name))}
              />
            } />
            <Route path="/hosts/:hostName/stacks" element={<HostStacksPage />} />
            <Route path="/hosts/:hostName/stacks/:stackName" element={<StackDetailPage />} />
            <Route path="/hosts/:hostName/images" element={<ImagesView hosts={hosts} />} />
            <Route path="/hosts/:hostName/networks" element={<NetworksView hosts={hosts} />} />
            <Route path="/hosts/:hostName/volumes" element={<VolumesView hosts={hosts} />} />
          </Routes>
        </main>

        <footer className="shrink-0 border-t border-slate-800 bg-slate-950/80">
          <div className="px-3 sm:px-4 lg:px-6 py-2 text-[10px] leading-none text-slate-500 text-center">
            © {new Date().getFullYear()} PrecisionPlanIT &amp; SoFMeRight (Kai)
          </div>
        </footer>
      </div>
    </div>
  );
}
