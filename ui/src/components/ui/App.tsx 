import React, { useMemo, useState } from "react";
import {
  Server,
  Boxes,
  GitBranch,
  RefreshCw,
  PlayCircle,
  Lock,
  Shield,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Clock,
  Search,
  Settings,
  Rocket,
  HardDrive,
  Layers,
  Terminal,
  Plus,
  KeyRound,
  EyeOff,
  ShieldCheck,
  FolderPlus,
  PanelLeft,
} from "lucide-react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";

const mockConfig = {
  repoProvider: "GitLab",
  repoUrl: "https://gitlab.com/precision/antparade",
  projectPath: "precision/antparade",
  branch: "main",
  tokenSet: true,
  localPath: "/opt/docker/ant-parade/docker-compose",
  refresh: "Every 10 min",
  autoPullLatest: true,
  applyOnChange: false,
  sops: { mode: "AGE", keyConfigured: true },
  stagingMode: true,
};

const mockData = [
  {
    host: "anchorage",
    groups: ["docker", "clustered"],
    lastSync: "2025-08-26T14:47:00-07:00",
    stacks: [
      {
        name: "grafana",
        type: "compose",
        path: "/opt/docker/ant-parade/docker-compose/anchorage/grafana",
        status: "drift",
        pullPolicy: "always",
        sops: true,
        createdAt: "2025-08-14T18:21:00-07:00",
        updatedAt: "2025-08-26T14:40:00-07:00",
        owner: "observability",
        containers: [
          {
            name: "grafana",
            image: "grafana/grafana:10.3",
            desiredImage: "grafana/grafana:10.4",
            state: "running",
            ports: "3000->3000/tcp",
            created: "2d ago",
            lastPulled: "7d ago",
          },
        ],
      },
      {
        name: "loki",
        type: "compose",
        path: "/opt/docker/ant-parade/docker-compose/anchorage/loki",
        status: "in_sync",
        pullPolicy: "digest_pinned",
        sops: false,
        owner: "observability",
        createdAt: "2025-08-10T09:20:00-07:00",
        updatedAt: "2025-08-19T09:20:00-07:00",
        containers: [
          {
            name: "loki",
            image: "grafana/loki@sha256:abcd…",
            desiredImage: "grafana/loki@sha256:abcd…",
            state: "running",
            ports: "3100->3100/tcp",
            created: "9d ago",
            lastPulled: "9d ago",
          },
        ],
      },
      {
        name: "bitwarden-identity",
        type: "script",
        path: "/opt/docker/ant-parade/docker-compose/anchorage/bitwarden-identity",
        status: "in_sync",
        pullPolicy: "n/a",
        sops: true,
        owner: "security",
        createdAt: "2025-08-23T11:05:00-07:00",
        updatedAt: "2025-08-26T11:05:00-07:00",
        containers: [
          {
            name: "bw-identity",
            image: "bitwarden/identity:2025.8",
            desiredImage: "bitwarden/identity:2025.8",
            state: "running",
            ports: "",
            created: "3d ago",
            lastPulled: "3d ago",
          },
        ],
      },
    ],
  },
  {
    host: "driftwood",
    groups: ["docker"],
    lastSync: "2025-08-26T14:43:00-07:00",
    stacks: [
      {
        name: "prom",
        type: "compose",
        path: "/opt/docker/ant-parade/docker-compose/driftwood/prom",
        status: "stopped",
        pullPolicy: "missing",
        sops: false,
        owner: "platform",
        createdAt: "2025-08-01T08:00:00-07:00",
        updatedAt: "2025-08-12T13:00:00-07:00",
        containers: [
          {
            name: "prometheus",
            image: "prom/prometheus:latest",
            desiredImage: "prom/prometheus:latest",
            state: "exited",
            ports: "9090->9090/tcp",
            created: "13d ago",
            lastPulled: "13d ago",
          },
        ],
      },
    ],
  },
];

const statusMeta = {
  in_sync: { label: "In Sync", icon: CheckCircle2, className: "bg-emerald-500/10 text-emerald-500" },
  drift: { label: "Drift", icon: AlertTriangle, className: "bg-amber-500/10 text-amber-500" },
  stopped: { label: "Stopped", icon: XCircle, className: "bg-slate-500/10 text-slate-400" },
  error: { label: "Error", icon: AlertTriangle, className: "bg-rose-500/10 text-rose-500" },
};

