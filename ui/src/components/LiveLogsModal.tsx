// ui/src/components/LiveLogsModal.tsx
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";

type Line = { stream: "stdout" | "stderr"; text: string };

export default function LiveLogsModal({
  host,
  container,
  onClose,
}: {
  host: string;
  container: string;
  onClose: () => void;
}) {
  const [lines, setLines] = useState<Line[]>([]);
  const [paused, setPaused] = useState(false);
  const boxRef = useRef<HTMLDivElement | null>(null);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (paused) return;

    const url = `/api/hosts/${encodeURIComponent(host)}/containers/${encodeURIComponent(
      container
    )}/logs/stream?tail=200`;
    const es = new EventSource(url, { withCredentials: true });
    esRef.current = es;

    const push = (stream: "stdout" | "stderr") => (ev: MessageEvent<string>) => {
      const text = ev.data ?? "";
      // chunk into individual lines if needed (SSE events are already line-based, but safe)
      const parts = text.split(/\r?\n/);
      setLines((prev) => {
        const next = prev.concat(parts.map((t) => ({ stream, text: t })));
        // keep last 2000 lines
        return next.length > 2000 ? next.slice(next.length - 2000) : next;
      });
      // auto-scroll
      if (boxRef.current) {
        boxRef.current.scrollTop = boxRef.current.scrollHeight;
      }
    };

    es.addEventListener("stdout", push("stdout"));
    es.addEventListener("stderr", push("stderr"));

    es.onerror = () => {
      // Network hiccup: allow EventSource to auto-retry; if it closes, user can close/reopen.
    };

    return () => {
      try { es.close(); } catch {}
    };
  }, [host, container, paused]);

  const copyAll = async () => {
    const txt = lines.map((l) => l.text).join("\n");
    try { await navigator.clipboard.writeText(txt); } catch {}
  };

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={onClose}>
      <div className="bg-slate-950 border border-slate-800 rounded-xl w-full max-w-5xl p-4" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-2">
          <div className="text-slate-200 font-semibold">
            Live Logs: <span className="text-slate-400">{host}</span> / <span className="text-slate-400">{container}</span>
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              className="border-slate-700"
              onClick={() => setPaused((p) => !p)}
            >
              {paused ? "Resume" : "Pause"}
            </Button>
            <Button size="sm" variant="outline" className="border-slate-700" onClick={copyAll}>
              Copy
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="border-slate-700"
              onClick={() => setLines([])}
            >
              Clear
            </Button>
            <Button size="sm" variant="outline" className="border-slate-700" onClick={onClose}>
              Close
            </Button>
          </div>
        </div>

        <div
          ref={boxRef}
          className="text-xs text-slate-200 bg-slate-900 border border-slate-800 rounded p-3 max-h-[70vh] h-[60vh] overflow-auto font-mono"
        >
          {lines.length === 0 && (
            <div className="text-slate-500">Waiting for log events…</div>
          )}
          {lines.map((l, i) => (
            <div key={i} className={l.stream === "stderr" ? "text-rose-300" : "text-slate-200"}>
              {l.text}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
