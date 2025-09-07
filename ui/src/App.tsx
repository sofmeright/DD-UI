import React, { useEffect, useMemo, useState } from "react";
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

export default function App() {
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [metricsCache, setMetricsCache] = useState<Record<string, { stacks: number; containers: number; drift: number; errors: number }>>({});
  const [page, setPage] = useState<"deployments" | "host" | "stack" | "images" | "networks" | "volumes">("deployments");
  const [activeHost, setActiveHost] = useState<Host | null>(null);
  const [activeStack, setActiveStack] = useState<{ name: string; iacId?: number } | null>(null);
  const [sessionChecked, setSessionChecked] = useState(false);
  const [authed, setAuthed] = useState<boolean>(false);
  const [filterQuery, setFilterQuery] = useState("");

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

  function openHost(name: string) {
    const h = hosts.find(x => x.name === name) || { name } as Host;
    setActiveHost(h); setActiveStack(null); setPage("host"); refreshMetricsForHosts([name]);
  }

  function openStack(name: string, iacId?: number) {
    if (!activeHost) return; setActiveStack({ name, iacId }); setPage("stack");
  }

  if (sessionChecked && !authed) return <LoginGate />;
  if (!sessionChecked) return <div className="min-h-screen bg-slate-950" />;

  const hostMetrics = activeHost ? (metricsCache[activeHost.name] || { stacks: 0, containers: 0, drift: 0, errors: 0 }) : null;

  return (
    <div className="min-h-screen flex">
      <LeftNav
        page={page}
        onGoDeployments={() => setPage("deployments")}
        onGoImages={() => setPage("images")}
        onGoNetworks={() => setPage("networks")}
        onGoVolumes={() => setPage("volumes")}
      />
      <div className="flex-1 min-w-0">
        <main className="px-6 py-6 space-y-6">
          {page === 'deployments' && (
            <DeploymentsView
              metrics={metrics}
              hosts={hosts}
              filteredHosts={filteredHosts}
              loading={loading}
              err={err}
              scanning={scanning}
              onScanAll={handleScanAll}
              onFilter={setFilterQuery}
              onOpenHost={openHost}
              refreshMetricsForHosts={refreshMetricsForHosts}
            />
          )}

          {page === 'host' && activeHost && (
            <>
              {/* quick metrics row for host */}
              <div className="grid md:grid-cols-4 gap-4">
                <div className="bg-transparent" />
                <div className="bg-transparent" />
                <div className="bg-transparent" />
                <div className="bg-transparent" />
              </div>
              <HostStacksView host={activeHost} onBack={() => setPage('deployments')} onSync={handleScanAll} onOpenStack={openStack} />
            </>
          )}

          {page === 'stack' && activeHost && activeStack && (
            <StackDetailView host={activeHost} stackName={activeStack.name} iacId={activeStack.iacId} onBack={() => setPage('host')} />
          )}

          {page === 'images' && <ImagesView hosts={hosts} />}
          {page === 'networks' && <NetworksView hosts={hosts} />}
          {page === 'volumes' && <VolumesView hosts={hosts} />}

          <div className="pt-6 pb-10 text-center text-xs text-slate-500">© 2025 PrecisionPlanIT &amp; SoFMeRight (Kai)</div>
        </main>
      </div>
    </div>
  );
}
