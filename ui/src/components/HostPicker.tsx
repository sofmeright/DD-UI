// ui/src/components/HostPicker.tsx
import { Host } from "@/types";
import { Card } from "@/components/ui/card";

export default function HostPicker({
  hosts, activeHost, setActiveHost,
}: { hosts: Host[]; activeHost: string; setActiveHost: (n: string)=>void }) {
  const currentHost = hosts.find(h => h.name === activeHost);
  
  return (
    <Card className="bg-slate-800/60 border-slate-700 px-3 py-1.5">
      <div className="flex items-center gap-3">
        <select
          className="bg-slate-900/70 border border-slate-600 text-slate-200 text-sm rounded px-2 py-1 min-w-[160px] focus:outline-none focus:ring-2 focus:ring-slate-500 focus:border-slate-500"
          value={activeHost}
          onChange={(e) => setActiveHost(e.target.value)}
        >
          {hosts.map(h => (
            <option key={h.name} value={h.name} className="bg-slate-900 text-slate-200">
              {h.name}
            </option>
          ))}
        </select>
        {currentHost?.address && (
          <div className="text-slate-300 text-sm font-mono">{currentHost.address}</div>
        )}
      </div>
    </Card>
  );
}