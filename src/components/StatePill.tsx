// ui/src/components/StatePill.tsx
export default function StatePill({ state, health }: { state?: string; health?: string }) {
    const s = (state || "").toLowerCase();
    const h = (health || "").toLowerCase();
    let classes = "border-slate-700 bg-slate-900 text-slate-300";
    let text = state || "unknown";
    if (h === "healthy") {
      classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200";
      text = "healthy";
    } else if (s.includes("running") || s.includes("up")) {
      classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200";
    } else if (s.includes("restarting")) {
      classes = "border-amber-700/60 bg-amber-900/40 text-amber-200";
    } else if (s.includes("paused")) {
      classes = "border-sky-700/60 bg-sky-900/40 text-sky-200";
    } else if (s.includes("exited") || s.includes("dead")) {
      classes = "border-rose-700/60 bg-rose-900/40 text-rose-200";
    }
    return (
      <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border ${classes}`}>
        {text}
      </span>
    );
  }