function StatPill({ status }: { status: keyof typeof statusMeta }) {
  const m = statusMeta[status] || statusMeta.in_sync;
  const Icon = m.icon as any;
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs ${m.className}`}>
      <Icon className="h-3.5 w-3.5" /> {m.label}
    </span>
  );
}

function SopsBadge({ enabled }: { enabled: boolean }) {
  const Icon = (enabled ? Lock : Shield) as any;
  return (
    <Badge variant="outline" className="border-slate-700/60 text-slate-300 bg-slate-800/40">
      <Icon className="h-3.5 w-3.5 mr-1" /> SOPS {enabled ? "enabled" : "—"}
    </Badge>
  );
}

function MaskedSecret({ label = "", configured = false }) {
  return (
    <div className="inline-flex items-center gap-2 px-2 py-1 rounded-md bg-slate-900/60 border border-slate-800 text-xs">
      <KeyRound className="h-3.5 w-3.5 text-slate-400" />
      <span className="text-slate-300">{label}</span>
      <span className={`ml-1 ${configured ? "text-emerald-400" : "text-rose-400"}`}>{configured ? "configured" : "missing"}</span>
      <EyeOff className="h-3.5 w-3.5 text-slate-500 ml-1" />
    </div>
  );
}

function LicenseBadge({ edition = "Community" }: { edition?: string }) {
  return (
    <Badge variant="outline" className="border-slate-700/60 text-slate-300 px-2 py-0.5 text-[11px] leading-none">
      {edition}
    </Badge>
  );
}

function SideNav({
  open,
  tab,
  setTab,
  onClose,
  license,
}: {
  open: boolean;
  tab: "settings" | "license";
  setTab: (t: "settings" | "license") => void;
  onClose: () => void;
  license: { edition: string };
}) {
  return (
    <aside className={`fixed inset-y-0 left-0 z-[70] w-80 bg-slate-950/95 border-r border-slate-800 transform transition-transform duration-200 ${open ? "translate-x-0" : "-translate-x-full"}`}>
      <div className="h-14 px-4 border-b border-slate-800 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <DDLogo />
          <div className="font-black uppercase tracking-tight text-slate-200">DD<span className="bg-clip-text text-transparent bg-gradient-to-r from-[#74ecbe] to-[#60a5fa]">UI</span></div>
        </div>
        <Button variant="ghost" size="icon" onClick={onClose}><XCircle className="h-5 w-5 text-slate-300" /></Button>
      </div>
      <div className="p-3 space-y-2">
        <div className="flex items-center justify-between px-2 py-1">
          <div className="text-xs text-slate-400">Edition</div>
          <LicenseBadge edition={license.edition} />
        </div>
        <div className="px-2 text-[11px] text-slate-500">for non-commercial / homelab use</div>
        <div className="mt-3 grid gap-1">
          <Button variant={tab === "settings" ? "default" : "outline"} className={`${tab === "settings" ? "bg-slate-800 text-white" : "border-slate-700 text-slate-200"}`} onClick={() => setTab("settings")}>Settings</Button>
          <Button variant={tab === "license" ? "default" : "outline"} className={`${tab === "license" ? "bg-slate-800 text-white" : "border-slate-700 text-slate-200"}`} onClick={() => setTab("license")}>License</Button>
        </div>
        {tab === "settings" && (
          <div className="mt-4 space-y-3 text-sm text-slate-300">
            <div className="text-slate-400 text-xs">General</div>
            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <span>Auto pull latest</span>
                <Switch />
              </div>
              <div className="flex items-center justify-between">
                <span>Apply on change</span>
                <Switch />
              </div>
            </div>
          </div>
        )}
        {tab === "license" && (
          <div className="mt-4 space-y-3 text-sm text-slate-300">
            <div className="text-slate-400 text-xs">License</div>
            <div className="text-xs text-slate-500">Load license from a secret file or environment variable.</div>
            <div>
              <div className="text-xs text-slate-400 mb-1">Secret file path</div>
              <Input defaultValue="/run/secrets/ddui_license" className="bg-slate-900/60 border-slate-800" />
            </div>
            <div>
              <div className="text-xs text-slate-400 mb-1">Environment variable</div>
              <Input defaultValue="DDUI_LICENSE" className="bg-slate-900/60 border-slate-800" />
            </div>
            <div className="text-xs text-slate-500">You can generate a free Community license in the app or on the site and mount it as a secret.</div>
            <div className="flex gap-2">
              <Button className="bg-[#74ecbe] text-slate-900">Apply License</Button>
              <Button variant="outline" className="border-slate-700 text-slate-200">Upload File</Button>
            </div>
          </div>
        )}
      </div>
    </aside>
  );
}

function DDLogo() {
  return (
    <div className="relative h-12 w-12 rounded-2xl bg-slate-900 border border-slate-800 grid place-items-center overflow-hidden shadow-[0_0_0_4px_rgba(116,236,190,0.12)]">
      <svg viewBox="0 0 64 64" className="h-11 w-11">
        <defs>
          <linearGradient id="gBeer" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="#fde68a" />
            <stop offset="100%" stopColor="#f59e0b" />
          </linearGradient>
        </defs>
        <rect x="14" y="6" width="36" height="52" rx="6" ry="6" fill="#0b1220" stroke="#334155" strokeWidth="2" />
        <rect x="16" y="16" width="32" height="40" rx="5" ry="5" fill="url(#gBeer)" />
        <g fill="#ffffff">
          <circle cx="20" cy="16" r="3" />
          <circle cx="26" cy="14" r="4" />
          <circle cx="33" cy="15" r="3" />
          <circle cx="39" cy="14" r="4" />
          <circle cx="45" cy="16" r="3" />
        </g>
        <path d="M18 28 Q 26 24 34 28 T 50 28" fill="none" stroke="#93c5fd" strokeWidth="2" opacity="0.9" />
        <g>
          <path d="M22 27.5c4 5 16 5 20 0H22Z" fill="#0ea5e9" />
          <g fill="#60a5fa">
            <rect x="24" y="22" width="5.5" height="5.5" rx="1" />
            <rect x="30.5" y="22" width="5.5" height="5.5" rx="1" />
            <rect x="37" y="22" width="5.5" height="5.5" rx="1" />
            <rect x="27.25" y="17" width="5.5" height="5.5" rx="1" />
            <rect x="33.75" y="17" width="5.5" height="5.5" rx="1" />
          </g>
          <rect x="36.5" y="13" width="1.5" height="9" fill="#1e40af" />
          <path d="M38 22l7-4-7-3v7Z" fill="#93c5fd" />
        </g>
      </svg>
    </div>
  );
}

function HeaderBar({ cfg, onOpenWizard, onToggleNav, onOpenSettings, licenseEdition }: { cfg: typeof mockConfig; onOpenWizard: () => void; onToggleNav: () => void; onOpenSettings: () => void; licenseEdition: string }) {
  return (
    <div className="bg-slate-950 border-b border-slate-800">
      <div className="max-w-7xl mx-auto px-4 py-3 flex items-center gap-3">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" onClick={onToggleNav}><PanelLeft className="h-5 w-5 text-slate-300" /></Button>
          <DDLogo />
          <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none flex items-center gap-2">
            DD<span className="bg-clip-text text-transparent bg-gradient-to-r from-[#74ecbe] to-[#60a5fa]">UI</span>
            <LicenseBadge edition={licenseEdition} />
          </div>
        </div>
        <Separator orientation="vertical" className="mx-2 h-8 bg-slate-800" />
        <div className="hidden md:flex items-center gap-2 text-xs">
          <Badge className="bg-slate-800 text-slate-200">
            <GitBranch className="h-3.5 w-3.5 mr-1" /> {cfg.projectPath}@{cfg.branch}
          </Badge>
          <Badge variant="outline" className="border-slate-700/60 text-slate-300">
            <HardDrive className="h-3.5 w-3.5 mr-1" /> {cfg.localPath}
          </Badge>
          <Badge variant="outline" className="border-slate-700/60 text-slate-300">
            <Clock className="h-3.5 w-3.5 mr-1" /> Refresh: {cfg.refresh}
          </Badge>
          <SopsBadge enabled={cfg.sops.keyConfigured} />
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Button className="bg-[#310937] hover:bg-[#2a0830] text-white"><RefreshCw className="h-4 w-4 mr-1" /> Sync</Button>
          <Button variant="outline" className="border-[#74ecbe] bg-slate-900/70 text-white hover:bg-slate-800 focus-visible:ring-2 focus-visible:ring-[#74ecbe] focus-visible:ring-offset-0 font-medium"><PlayCircle className="h-4 w-4 mr-1" /> Plan</Button>
          <Button className="bg-[#74ecbe] hover:bg-[#63d9ad] text-slate-900"><Rocket className="h-4 w-4 mr-1" /> Apply</Button>
          <Button className="bg-[#74ecbe] hover:bg-[#63d9ad] text-slate-900" onClick={onOpenWizard}><Plus className="h-4 w-4 mr-1" /> Add Deployment</Button>
          <Button variant="ghost" size="icon" onClick={onOpenSettings}><Settings className="h-5 w-5 text-slate-300" /></Button>
        </div>
      </div>
    </div>
  );
}

function MetricCard({ title, value, icon: Icon, accent }: { title: string; value: React.ReactNode; icon: any; accent?: boolean }) {
  return (
    <Card className={`border-slate-800 ${accent ? "bg-slate-900/40 border-[#74ecbe66]" : "bg-slate-900/40"}`}>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-slate-300">{title}</CardTitle>
        <Icon className="h-4 w-4 text-slate-400" />
      </CardHeader>
      <CardContent>
        <div className={`text-2xl font-extrabold ${accent ? "text-[#74ecbe]" : "text-white"}`}>{value}</div>
      </CardContent>
    </Card>
  );
}

function ContainerList({ items }: { items: any[] }) {
  return (
    <div className="rounded-xl border border-slate-800 divide-y divide-slate-800">
      {items.map((c, idx) => (
        <div key={idx} className="p-3 flex flex-col gap-2">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2 min-w-0">
              <span className="font-semibold text-slate-100 truncate max-w-[14rem]" title={c.name}>{c.name}</span>
              {c.state === "running" ? (
                <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-emerald-500/10 text-emerald-400">running</span>
              ) : (
                <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-slate-500/10 text-slate-400">{c.state}</span>
              )}
            </div>
            <div className="text-xs text-slate-400">{c.created}</div>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm">
            <div className="text-slate-300 break-all">
              <span className="text-slate-400">image:</span> {c.image}
              {c.desiredImage && c.desiredImage !== c.image && (
                <span className="text-amber-400 block">desired → {c.desiredImage}</span>
              )}
            </div>
            <div className="text-slate-300">
              <span className="text-slate-400">ports:</span> {c.ports || "—"}
            </div>
            <div className="text-slate-300">
              <span className="text-slate-400">last pulled:</span> {c.lastPulled}
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="outline" size="sm" className="border-slate-700 text-slate-200"><RefreshCw className="h-3.5 w-3.5 mr-1" /> Pull</Button>
            <Button variant="outline" size="sm" className="border-slate-700 text-slate-200"><Terminal className="h-3.5 w-3.5 mr-1" /> Logs</Button>
          </div>
        </div>
      ))}
    </div>
  );
}

function StackCard({ stack }: { stack: any }) {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <Card className="bg-slate-900/50 border-slate-800 hover:shadow-lg rounded-2xl overflow-hidden h-full flex flex-col">
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2">
                <Badge className="bg-[#310937] text-white">{stack.type}</Badge>
                <StatPill status={stack.status} />
                <SopsBadge enabled={stack.sops} />
              </div>
              <CardTitle className="mt-2 text-2xl font-extrabold tracking-tight text-white">{stack.name}</CardTitle>
              <div className="text-xs text-slate-400 flex items-center gap-1 min-w-0"><Layers className="h-3.5 w-3.5" /> <span className="truncate max-w-[18rem]" title={stack.path}>{stack.path}</span></div>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant="outline" className="border-slate-700/60 text-slate-300">policy: {stack.pullPolicy}</Badge>
              <Button variant="outline" className="border-[#74ecbe] bg-slate-900/70 text-white hover:bg-slate-800 focus-visible:ring-2 focus-visible:ring-[#74ecbe] focus-visible:ring-offset-0 font-medium" size="sm"><PlayCircle className="h-4 w-4 mr-1" /> Plan</Button>
              <Button className="bg-[#74ecbe] hover:bg-[#63d9ad] text-slate-900" size="sm"><Rocket className="h-4 w-4 mr-1" /> Apply</Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="pb-4 flex-1 flex flex-col">
          <div className="flex items-center justify-between text-sm"><div className="text-white font-semibold">{stack.containers.length} container{stack.containers.length > 1 ? "s" : ""}</div>
            <Button variant="ghost" size="sm" onClick={() => setOpen((v) => !v)} className="text-slate-300 hover:text-white">
              {open ? "Hide containers" : "Show containers"}
            </Button>
          </div>
          {open && (
            <div className="mt-3">
              <ContainerList items={stack.containers} />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function HostSection({ host }: { host: any }) {
  const counts = useMemo(() => {
    const total = host.stacks.length;
    const drift = host.stacks.filter((s: any) => s.status === "drift").length;
    const stopped = host.stacks.filter((s: any) => s.status === "stopped").length;
    return { total, drift, stopped };
  }, [host]);

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="h-9 w-9 rounded-xl bg-slate-900 grid place-items-center border border-slate-800"><Server className="h-5 w-5 text-slate-300" /></div>
          <div>
            <div className="text-xs text-slate-400">host</div>
            <div className="text-lg font-semibold">{host.host}</div>
          </div>
          <div className="ml-2 flex items-center gap-2">
            {host.groups.map((g: string) => (
              <Badge key={g} variant="outline" className="border-slate-700/60 text-slate-300">{g}</Badge>
            ))}
          </div>
        </div>
        <div className="text-xs text-slate-400 flex items-center gap-2">
          <Clock className="h-3.5 w-3.5" /> Last sync {new Date(host.lastSync).toLocaleTimeString()}
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4 items-stretch">
        {host.stacks.map((s: any) => (
          <StackCard key={s.name} stack={s} />
        ))}
      </div>

      <div className="flex items-center gap-3 text-sm text-slate-300">
        <span>Stacks: <span className="text-[#74ecbe] font-semibold">{counts.total}</span></span>
        <span>Drift: <span className="text-amber-400">{counts.drift}</span></span>
        <span>Stopped: <span className="text-white font-semibold">{counts.stopped}</span></span>
      </div>
    </section>
  );
}

function hasDrift(stack: any) {
  return (stack.containers || []).some((c: any) => c.desiredImage && c.desiredImage !== c.image);
}

function HostManifestSection({ host }: { host: any }) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="h-9 w-9 rounded-xl bg-slate-900 grid place-items-center border border-slate-800"><Server className="h-5 w-5 text-slate-300" /></div>
          <div>
            <div className="text-xs text-slate-400">host</div>
            <div className="text-lg font-semibold">{host.host}</div>
          </div>
          <div className="ml-2 flex items-center gap-2">
            {host.groups.map((g: string) => (
              <Badge key={g} variant="outline" className="border-slate-700/60 text-slate-300">{g}</Badge>
            ))}
          </div>
        </div>
        <div className="text-xs text-slate-400 flex items-center gap-2">
          <Clock className="h-3.5 w-3.5" /> Last sync {new Date(host.lastSync).toLocaleTimeString()}
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-3 text-left">Stack</th>
              <th className="p-3 text-left">Type</th>
              <th className="p-3 text-left">Status</th>
              <th className="p-3 text-left">Containers</th>
              <th className="p-3 text-left">Images</th>
              <th className="p-3 text-left">Ports</th>
              <th className="p-3 text-left">Policy</th>
              <th className="p-3 text-left">SOPS</th>
              <th className="p-3 text-left">Created</th>
              <th className="p-3 text-left">Modified</th>
              <th className="p-3 text-left">Owner</th>
              <th className="p-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {host.stacks.map((s: any, idx: number) => {
              const total = s.containers.length;
              const running = s.containers.filter((c: any) => c.state === "running").length;
              const ports = s.containers.reduce((acc: number, c: any) => acc + (c.ports ? String(c.ports).split(",").filter(Boolean).length : 0), 0);
              const firstImg = s.containers[0]?.image || "—";
              const firstDesired = s.containers[0]?.desiredImage || "";
              const drift = hasDrift(s);
              const createdAt = s.createdAt ? new Date(s.createdAt).toLocaleDateString() : "—";
              const updatedAt = s.updatedAt ? new Date(s.updatedAt).toLocaleDateString() : "—";
              const owner = s.owner || "—";
              return (
                <tr key={idx} className="border-t border-slate-800 hover:bg-slate-900/40">
                  <td className="p-3 font-medium text-slate-200">{s.name}</td>
                  <td className="p-3 text-slate-300">{s.type}</td>
                  <td className="p-3"><StatPill status={s.status} /></td>
                  <td className="p-3 text-white font-semibold">{running}/{total}</td>
                  <td className="p-3 text-slate-300">
                    <div className="flex items-center gap-2">
                      <span>{firstImg}</span>
                      {drift && <span className="text-amber-400 text-xs">→ {firstDesired}</span>}
                    </div>
                  </td>
                  <td className="p-3 text-slate-300">{ports || "—"}</td>
                  <td className="p-3 text-slate-300">{s.pullPolicy}</td>
                  <td className="p-3">{s.sops ? <Lock className="h-4 w-4 text-slate-300" /> : <Shield className="h-4 w-4 text-slate-500" />}</td>
                  <td className="p-3 text-slate-300" title={s.createdAt || ""}>{createdAt}</td>
                  <td className="p-3 text-slate-300" title={s.updatedAt || ""}>{updatedAt}</td>
                  <td className="p-3 text-slate-300">{owner}</td>
                  <td className="p-3 text-right">
                    <div className="flex justify-end gap-2">
                      <Button variant="outline" size="sm" className="border-[#74ecbe] bg-slate-900/70 text-white hover:bg-slate-800 focus-visible:ring-2 focus-visible:ring-[#74ecbe] focus-visible:ring-offset-0 font-medium"><PlayCircle className="h-3.5 w-3.5 mr-1" /> Plan</Button>
                      <Button size="sm" className="bg-[#310937] hover:bg-[#2a0830] text-white"><Rocket className="h-3.5 w-3.5 mr-1" /> Apply</Button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function StepLabel({ n, label }: { n: number; label: string }) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <div className="h-6 w-6 rounded-full grid place-items-center bg-slate-800 border border-slate-700 text-slate-200">{n}</div>
      <div className="text-slate-300">{label}</div>
    </div>
  );
}

function AddDeploymentWizard({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [step, setStep] = useState(1);
  const [source, setSource] = useState<"repo" | "local">("repo");
  const [form, setForm] = useState<any>({
    repoUrl: mockConfig.repoUrl,
    branch: mockConfig.branch,
    localPath: mockConfig.localPath,
    host: "anchorage",
    stack: "",
    type: "compose",
    pullPolicy: "always",
    removeConflicting: true,
    sopsConfigured: mockConfig.sops.keyConfigured,
  });

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[60]">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="absolute inset-x-0 top-10 mx-auto max-w-3xl">
        <Card className="bg-slate-950/95 border-slate-800 shadow-2xl">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <FolderPlus className="h-5 w-5" /> New Deployment
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-5">
            <div className="grid grid-cols-4 gap-3">
              <StepLabel n={1} label="Source" />
              <StepLabel n={2} label="Configure" />
              <StepLabel n={3} label="Secrets" />
              <StepLabel n={4} label="Review" />
            </div>

            {step === 1 && (
              <div className="space-y-4">
                <div className="text-sm text-slate-300">Where should we read stack files from?</div>
                <div className="flex items-center gap-4">
                  <label className={`px-3 py-2 rounded-lg border ${source === "repo" ? "border-emerald-500/60 bg-emerald-500/5" : "border-slate-800 bg-slate-900/50"}`}>
                    <input type="radio" name="source" className="mr-2" checked={source === "repo"} onChange={() => setSource("repo")} /> Repo (Git)
                  </label>
                  <label className={`px-3 py-2 rounded-lg border ${source === "local" ? "border-emerald-500/60 bg-emerald-500/5" : "border-slate-800 bg-slate-900/50"}`}>
                    <input type="radio" name="source" className="mr-2" checked={source === "local"} onChange={() => setSource("local")} /> Local path
                  </label>
                </div>
                {source === "repo" ? (
                  <div className="grid md:grid-cols-2 gap-3">
                    <div>
                      <div className="text-xs text-slate-400 mb-1">Repository URL</div>
                      <Input value={form.repoUrl} onChange={(e) => setForm({ ...form, repoUrl: e.target.value })} className="bg-slate-900/60 border-slate-800" />
                    </div>
                    <div>
                      <div className="text-xs text-slate-400 mb-1">Branch</div>
                      <Input value={form.branch} onChange={(e) => setForm({ ...form, branch: e.target.value })} className="bg-slate-900/60 border-slate-800" />
                    </div>
                    <div className="md:col-span-2 flex items-center gap-2">
                      <MaskedSecret label="Repo token" configured={true} />
                      <span className="text-xs text-slate-500">Token is never rendered; used only for fetch.</span>
                    </div>
                  </div>
                ) : (
                  <div>
                    <div className="text-xs text-slate-400 mb-1">Local Path</div>
                    <Input value={form.localPath} onChange={(e) => setForm({ ...form, localPath: e.target.value })} className="bg-slate-900/60 border-slate-800" />
                    <div className="text-xs text-slate-500 mt-1">Local staging lets you dry-run before committing to Git.</div>
                  </div>
                )}
              </div>
            )}

            {step === 2 && (
              <div className="grid md:grid-cols-2 gap-4">
                <div>
                  <div className="text-xs text-slate-400 mb-1">Host / Group</div>
                  <Input placeholder="anchorage or group name" className="bg-slate-900/60 border-slate-800" value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} />
                </div>
                <div>
                  <div className="text-xs text-slate-400 mb-1">Stack name</div>
                  <Input placeholder="grafana" className="bg-slate-900/60 border-slate-800" value={form.stack} onChange={(e) => setForm({ ...form, stack: e.target.value })} />
                </div>
                <div>
                  <div className="text-xs text-slate-400 mb-1">Type</div>
                  <div className="flex items-center gap-4 text-sm">
                    <label><input type="radio" name="t" checked={form.type === "compose"} onChange={() => setForm({ ...form, type: "compose" })} /> <span className="ml-1">Compose</span></label>
                    <label><input type="radio" name="t" checked={form.type === "script"} onChange={() => setForm({ ...form, type: "script" })} /> <span className="ml-1">Script</span></label>
                  </div>
                </div>
                <div>
                  <div className="text-xs text-slate-400 mb-1">Pull policy</div>
                  <div className="flex items-center gap-4 text-sm">
                    <label><input type="radio" name="pp" checked={form.pullPolicy === "always"} onChange={() => setForm({ ...form, pullPolicy: "always" })} /> <span className="ml-1">always</span></label>
                    <label><input type="radio" name="pp" checked={form.pullPolicy === "missing"} onChange={() => setForm({ ...form, pullPolicy: "missing" })} /> <span className="ml-1">missing</span></label>
                    <label><input type="radio" name="pp" checked={form.pullPolicy === "digest_pinned"} onChange={() => setForm({ ...form, pullPolicy: "digest_pinned" })} /> <span className="ml-1">digest_pinned</span></label>
                  </div>
                </div>
                <div className="md:col-span-2">
                  <label className="flex items-center gap-2 text-sm text-slate-300">
                    <Switch checked={form.removeConflicting} onCheckedChange={(v: any) => setForm({ ...form, removeConflicting: v })} />
                    Remove conflicting named containers
                  </label>
                </div>
              </div>
            )}

            {step === 3 && (
              <div className="space-y-3">
                <div className="text-sm text-slate-300">Secrets (SOPS/AGE)</div>
                <div className="text-xs text-slate-400">AGE private key is read only at runtime and never persisted. Decrypted envs live in tmpfs and are shredded after apply.</div>
                <div className="flex items-center gap-3">
                  <MaskedSecret label="AGE key" configured={true} />
                  <Badge variant="outline" className="border-slate-700/60 text-slate-300"><ShieldCheck className="h-3.5 w-3.5 mr-1" /> in-memory only</Badge>
                </div>
                <div className="grid md:grid-cols-2 gap-3">
                  <div>
                    <div className="text-xs text-slate-400 mb-1">Env files</div>
                    <Input placeholder=".env, .env.local, .env.sops" className="bg-slate-900/60 border-slate-800" />
                  </div>
                  <div>
                    <div className="text-xs text-slate-400 mb-1">Hooks (pre/post) optional</div>
                    <Input placeholder="./pre.sh, ./smoke-test.sh" className="bg-slate-900/60 border-slate-800" />
                  </div>
                </div>
              </div>
            )}

            {step === 4 && (
              <div className="space-y-3">
                <div className="text-sm text-slate-300">Review</div>
                <div className="bg-slate-900/60 border border-slate-800 rounded-xl p-3 text-sm text-slate-200">
                  <div><b>Target</b>: {form.host} / {form.stack} ({form.type})</div>
                  <div><b>Policy</b>: {form.pullPolicy} • removeConflicting: {String(form.removeConflicting)}</div>
                  <div><b>Source</b>: {source === "repo" ? `${form.repoUrl}@${form.branch}` : form.localPath}</div>
                  <div className="text-xs text-slate-400 mt-2">A dry-run plan will be shown before any changes are applied.</div>
                </div>
              </div>
            )}

            <div className="flex items-center justify-between">
              <div className="text-xs text-slate-500 flex items-center gap-2">
                <ShieldCheck className="h-3.5 w-3.5" /> Security by default: keys masked, never persisted; tmpfs decrypt.
              </div>
              <div className="flex items-center gap-2">
                <Button variant="outline" className="border-slate-700 text-slate-200" onClick={onClose}>Cancel</Button>
                {step > 1 && <Button variant="outline" className="border-slate-700 text-slate-200" onClick={() => setStep(step - 1)}>Back</Button>}
                {step < 4 && <Button className="bg-[#74ecbe] hover:bg-[#63d9ad] text-slate-900" onClick={() => setStep(step + 1)}>Next</Button>}
                {step === 4 && <Button className="bg-[#74ecbe] hover:bg-[#63d9ad] text-slate-900"><PlayCircle className="h-4 w-4 mr-1" /> Plan & Create</Button>}
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function FiltersBar({ query, setQuery, toggles, setToggles }: any) {
  return (
    <div className="flex flex-col md:flex-row gap-3 md:items-center md:justify-between">
      <div className="flex items-center gap-2 w-full md:w-auto">
        <div className="relative w-full md:w-96">
          <Search className="h-4 w-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
          <Input
            placeholder="Filter by host, stack, image…"
            className="pl-9 bg-slate-900/50 border-slate-800 text-slate-200 placeholder:text-slate-500"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      </div>
      <div className="flex items-center gap-5">
        <label className="flex items-center gap-2 text-sm text-slate-300">
          <Switch checked={toggles.staging} onCheckedChange={(v: any) => setToggles((t: any) => ({ ...t, staging: v }))} />
          Staging Mode (use local path)
        </label>
        <label className="flex items-center gap-2 text-sm text-slate-300">
          <Switch checked={toggles.autoPull} onCheckedChange={(v: any) => setToggles((t: any) => ({ ...t, autoPull: v }))} />
          Auto pull latest
        </label>
        <label className="flex items-center gap-2 text-sm text-slate-300">
          <Switch checked={toggles.applyOnChange} onCheckedChange={(v: any) => setToggles((t: any) => ({ ...t, applyOnChange: v }))} />
          Apply on change
        </label>
      </div>
    </div>
  );
}

export default function AntParadeHubMock() {
  const [query, setQuery] = useState("");
  const [toggles, setToggles] = useState({ staging: mockConfig.stagingMode, autoPull: mockConfig.autoPullLatest, applyOnChange: mockConfig.applyOnChange });
  const [showWizard, setShowWizard] = useState(false);
  const [navOpen, setNavOpen] = useState(false);
  const [navTab, setNavTab] = useState<"settings" | "license">("settings");

  const flatStacks = useMemo(() => mockData.flatMap((h) => h.stacks.map((s) => ({ ...s, __host: h.host }))), []);
  const metrics = useMemo(() => {
    const stacks = flatStacks.length;
    const containers = flatStacks.reduce((acc, s: any) => acc + s.containers.length, 0);
    const drift = flatStacks.filter((s: any) => s.status === "drift").length;
    const errors = flatStacks.filter((s: any) => s.status === "error").length;
    return { stacks, containers, drift, errors };
  }, [flatStacks]);

  const filteredHosts = useMemo(() => {
    if (!query.trim()) return mockData;
    const q = query.toLowerCase();
    return mockData
      .map((h) => ({
        ...h,
        stacks: h.stacks.filter((s: any) => [
          h.host,
          s.name,
          s.path,
          s.type,
          ...s.containers.map((c: any) => `${c.name} ${c.image} ${c.desiredImage}`),
        ].join(" ").toLowerCase().includes(q)),
      }))
      .filter((h) => h.stacks.length > 0);
  }, [query]);

  return (
    <div className="relative min-h-screen bg-slate-950 text-slate-100">
      <HeaderBar
        cfg={mockConfig}
        licenseEdition="Community"
        onOpenWizard={() => setShowWizard(true)}
        onToggleNav={() => setNavOpen(true)}
        onOpenSettings={() => { setNavTab("settings"); setNavOpen(true); }}
      />

      <main className="max-w-7xl mx-auto px-4 py-6 space-y-6">
        <div className="grid md:grid-cols-4 gap-4">
          <MetricCard title="Stacks" value={metrics.stacks} icon={Boxes} accent />
          <MetricCard title="Containers" value={metrics.containers} icon={Layers} accent />
          <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
          <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
        </div>

        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-4">
            <FiltersBar query={query} setQuery={setQuery} toggles={toggles} setToggles={setToggles} />
          </CardContent>
        </Card>

        <Tabs defaultValue="manifest" className="mt-2">
          <TabsList className="bg-slate-900/60 border border-slate-800">
            <TabsTrigger value="manifest">Manifest View</TabsTrigger>
            <TabsTrigger value="cards">Cards View</TabsTrigger>
          </TabsList>

          <TabsContent value="manifest" className="mt-4 space-y-10">
            {filteredHosts.map((h) => (
              <HostManifestSection key={h.host} host={h} />
            ))}
            {filteredHosts.length === 0 && <div className="text-center py-20 text-slate-400">No stacks match your filter.</div>}
          </TabsContent>

          <TabsContent value="cards" className="mt-4 space-y-10">
            {filteredHosts.map((h) => (
              <HostSection key={h.host} host={h} />
            ))}
            {filteredHosts.length === 0 && <div className="text-center py-20 text-slate-400">No stacks match your filter.</div>}
          </TabsContent>
        </Tabs>
      </main>

      <div className="border-t border-slate-800 bg-slate-950/80">
        <div className="max-w-7xl mx-auto px-4 py-4 text-xs text-slate-300 flex flex-wrap items-center gap-2">
          <ShieldCheck className="h-4 w-4" /> Security by default:
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">AGE key never persisted</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Decrypt to tmpfs only</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
        </div>
      </div>

      <footer className="border-t border-slate-800 bg-slate-950/90">
        <div className="max-w-7xl mx-auto px-4 py-4 text-xs text-slate-400 flex flex-wrap items-center gap-2">
          <span>Designated Driver UI (DDUI)</span>
          <span className="text-slate-600">•</span>
          <span>© 2025 PrecisionPlanIT • HomelabHelpdesk • SoFMeRight</span>
          <span className="ml-auto">v0.1.0</span>
        </div>
      </footer>

      <SideNav
        open={navOpen}
        tab={navTab}
        setTab={setNavTab}
        onClose={() => setNavOpen(false)}
        license={{ edition: "Community Edition" }}
      />

      <AddDeploymentWizard open={showWizard} onClose={() => setShowWizard(false)} />
    </div>
  );
}
