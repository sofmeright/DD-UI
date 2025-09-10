// ui/src/components/ConsoleModal.tsx
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Terminal as Term } from "xterm";
import { FitAddon } from "xterm-addon-fit";
import "xterm/css/xterm.css";

export default function ConsoleModal({
  host,
  container,
  onClose,
  cmd = "/bin/sh",
}: {
  host: string;
  container: string;
  onClose: () => void;
  cmd?: string;
}) {
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Term | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<"connecting" | "open" | "closed">("connecting");

  useEffect(() => {
    const term = new Term({
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      convertEol: true,
      cursorBlink: true,
      theme: { background: "#0b1220" },
      allowProposedApi: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    termRef.current = term;
    fitRef.current = fit;

    if (wrapRef.current) {
      term.open(wrapRef.current);
      // Give layout a tick before first fit
      setTimeout(() => fit.fit(), 10);
    }

    const proto = location.protocol === "https:" ? "wss" : "ws";
    const url = `${proto}://${location.host}/api/ws/hosts/${encodeURIComponent(
      host
    )}/containers/${encodeURIComponent(container)}/exec?cmd=${encodeURIComponent(cmd)}`;

    const ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus("open");
      term.focus();
    };
    ws.onclose = () => {
      setStatus("closed");
    };
    ws.onerror = () => {
      setStatus("closed");
    };
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        const dec = new TextDecoder();
        term.write(dec.decode(new Uint8Array(ev.data)));
      } else {
        term.write(String(ev.data));
      }
    };

    const onData = term.onData((d) => {
      try {
        ws.send(d);
      } catch {
        /* ignore */
      }
    });

    const onResize = () => {
      try {
        fit.fit();
        // optional: send a resize control message; server ignores if unsupported
        const dims = (term as any)._core?._renderService?.dimensions;
        const cols = term.cols;
        const rows = term.rows;
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: "resize", cols, rows }));
        }
      } catch {
        /* ignore */
      }
    };
    window.addEventListener("resize", onResize);
    const obs = new ResizeObserver(onResize);
    if (wrapRef.current) obs.observe(wrapRef.current);

    return () => {
      onData.dispose();
      window.removeEventListener("resize", onResize);
      obs.disconnect();
      try { ws.close(); } catch {}
      term.dispose();
    };
  }, [host, container, cmd]);

  return (
    <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4" onClick={onClose}>
      <div
        className="bg-slate-950 border border-slate-800 rounded-xl w-full max-w-5xl h-[70vh] p-3 flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-2">
          <div className="text-slate-200 font-semibold">
            Console: <span className="text-slate-400">{host}</span> / <span className="text-slate-400">{container}</span>
            <span className="ml-2 text-xs px-2 py-0.5 rounded bg-slate-800/60 border border-slate-700 text-slate-300">
              {status}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              className="border-slate-700"
              onClick={() => {
                // send Ctrl-C
                try { wsRef.current?.send("\x03"); } catch {}
              }}
            >
              Ctrl-C
            </Button>
            <Button size="sm" variant="outline" className="border-slate-700" onClick={onClose}>
              Close
            </Button>
          </div>
        </div>
        <div className="flex-1 rounded-md border border-slate-800 bg-[#0b1220] overflow-hidden">
          <div ref={wrapRef} className="w-full h-full" />
        </div>
        <div className="pt-2 text-xs text-slate-400">
          Tip: This attaches with a TTY to <code>{cmd}</code>. You can change the default command in the button handler if needed.
        </div>
      </div>
    </div>
  );
}
