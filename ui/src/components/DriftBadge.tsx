// ui/src/components/DriftBadge.tsx
import { Badge } from "@/components/ui/badge";

export default function DriftBadge(d: "in_sync" | "drift" | "unknown") {
  if (d === "in_sync") return <Badge className="bg-emerald-900/40 border-emerald-700/40 text-emerald-200">In sync</Badge>;
  if (d === "drift") return <Badge variant="destructive">Drift</Badge>;
  return <Badge variant="outline" className="border-slate-700 text-slate-300">Unknown</Badge>;
}