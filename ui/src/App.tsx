import React, { useEffect, useMemo, useState } from "react";
import LeftNav from "./components/LeftNav";
import DeploymentsView from "./DeploymentsView";
import ImagesView from "./ImagesView";
import NetworksView from "./NetworksView";
import VolumesView from "./VolumesView";
import { HostStacksView } from "./HostStacksView";
import { StackDetailView } from "./StackDetailView";

/* ===== Types used in App for metrics/routing ===== */
export type Host = {
  name: string;
  address?: string;
  groups?: string[];
};

type ApiContainer = {
  name: string;
  image: string;
  state: string;
  status: string;
  owner?: string;
  ports?: any;
  labels?: Record<string, string>;
  updated_at?: string;
  created_ts?: string;
  ip_addr?: string;
  compose_project?: string;
  compose_service?: string;
  stack?: string | null;
};

type IacEnvFile = { path: string; sops: boolean };

type IacService = {
  id: number;
  stack_id: number;
  service_name: string;
  container_name?: string;
  image?: string;
  labels: Record<string, string>;
  env_keys: string[];
  env_files: IacEnvFile[];
  ports: any[];
  volumes: any[];
  deploy: Record<string, any>;
};

type IacStack = {
  id: number;
  name: string; // stack_name
  scope_kind: string;
  scope_name: string;
  deploy_kind: "compose" | "script" | "unmanaged" | string;
  pull_policy?: string;
  sops_status: "all" | "partial" | "none" | string;
  iac_enabled: boolean;
  rel_path: string;
  compose?: string;
  services: IacService[] | null | undefined;
};

type SessionResp = {
  user: null | {
    sub: string;
    email: string;
    name: string;
    picture?: string;
  };
};

