// ui/src/components/HostPicker.tsx
import { Host } from "@/types";

export default function HostPicker({
  hosts, activeHost, setActiveHost,
}: { hosts: Host[]; activeHost: string; setActiveHost: (n: string)=>void }) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-slate-300">Host</span>
      <select
        className="bg-slate-950 border border-slate-800 text-slate-200 text-sm rounded px-2 py-1"
        value={activeHost}
        onChange={(e) => setActiveHost(e.target.value)}
      >
        {hosts.map(h => <option key={h.name} value={h.name}>{h.name}</option>)}
      </select>
    </div>
  );
}