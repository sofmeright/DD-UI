// ui/src/views/HostsView.tsx  
import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import SearchBar from "@/components/SearchBar";
import GitSyncToggle from "@/components/GitSyncToggle";
import DevOpsToggle from "@/components/DevOpsToggle";
import AddHostDialog from "@/components/AddHostDialog";
import { Boxes, Layers, AlertTriangle, XCircle, RefreshCw, Server, Plus, Edit2 } from "lucide-react";
import { Host } from "@/types";
import { handle401 } from "@/utils/auth";

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
  onOpenHost: (name: string) => void;
  refreshMetricsForHosts: (names: string[]) => Promise<void>;
}) {
  const [query, setQuery] = useState("");
  const [showAddHostDialog, setShowAddHostDialog] = useState(false);
  const [hostToEdit, setHostToEdit] = useState<Host | null>(null);

  return (
    <div className="space-y-3">
      {/* Action bar matching other pages */}
      <div className="flex items-center gap-4">
        <div className="text-lg font-semibold text-white">Hosts</div>
        
        {/* Small square metric cards matching host picker size */}
        <div className="flex items-center gap-2">
          <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
            <Server className="h-4 w-4 text-slate-400" />
            <span className="text-sm text-slate-300">{metrics.hosts}</span>
          </div>
          <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
            <Layers className="h-4 w-4 text-slate-400" />
            <span className="text-sm text-slate-300">{metrics.stacks}</span>
          </div>
          <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
            <Boxes className="h-4 w-4 text-slate-400" />
            <span className="text-sm text-slate-300">{metrics.containers}</span>
          </div>
          <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-amber-400" />
            <span className="text-sm text-amber-400">{metrics.drift}</span>
          </div>
          <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
            <XCircle className="h-4 w-4 text-rose-400" />
            <span className="text-sm text-rose-400">{metrics.errors}</span>
          </div>
        </div>
        
        <SearchBar 
          value={query}
          onChange={(value) => { setQuery(value); onFilter(value); }}
          placeholder="Search hosts, groups, addresses..."
          className="w-96"
        />
        
        <Button onClick={onScanAll} disabled={scanning} className="bg-[#310937] hover:bg-[#2a0830] text-white">
          <RefreshCw className={`h-4 w-4 mr-1 ${scanning ? "animate-spin" : ""}`} />
          {scanning ? "Scanning…" : "Sync"}
        </Button>

        <Button 
          onClick={() => {
            setHostToEdit(null);  // Clear edit state for new host
            setShowAddHostDialog(true);
          }}
          className="bg-emerald-700 hover:bg-emerald-600 text-white"
        >
          <Plus className="h-4 w-4 mr-1" />
          Add Host
        </Button>

        {/* Toggles positioned at the end */}
        <div className="ml-auto flex items-center gap-3">
          <DevOpsToggle level="global" />
          <GitSyncToggle />
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-800">
        <table className="w-full text-sm">
          <thead className="bg-slate-900/70 text-slate-300">
            <tr>
              <th className="p-3 text-left">Host</th>
              <th className="p-3 text-left">Address</th>
              <th className="p-3 text-left">Groups</th>
              <th className="p-3 text-center">Scan</th>
              <th className="p-3 text-center">Status</th>
              <th className="p-3 text-center">Edit</th>
              <th className="p-3 text-center">DevOps</th>
            </tr>
          </thead>
          <tbody>
            {loading && (<tr><td className="p-4 text-slate-500" colSpan={7}>Loading hosts…</td></tr>)}
            {err && !loading && (<tr><td className="p-4 text-rose-300" colSpan={7}>{err}</td></tr>)}
            {!loading && filteredHosts.map((h) => (
              <tr key={h.name} className="border-t border-slate-800 hover:bg-slate-900/40">
                <td className="p-3 font-medium text-slate-200">
                  <button className="hover:underline" onClick={() => onOpenHost(h.name)}>{h.name}</button>
                </td>
                <td className="p-3 text-slate-300">{h.address || "—"}</td>
                <td className="p-3 text-slate-300">{(h.groups || []).length ? (h.groups || []).join(", ") : "—"}</td>
                <td className="p-3 text-center">
                  <Button
                    size="sm"
                    variant="outline"
                    className="border-slate-700 text-slate-200 hover:bg-slate-800"
                    onClick={async () => {
                      const r = await fetch(`/api/scan/host/${encodeURIComponent(h.name)}`, { method: "POST", credentials: "include" });
                      if (r.status === 401) {
                        handle401();
                        return;
                      }
                      await refreshMetricsForHosts([h.name]);
                    }}
                  >
                    <RefreshCw className="h-4 w-4 mr-1" />
                    Scan
                  </Button>
                </td>
                <td className="p-3 text-center">—</td>
                <td className="p-3 text-center">
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-slate-400 hover:text-slate-200 hover:bg-slate-800"
                    onClick={() => {
                      setHostToEdit(h);
                      setShowAddHostDialog(true);
                    }}
                  >
                    <Edit2 className="h-4 w-4" />
                  </Button>
                </td>
                <td className="p-3 text-center">
                  <DevOpsToggle 
                    level="host" 
                    hostName={h.name}
                    className="justify-center"
                  />
                </td>
              </tr>
            ))}
            {!loading && filteredHosts.length === 0 && (
              <tr><td className="p-6 text-center text-slate-500" colSpan={7}>No hosts.</td></tr>
            )}
          </tbody>
        </table>
      </div>

      <AddHostDialog
        open={showAddHostDialog}
        onClose={() => {
          setShowAddHostDialog(false);
          setHostToEdit(null);
        }}
        onHostAdded={() => {
          setShowAddHostDialog(false);
          setHostToEdit(null);
          // Trigger a refresh of the hosts list
          window.location.reload();
        }}
        hostToEdit={hostToEdit}
        isEditMode={!!hostToEdit}
      />
    </div>
  );
}