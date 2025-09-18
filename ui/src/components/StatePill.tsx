// ui/src/components/StatePill.tsx
import { debugLog } from "@/utils/logging";

export default function StatePill({ state, status, health }: { state?: string; status?: string; health?: string }) {
    const s = (state || "").toLowerCase();
    const st = (status || "").toLowerCase();
    const h = (health || "").toLowerCase();
    
    // Debug logging to understand what values we're getting
    if (s.includes("paused") || st.includes("paused")) {
        debugLog(`StatePill PAUSED: state="${state}", status="${status}", s="${s}", st="${st}"`);
    }
    
    let classes = "border-slate-700 bg-slate-900 text-slate-300";
    let text = state || "unknown";
    let displayText = text;
    
    // Priority: health status > detailed status > state
    if (h === "healthy") {
        classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200";
        displayText = "healthy";
    } else if (h === "unhealthy") {
        classes = "border-rose-700/60 bg-rose-900/40 text-rose-200";
        displayText = "unhealthy";
    } else if (h === "starting") {
        classes = "border-amber-700/60 bg-amber-900/40 text-amber-200 animate-pulse";
        displayText = "starting";
    } else if (st.includes("starting")) {
        classes = "border-amber-700/60 bg-amber-900/40 text-amber-200 animate-pulse";
        displayText = "starting";
    } else if (s.includes("paused") || st.includes("paused")) {
        classes = "border-sky-700/60 bg-sky-900/40 text-sky-200";
        displayText = "paused";
    } else if (s.includes("restarting")) {
        classes = "border-amber-700/60 bg-amber-900/40 text-amber-200 animate-pulse";
        displayText = "restarting";
    } else if (st.includes("up") || s.includes("running")) {
        classes = "border-emerald-700/60 bg-emerald-900/40 text-emerald-200";
        displayText = "running";
    } else if (st.includes("exited") || s.includes("exited")) {
        classes = "border-rose-700/60 bg-rose-900/40 text-rose-200";
        // Extract exit code if available (e.g., "Exited (0) 2 minutes ago")
        const exitMatch = st.match(/exited\s*\((\d+)\)/i);
        if (exitMatch && exitMatch[1]) {
            const code = exitMatch[1];
            displayText = code === "0" ? "exited" : `exited (error: ${code})`;
        } else {
            displayText = "exited";
        }
    } else if (s.includes("dead")) {
        classes = "border-rose-700/60 bg-rose-900/40 text-rose-200";
        displayText = "dead";
    } else if (s.includes("created")) {
        classes = "border-slate-700/60 bg-slate-800/40 text-slate-300";
        displayText = "created";
    } else if (s.includes("removing")) {
        classes = "border-orange-700/60 bg-orange-900/40 text-orange-200 animate-pulse";
        displayText = "removing";
    }
    
    return (
      <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border ${classes}`}>
        {displayText}
      </span>
    );
  }