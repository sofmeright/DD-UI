// ui/src/components/LiveLogsModal.tsx
import { useEffect, useRef, useState, useCallback } from "react";
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
  const [autoScroll, setAutoScroll] = useState(true); // default ON
  const boxRef = useRef<HTMLDivElement | null>(null);
  const esRef = useRef<EventSource | null>(null);

  const scrollToBottom = useCallback(() => {
    if (!boxRef.current) return;
    boxRef.current.scrollTop = boxRef.current.scrollHeight;
  }, []);

  // Toggle autoScroll OFF if user manually scrolls up
  useEffect(() => {
    const el = boxRef.current;
    if (!el) return;

    const onScroll = () => {
      if (!el) return;
      const nearBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 40;
      if (autoScroll !== nearBottom) {
        // If user moved away from bottom, disable autoScroll; if they return, re-enable
        setAutoScroll(nearBottom);
      }
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [autoScroll]);

  useEffect(() => {
    if (paused) {
      // ensure any existing stream is closed
      try { esRef.current?.close(); } catch {}
      esRef.current = null;
      return;
    }

    const url = `/api/containers/hosts/${encodeURIComponent(host)}/${encodeURIComponent(
      container
    )}/logs/stream?tail=200`;
    const es = new EventSource(url, { withCredentials: true });
    esRef.current = es;

    const push = (stream: "stdout" | "stderr") => (ev: MessageEvent<string>) => {
      const text = ev.data ?? "";
      const parts = text.split(/\r?\n/);
      setLines((prev) => {
        const next = prev.concat(parts.map((t) => ({ stream, text: t })));
        // keep last 2000 lines
        return next.length > 2000 ? next.slice(next.length - 2000) : next;
      });
      if (autoScroll) {
        // let DOM paint the new nodes, then snap to bottom
        requestAnimationFrame(scrollToBottom);
      }
    };

    es.addEventListener("stdout", push("stdout"));
    es.addEventListener("stderr", push("stderr"));

    es.onerror = () => {
      // Let EventSource auto-retry; if the server closes, user can reopen.
    };

    return () => {
      try { es.close(); } catch {}
    };
  }, [host, container, paused, autoScroll, scrollToBottom]);

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
            <span className="ml-2 text-xs px-2 py-0.5 rounded bg-slate-800/60 border border-slate-700 text-slate-300">
              {paused ? "paused" : "streaming"}
            </span>
            <span className="ml-2 text-xs px-2 py-0.5 rounded bg-slate-800/60 border border-slate-700 text-slate-300">
              {autoScroll ? "auto-scroll" : "free scroll"}
            </span>
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
            <Button
              size="sm"
              variant="outline"
              className="border-slate-700"
              onClick={() => setAutoScroll((v) => !v)}
              title="Toggle following the newest lines"
            >
              {autoScroll ? "Free scroll" : "Follow newest"}
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
