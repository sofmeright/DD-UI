import React, { useMemo, useState } from "react";
import { Boxes, Layers, AlertTriangle, XCircle, Search, GitBranch, HardDrive, Clock, PanelLeft, RefreshCw, PlayCircle, Rocket, ShieldCheck } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";

const mockConfig = {
  projectPath: "precision/antparade",
  branch: "main",
  localPath: "/opt/docker/ant-parade/docker-compose",
  refresh: "Every 10 min",
  sopsConfigured: true,
  autoPullLatest: true,
  applyOnChange: false,
  stagingMode: true
};

const mockData = [
  { host: "anchorage", groups: ["docker", "clustered"], lastSync: new Date().toISOString(), stacks: [
    { name: "grafana", type: "compose", status: "drift", pullPolicy: "always", sops: true, containers: [{ name: "grafana", image: "grafana/grafana:10.3", desiredImage: "grafana/grafana:10.4", state: "running", ports: "3000->3000/tcp", created: "2d ago", lastPulled: "7d ago" }]}
  ]},
  { host: "driftwood", groups: ["docker"], lastSync: new Date().toISOString(), stacks: [
    { name: "prom", type: "compose", status: "stopped", pullPolicy: "missing", sops: false, containers: [{ name: "prometheus", image: "prom/prometheus:latest", desiredImage: "prom/prometheus:latest", state: "exited", ports: "9090->9090/tcp", created: "13d ago", lastPulled: "13d ago" }]}
  ]}
];

function MetricCard({ title, value, icon: Icon, accent=false }: { title: string; value: React.ReactNode; icon: any; accent?: boolean }) {
  return (
    <Card className={`border-slate-800 ${accent ? "bg-slate-900/40 border-brand/40" : "bg-slate-900/40"}`}>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-slate-300">{title}</CardTitle>
        <Icon className="h-4 w-4 text-slate-400" />
      </CardHeader>
      <CardContent>
        <div className={`text-2xl font-extrabold ${accent ? "text-brand" : "text-white"}`}>{value}</div>
      </CardContent>
    </Card>
  );
}

export default function App() {
  const [query, setQuery] = useState("");
  const [toggles, setToggles] = useState({ staging: mockConfig.stagingMode, autoPull: mockConfig.autoPullLatest, applyOnChange: mockConfig.applyOnChange });

  const flatStacks = useMemo(() => mockData.flatMap(h => h.stacks), []);
  const metrics = useMemo(() => {
    const stacks = flatStacks.length;
    const containers = flatStacks.reduce((acc, s:any) => acc + s.containers.length, 0);
    const drift = flatStacks.filter((s:any) => s.status === "drift").length;
    const errors = flatStacks.filter((s:any) => s.status === "error").length;
    return { stacks, containers, drift, errors };
  }, [flatStacks]);

  return (
    <div className="min-h-screen">
      <div className="bg-slate-950 border-b border-slate-800">
        <div className="max-w-7xl mx-auto px-4 py-3 flex items-center gap-3">
          <Button variant="ghost" size="icon"><PanelLeft className="h-5 w-5 text-slate-300" /></Button>
          <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none flex items-center gap-2">
            DD<span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">UI</span>
            <Badge variant="outline">Community</Badge>
          </div>
          <Separator orientation="vertical" className="mx-2 h-8 bg-slate-800" />
          <div className="hidden md:flex items-center gap-2 text-xs">
            <Badge className="bg-slate-800 text-slate-200"><GitBranch className="h-3.5 w-3.5 mr-1" /> {mockConfig.projectPath}@{mockConfig.branch}</Badge>
            <Badge variant="outline" className="border-slate-700/60 text-slate-300"><HardDrive className="h-3.5 w-3.5 mr-1" /> {mockConfig.localPath}</Badge>
            <Badge variant="outline" className="border-slate-700/60 text-slate-300"><Clock className="h-3.5 w-3.5 mr-1" /> Refresh: {mockConfig.refresh}</Badge>
            <Badge variant="outline" className="border-slate-700/60 text-slate-300">SOPS {mockConfig.sopsConfigured ? "enabled" : "—"}</Badge>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <Button className="bg-[#310937] hover:bg-[#2a0830] text-white"><RefreshCw className="h-4 w-4 mr-1" /> Sync</Button>
            <Button variant="outline" className="border-brand bg-slate-900/70 text-white hover:bg-slate-800"><PlayCircle className="h-4 w-4 mr-1" /> Plan</Button>
            <Button className="bg-brand hover:brightness-110 text-slate-900"><Rocket className="h-4 w-4 mr-1" /> Apply</Button>
          </div>
        </div>
      </div>

      <main className="max-w-7xl mx-auto px-4 py-6 space-y-6">
        <div className="grid md:grid-cols-4 gap-4">
          <MetricCard title="Stacks" value={metrics.stacks} icon={Boxes} accent />
          <MetricCard title="Containers" value={metrics.containers} icon={Layers} accent />
          <MetricCard title="Drift" value={<span className="text-amber-400">{metrics.drift}</span>} icon={AlertTriangle} />
          <MetricCard title="Errors" value={<span className="text-rose-400">{metrics.errors}</span>} icon={XCircle} />
        </div>

        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-4">
            <div className="flex flex-col md:flex-row gap-3 md:items-center md:justify-between">
              <div className="flex items-center gap-2 w-full md:w-auto">
                <div className="relative w-full md:w-96">
                  <Search className="h-4 w-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
                  <Input placeholder="Filter by host, stack, image…" className="pl-9 bg-slate-900/50 border-slate-800 text-slate-200 placeholder:text-slate-500" value={query} onChange={(e) => setQuery(e.target.value)} />
                </div>
              </div>
              <div className="flex items-center gap-5">
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <Switch checked={toggles.staging} onCheckedChange={(v:boolean)=>setToggles(t=>({...t, staging:v}))} /> Staging Mode
                </label>
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <Switch checked={toggles.autoPull} onCheckedChange={(v:boolean)=>setToggles(t=>({...t, autoPull:v}))} /> Auto pull latest
                </label>
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <Switch checked={toggles.applyOnChange} onCheckedChange={(v:boolean)=>setToggles(t=>({...t, applyOnChange:v}))} /> Apply on change
                </label>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="bg-slate-900/40 border-slate-800">
          <CardContent className="py-4 flex flex-wrap items-center gap-3 text-sm text-slate-300">
            <ShieldCheck className="h-4 w-4" /> Security by default:
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">AGE key never persisted</span>
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Decrypt to tmpfs only</span>
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
            <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
          </CardContent>
        </Card>

        <Tabs defaultValue="manifest" className="mt-2">
          <TabsList className="bg-slate-900/60 border border-slate-800">
            <TabsTrigger value="manifest">Manifest View</TabsTrigger>
            <TabsTrigger value="cards">Cards View</TabsTrigger>
          </TabsList>
          <TabsContent value="manifest" className="mt-4">
            <div className="text-slate-300 text-sm">Manifest table goes here.</div>
          </TabsContent>
          <TabsContent value="cards" className="mt-4">
            <div className="text-slate-300 text-sm">Cards grid goes here.</div>
          </TabsContent>
        </Tabs>
      </main>
    </div>
  );
}
