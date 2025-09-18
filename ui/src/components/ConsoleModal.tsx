// ui/src/components/ConsoleModal.tsx
import { useEffect, useRef, useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Terminal as Term } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

type Shell = "auto" | "bash" | "ash" | "dash" | "sh";

export default function ConsoleModal({
  host,
  container,
  onClose,
  defaultShell = "auto",
}: {
  host: string;
  container: string;
  onClose: () => void;
  defaultShell?: Shell;
}) {
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Term | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  const [status, setStatus] = useState<"connecting" | "open" | "closed">("connecting");
  const [shell, setShell] = useState<Shell>(defaultShell);
  const [connBump, setConnBump] = useState(0); // change to force reconnect

  const connect = useCallback(() => {
    setStatus("connecting");

    const proto = location.protocol === "https:" ? "wss" : "ws";
    const qs = new URLSearchParams();
    qs.set("shell", shell);
    const url = `${proto}://${location.host}/api/ws/hosts/${encodeURIComponent(
      host
    )}/containers/${encodeURIComponent(container)}/exec?${qs.toString()}`;

    const term = (termRef.current ||= new Term({
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      convertEol: true,
      cursorBlink: true,
      theme: { background: "#0b1220" },
      allowProposedApi: true,
      scrollback: 4000,
    }));

    let fit = fitRef.current;
    if (!fit) {
      fit = new FitAddon();
      term.loadAddon(fit);
      fitRef.current = fit;
    }

    if (wrapRef.current && (term as any)._core?.screenElement == null) {
      term.open(wrapRef.current);
      // allow layout to settle then fit
      setTimeout(() => fit!.fit(), 10);
    }

    const ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus("open");
      // fit and send initial resize
      try {
        fit!.fit();
        ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
      } catch {}
      term.focus();
      term.writeln(`\x1b[38;5;67m[connected]\x1b[0m  shell=${shell}`);
    };

    ws.onclose = (event) => {
      setStatus("closed");
      // Check if it's an auth error (1008 is policy violation, commonly used for auth failures)
      if (event.code === 1008 || event.reason?.includes('401') || event.reason?.includes('unauthorized')) {
        term.writeln(`\r\n\x1b[38;5;203m[authentication failed - redirecting to login]\x1b[0m`);
        setTimeout(() => window.location.replace('/auth/login'), 1500);
      } else {
        term.writeln(`\r\n\x1b[38;5;203m[disconnected]\x1b[0m`);
      }
    };
    ws.onerror = () => {
      setStatus("closed");
      term.writeln(`\r\n\x1b[38;5;203m[error]\x1b[0m`);
    };

    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        const dec = new TextDecoder();
        term.write(dec.decode(new Uint8Array(ev.data)));
      } else {
        term.write(String(ev.data));
      }
    };

    const sub = term.onData((d) => {
      try {
        ws.send(d);
      } catch {}
    });

    const onResize = () => {
      try {
        fit!.fit();
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
        }
      } catch {}
    };
    window.addEventListener("resize", onResize);
    const obs = new ResizeObserver(onResize);
    if (wrapRef.current) obs.observe(wrapRef.current);

    return () => {
      sub.dispose();
      window.removeEventListener("resize", onResize);
      obs.disconnect();
      try {
        ws.close();
      } catch {}
    };
  }, [host, container, shell, connBump]);

  // (Re)connect lifecycle
  useEffect(() => {
    return connect();
  }, [connect]);

  // Build terminal on first mount
  useEffect(() => {
    const term = (termRef.current ||= new Term());
    return () => {
      try {
        term.dispose();
      } catch {}
      termRef.current = null;
      fitRef.current = null;
      wsRef.current = null;
    };
  }, []);

  return (
    <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4" onClick={onClose}>
      <div
        className="bg-slate-950 border border-slate-800 rounded-xl w-full max-w-5xl h-[70vh] p-3 flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-2">
          <div className="text-slate-200 font-semibold">
            Console: <span className="text-slate-400">{host}</span> /{" "}
            <span className="text-slate-400">{container}</span>
            <span className="ml-2 text-xs px-2 py-0.5 rounded bg-slate-800/60 border border-slate-700 text-slate-300">
              {status}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <label className="text-xs text-slate-300">Shell</label>
            <select
              className="text-xs bg-slate-900 border border-slate-700 rounded px-2 py-1 text-slate-200"
              value={shell}
              onChange={(e) => setShell(e.target.value as Shell)}
              title="Pick a shell; Auto tries bash → ash → dash → sh"
            >
              <option value="auto">Auto</option>
              <option value="bash">bash</option>
              <option value="ash">ash</option>
              <option value="dash">dash</option>
              <option value="sh">sh</option>
            </select>

            <Button
              size="sm"
              variant="outline"
              className="border-slate-700"
              onClick={() => setConnBump((n) => n + 1)}
              title="Reconnect with current shell"
            >
              Reconnect
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="border-slate-700"
              onClick={() => {
                try {
                  wsRef.current?.send("\x03"); // Ctrl-C
                } catch {}
              }}
            >
              Ctrl-C
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="border-slate-700"
              onClick={() => {
                try {
                  wsRef.current?.send("\x04"); // Ctrl-D (EOF)
                } catch {}
              }}
            >
              Ctrl-D
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
          Tip: Auto tries <code>bash</code> → <code>ash</code> → <code>dash</code> → <code>sh</code>.
          Resize is wired so the terminal fits your window.
        </div>
      </div>
    </div>
  );
}