export default function App() {
  const [query, setQuery] = useState("");
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loadingHosts, setLoadingHosts] = useState(true);
  const [hostsErr, setHostsErr] = useState<string | null>(null);

  const [page, setPage] = useState<"deployments" | "host" | "stack" | "images" | "networks" | "volumes">("deployments");
  const [activeHost, setActiveHost] = useState<Host | null>(null);
  const [activeStack, setActiveStack] = useState<{ name: string; iacId?: number } | null>(null);

  const [sessionChecked, setSessionChecked] = useState(false);
  const [authed, setAuthed] = useState<boolean>(false);

  const [scanning, setScanning] = useState(false);
  const [metricsCache, setMetricsCache] = useState<
    Record<string, { stacks: number; containers: number; drift: number; errors: number }>
  >({});

  /* ----- session check ----- */
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

  /* ----- load hosts ----- */
  useEffect(() => {
    if (!authed) return;
    let cancel = false;
    (async () => {
      setLoadingHosts(true); setHostsErr(null);
      try {
        const r = await fetch("/api/hosts", { credentials: "include" });
        if (r.status === 401) { window.location.replace("/auth/login"); return; }
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = await r.json();
        const items = Array.isArray(data.items) ? data.items : [];
        const mapped: Host[] = items.map((h: any) => ({
          name: h.name, address: h.addr ?? h.address ?? "", groups: h.groups ?? []
        }));
        if (!cancel) setHosts(mapped);
      } catch (e: any) {
        if (!cancel) setHostsErr(e?.message || "Failed to load hosts");
      } finally {
        if (!cancel) setLoadingHosts(false);
      }
    })();
    return () => { cancel = true; };
  }, [authed]);

  /* ----- filtering ----- */
  const filteredHosts = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter(h => [h.name, h.address || "", ...(h.groups || [])].join(" ").toLowerCase().includes(q));
  }, [hosts, query]);

  /* ----- metrics helpers ----- */
  const OK_STATES = new Set(["running", "created", "restarting", "healthy", "up"]);
  function isBadState(state?: string) {
    const s = (state || "").toLowerCase();
    if (!s) return false;
    for (const ok of OK_STATES) if (s.includes(ok)) return false;
    return true;
  }

  function computeHostMetrics(runtime: ApiContainer[], iac: IacStack[]) {
    const rtByStack = new Map<string, ApiContainer[]>();
    for (const c of runtime) {
      const key = (c.compose_project || c.stack || "(none)").trim() || "(none)";
      if (!rtByStack.has(key)) rtByStack.set(key, []);
      rtByStack.get(key)!.push(c);
    }
    const iacByName = new Map<string, IacStack>();
    for (const s of iac) iacByName.set(s.name, s);

    const names = new Set<string>([...rtByStack.keys(), ...iacByName.keys()]);

    let stacks = 0;
    let containers = runtime.length;
    let drift = 0;
    let errors = 0;

    for (const c of runtime) if (isBadState(c.state)) errors++;

    for (const sname of names) {
      stacks++;
      const rcs = rtByStack.get(sname) || [];
      const is = iacByName.get(sname);
      const services: IacService[] = Array.isArray(is?.services) ? (is!.services as IacService[]) : [];
      const hasIac = !!is && (services.length > 0 || !!is.compose);
      let stackDrift = false;

      const desiredImageFor = (c: ApiContainer): string | undefined => {
        if (!is || services.length === 0) return undefined;
        const svc = services.find(x =>
          (c.compose_service && x.service_name === c.compose_service) ||
          (x.container_name && x.container_name === c.name)
        );
        return svc?.image || undefined;
      };

      for (const c of rcs) {
        const desired = desiredImageFor(c);
        if (desired && desired.trim() && desired.trim() !== (c.image || "").trim()) { stackDrift = true; break; }
      }
      if (!stackDrift && is && services.length > 0) {
        for (const svc of services) {
          const match = rcs.some(c =>
            (c.compose_service && svc.service_name === c.compose_service) ||
            (svc.container_name && c.name === svc.container_name)
          );
          if (!match) { stackDrift = true; break; }
        }
      }
      if (!rcs.length && hasIac && services.length > 0) stackDrift = true;
      if (stackDrift) drift++;
    }

    return { stacks, containers, drift, errors };
  }

  async function refreshMetricsForHosts(hostNames: string[]) {
    if (!hostNames.length) return;
    const limit = 4;
    let idx = 0;
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

  const metricsSummary = useMemo(() => {
    let stacks = 0, containers = 0, drift = 0, errors = 0;
    for (const h of filteredHosts) {
      const m = metricsCache[h.name];
      if (!m) continue;
      stacks += m.stacks;
      containers += m.containers;
      drift += m.drift;
      errors += m.errors;
    }
    return { hosts: filteredHosts.length, stacks, containers, drift, errors };
  }, [filteredHosts, metricsCache]);

  async function handleScanAll() {
    if (scanning) return;
    setScanning(true);
    try {
      await fetch("/api/iac/scan", { method: "POST", credentials: "include" }).catch(()=>{});
      const res = await fetch("/api/scan/all", { method: "POST", credentials: "include" });
      if (res.status === 401) { window.location.replace("/auth/login"); return; }
      await res.json(); // result details already reflected by next metrics refresh
      await refreshMetricsForHosts(hosts.map(h => h.name));
    } finally {
      setScanning(false);
    }
  }

  async function handleScanHost(name: string) {
    if (scanning) return;
    setScanning(true);
    try {
      await fetch(`/api/scan/host/${encodeURIComponent(name)}`, { method: "POST", credentials: "include" });
      await refreshMetricsForHosts([name]);
    } finally {
      setScanning(false);
    }
  }

  function openHost(name: string) {
    const h = hosts.find(x => x.name === name) || { name };
    setActiveHost(h as Host);
    setActiveStack(null);
    setPage("host");
    refreshMetricsForHosts([name]);
  }

  function openStack(name: string, iacId?: number) {
    if (!activeHost) return;
    setActiveStack({ name, iacId });
    setPage("stack");
  }

  if (sessionChecked && !authed) return <div className="min-h-screen bg-slate-950" />; // redirect already fired
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
          {page === "deployments" && (
            <DeploymentsView
              hosts={hosts}
              filteredHosts={filteredHosts}
              loading={loadingHosts}
              err={hostsErr}
              query={query}
              setQuery={setQuery}
              scanning={scanning}
              onScanAll={handleScanAll}
              onScanHost={handleScanHost}
              onOpenHost={openHost}
              metricsSummary={metricsSummary}
              hostMetrics={metricsCache}
            />
          )}

          {page === "host" && activeHost && (
            <HostStacksView
              host={activeHost}
              onBack={() => setPage("deployments")}
              onSync={handleScanAll}
              onOpenStack={(name, id) => openStack(name, id)}
            />
          )}

          {page === "stack" && activeHost && activeStack && (
            <StackDetailView
              host={activeHost}
              stackName={activeStack.name}
              iacId={activeStack.iacId}
              onBack={() => setPage("host")}
            />
          )}

          {page === "images" && <ImagesView hosts={hosts} />}
          {page === "networks" && <NetworksView hosts={hosts} />}
          {page === "volumes" && <VolumesView hosts={hosts} />}

          <div className="pt-6 pb-10 text-center text-xs text-slate-500">
            © 2025 PrecisionPlanIT &amp; SoFMeRight (Kai)
          </div>
        </main>
      </div>
    </div>
  );
}
