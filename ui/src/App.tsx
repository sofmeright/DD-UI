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
import DashboardView from "@/views/DashboardView";
import GroupsView from "@/views/GroupsView";
import CleanupView from "@/views/CleanupView";
import LoggingView from "@/views/LoggingView";
import GitSyncView from "@/views/GitSyncView";
import { SessionResp, Host, ApiContainer, IacStack } from "@/types";
import { computeHostMetrics } from "@/utils/metrics";
import { setAuthFailureCallback } from "@/utils/auth";

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
  
  // Register global auth failure handler
  useEffect(() => {
    setAuthFailureCallback(() => {
      setAuthed(false);
      setSessionChecked(true);
    });
  }, []);
  
  // Check authentication on every route change
  useEffect(() => {
    // Always check on route changes, even if not currently authed
    if (!sessionChecked) return;
    
    // Verify session is still valid on route changes
    fetch("/api/session", { credentials: "include" }) .then(r => {
        if (r.status === 401) {
          setAuthed(false);
          setSessionChecked(true);
          // Force immediate redirect
          window.location.href = '/auth/login';
        }
      }) .catch(() => {
        // Network error - assume not authenticated
        setAuthed(false);
        setSessionChecked(true);
        window.location.href = '/auth/login';
      });
  }, [location.pathname]); // Re-check on every route change
  
  // Determine current page from URL (for LeftNav highlight)
  const getCurrentPage = () => {
    const path = location.pathname;
    if (path === "/dashboard") return "dashboard";
    if (path === "/hosts") return "hosts";
    if (/^\/hosts\/[^/]+\/stacks/.test(path)) return "stacks";
    if (path === "/groups") return "groups";
    if (/^\/hosts\/[^/]+\/images/.test(path)) return "images";
    if (/^\/hosts\/[^/]+\/networks/.test(path)) return "networks";
    if (/^\/hosts\/[^/]+\/volumes/.test(path)) return "volumes";
    if (path === "/cleanup" || /^\/hosts\/[^/]+\/cleanup/.test(path)) return "cleanup";
    if (path === "/logging") return "logging";
    if (path === "/git") return "git";
    return "hosts";
  };

  // Current host inferred from URL, fallback to localStorage, then first host
  const currentHostFromPath = () => {
    const m = location.pathname.match(/^\/hosts\/([^/]+)/);
    return (m && decodeURIComponent(m[1])) || hosts[0]?.name || "";
  };

  // Get best host for navigation: URL -> localStorage -> first host
  const getBestHost = () => {
    const urlHost = currentHostFromPath();
    if (urlHost) return urlHost;
    
    const stored = localStorage.getItem('dd_ui_selected_host');
    if (stored && hosts.some(h => h.name === stored)) return stored;
    
    return hosts[0]?.name || "";
  };

  useEffect(() => {
    let cancel = false;
    (async () => {
      try {
        const r = await fetch("/api/session", { credentials: "include" });
        if (r.status === 401) {
          // Session expired or not authenticated
          if (!cancel) {
            setAuthed(false);
            setSessionChecked(true);
          }
          window.location.replace("/auth/login");
          return;
        }
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = (await r.json()) as SessionResp;
        if (!cancel) setAuthed(!!data.user);
      } catch {
        // Network error or other issue
        if (!cancel) setAuthed(false);
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

  // Periodic session validation (every 30 seconds)
  useEffect(() => {
    if (!authed) return;
    
    const interval = setInterval(() => {
      fetch("/api/session", { credentials: "include" }) .then(r => {
          if (r.status === 401) {
            setAuthed(false);
            setSessionChecked(true);
            clearInterval(interval);
            window.location.replace("/auth/login");
          }
        }) .catch(() => {
          // Network error - keep trying
        });
    }, 30000); // Check every 30 seconds
    
    return () => clearInterval(interval);
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
            fetch(`/api/containers/hosts/${encodeURIComponent(name)}`, { credentials: "include" }),
            fetch(`/api/iac/scopes/${encodeURIComponent(name)}`, { credentials: "include" }),
          ]);
          if (rc.status === 401 || ri.status === 401) { window.location.replace("/auth/login"); return; }
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

  // Early return for unauthenticated users
  if (!sessionChecked) {
    return <div className="min-h-screen bg-slate-950" />;
  }
  
  if (!authed) {
    return <LoginGate />;
  }

  // Pages bound to new routes (only rendered when authenticated)

  // Stacks list WITH DROPDOWN (wrap HostStacksView and remount on :hostName to avoid stale data)
  const HostStacksPage = () => {
    const { hostName } = useParams<{ hostName: string }>();
    const decodedHostName = hostName ? decodeURIComponent(hostName) : "";
    const host = hosts.find(h => h.name === decodedHostName);
    
    // Show loading if hosts are still loading
    if (loading) {
      return (
        <div className="min-h-screen bg-slate-950 text-white flex items-center justify-center">
          <div className="text-center">
            <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-white"></div>
            <p className="mt-4">Loading hosts...</p>
          </div>
        </div>
      );
    }
    
    // If hosts are loaded but host not found, redirect to hosts page
    if (!host) {
      navigate("/hosts");
      return null;
    }
    
    return (
      <HostStacksView
        key={hostName || "all"}
        host={host}
        hosts={hosts}
        onSync={() => refreshMetricsForHosts([host.name])}
        onOpenStack={(stackName) =>
          navigate(`/hosts/${encodeURIComponent(host.name)}/stacks/${encodeURIComponent(stackName)}`)
        }
        onHostChange={(newHostName) => navigate(`/hosts/${encodeURIComponent(newHostName)}/stacks`)}
      />
    );
  };

  // Stack details
  const StackDetailPage = () => {
    const { hostName, stackName } = useParams<{ hostName: string; stackName: string }>();
    const host = hosts.find(h => h.name === hostName);
    if (!host || !stackName) {
      navigate("/hosts");
      return null;
    }

    return (
      <StackDetailView
        host={host}
        stackName={stackName}
        onBack={() => navigate(`/hosts/${encodeURIComponent(host.name)}/stacks`)}
      />
    );
  };

  // Simple wrappers so images/networks/volumes remount on :hostName (prevents stale UI)
  const HostImagesPage = () => {
    const { hostName } = useParams<{ hostName: string }>();
    return <ImagesView key={hostName || "all"} hosts={hosts} />;
  };
  const HostNetworksPage = () => {
    const { hostName } = useParams<{ hostName: string }>();
    return <NetworksView key={hostName || "all"} hosts={hosts} />;
  };
  const HostVolumesPage = () => {
    const { hostName } = useParams<{ hostName: string }>();
    return <VolumesView key={hostName || "all"} hosts={hosts} />;
  };

  return (
    <div className="min-h-screen bg-slate-950 flex">
      <LeftNav
        page={getCurrentPage()}
        onGoDashboard={() => navigate('/dashboard')}
        onGoHosts={() => navigate('/hosts')}
        onGoStacks={() => {
          const h = getBestHost();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/stacks`);
          else navigate('/hosts');
        }}
        onGoGroups={() => navigate('/groups')}
        onGoImages={() => {
          const h = getBestHost();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/images`);
          else navigate('/hosts');
        }}
        onGoNetworks={() => {
          const h = getBestHost();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/networks`);
          else navigate('/hosts');
        }}
        onGoVolumes={() => {
          const h = getBestHost();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/volumes`);
          else navigate('/hosts');
        }}
        onGoCleanup={() => {
          const h = getBestHost();
          if (h) navigate(`/hosts/${encodeURIComponent(h)}/cleanup`);
          else navigate('/cleanup');
        }}
        onGoLogging={() => {
          navigate('/logging');
        }}
        onGoGitSync={() => {
          navigate('/git');
        }}
      />

      {/* Right side: layout unchanged */}
      <div className="flex-1 min-w-0 max-h-screen flex flex-col">
        <main className="flex-1 overflow-y-auto overflow-x-hidden p-3 sm:p-4 lg:p-6">
          <Routes>
            {/* Dashboard */}
            <Route path="/dashboard" element={<DashboardView hosts={hosts} />} />
            
            {/* Infrastructure routes */}
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
            <Route path="/groups" element={<GroupsView hosts={hosts} />} />
            
            {/* Cleanup routes */}
            <Route path="/cleanup" element={<CleanupView hosts={hosts} loading={loading} />} />
            <Route path="/hosts/:hostName/cleanup" element={<CleanupView hosts={hosts} loading={loading} />} />
            
            {/* Logging route */}
            <Route path="/logging" element={<LoggingView />} />
            
            {/* Git Sync route */}
            <Route path="/git" element={<GitSyncView />} />
            
            {/* Resource routes */}
            <Route path="/hosts/:hostName/images" element={<HostImagesPage />} />
            <Route path="/hosts/:hostName/networks" element={<HostNetworksPage />} />
            <Route path="/hosts/:hostName/volumes" element={<HostVolumesPage />} />
            
            {/* Default and catch-all */}
            <Route path="/" element={<DashboardView hosts={hosts} />} />
            <Route path="*" element={
              <div className="min-h-screen bg-slate-950 text-white flex items-center justify-center">
                <div className="text-center">
                  <h1 className="text-2xl mb-4 text-white">Page Not Found</h1>
                  <p className="text-slate-300 mb-6">The page you're looking for doesn't exist.</p>
                  <button 
                    onClick={() => navigate('/dashboard')} 
                    className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded transition-colors"
                  >
                    Go to Dashboard
                  </button>
                </div>
              </div>
            } />
          </Routes>
        </main>

        {/* Footer preserved */}
        <footer className="shrink-0 border-t border-slate-800 bg-slate-950/80">
          <div className="px-3 sm:px-4 lg:px-6 py-2 text-[10px] leading-none text-slate-500 text-center">
            © {new Date().getFullYear()} PrecisionPlanIT &amp; SoFMeRight (Kai)
          </div>
        </footer>
      </div>
    </div>
  );
}
