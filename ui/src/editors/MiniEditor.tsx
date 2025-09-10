import { useEffect, useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Shield, ShieldOff, Save, Maximize2, Minimize2 } from "lucide-react";
import Editor from "@monaco-editor/react";
import type { IacFileMeta } from "@/types";

export default function MiniEditor({
  id, initialPath, stackId, ensureStack, refresh, fileMeta,
}: {
  id: string;
  initialPath: string;
  stackId?: number;
  ensureStack: () => Promise<number>;
  refresh: () => void;
  fileMeta?: IacFileMeta;
}) {
  const [path, setPath] = useState(initialPath);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [decryptView, setDecryptView] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [fullscreen, setFullscreen] = useState(false);

  // derived: isSops based on meta or path hints or content marker
  const isSopsMeta = !!fileMeta?.sops;
  const isSopsPath = useMemo(() => {
    const low = path.toLowerCase();
    return low.includes("_secret.env") || low.includes("_private.env");
  }, [path]);
  const isSopsByContent = useMemo(() => {
    // quick heuristics: YAML with "sops:" block or dotenv with "ENC["
    const t = (content || "").toLowerCase();
    return /\nsops:/.test(t) || t.includes("enc[");
  }, [content]);

  const isSops = isSopsMeta || isSopsPath || isSopsByContent;

  useEffect(() => { setPath(initialPath); setContent(""); setErr(null); setDecryptView(false); }, [initialPath]);

  useEffect(() => {
    let cancel = false;
    (async () => {
      if (!stackId) return;
      // If user pressed "New ..." and we only prefilled a base directory (ends with /), skip fetch.
      if (path.endsWith("/")) { setContent(""); return; }
      setLoading(true); setErr(null);
      try {
        const url = `/api/iac/stacks/${stackId}/file?path=${encodeURIComponent(path)}`;
        const r = await fetch(url, { credentials: "include" });
        if (!r.ok) {
          if (r.status !== 404) throw new Error(`${r.status} ${r.statusText}`);
          setContent("");
        } else {
          const txt = await r.text();
          if (!cancel) setContent(txt);
        }
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    return () => { cancel = true; };
  }, [stackId, path]);

  async function saveFile(forceSops?: boolean) {
    if (path.endsWith("/")) { setErr("Please enter a full filename (not just a folder)."); return; }
    setLoading(true); setErr(null);
    try {
      const idToUse = stackId ?? await ensureStack();
      const r = await fetch(`/api/iac/stacks/${idToUse}/file`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path, content, sops: !!forceSops }),
      });
      if (!r.ok) {
        const txt = await r.text().catch(()=>"");
        throw new Error(txt || `${r.status} ${r.statusText}`);
      }
      refresh();
    } catch (e: any) {
      setErr(e?.message || "Failed to save");
    } finally {
      setLoading(false);
    }
  }

  // MiniEditor.revealSops()
  async function revealSops() {
    if (!stackId) { setErr("Create the stack by saving a file first."); return; }
    if (path.endsWith("/")) { setErr("Please enter a full filename first."); return; }
    setDecryptView(true);
    setLoading(true); setErr(null);
    try {
      const url = `/api/iac/stacks/${stackId}/file?path=${encodeURIComponent(path)}&decrypt=1`;
      const r = await fetch(url, {
        credentials: "include",
        headers: { "X-Confirm-Reveal": "yes" },
      });
      const txt = await r.text();
      if (!r.ok) {
        throw new Error(txt || `${r.status} ${r.statusText}`);
      }
      setContent(txt);
    } catch (e: any) {
      setErr(e?.message || "Failed to decrypt");
    } finally {
      setLoading(false);
    }
  }

  async function encryptSops() {
    if (!stackId) { setErr("Create the stack by saving a file first."); return; }
    if (path.endsWith("/")) { setErr("Please enter a full filename first."); return; }
    if (isSops) { setErr("File already appears to be SOPS-encrypted."); return; }
    if (!confirm("Encrypt this file with SOPS? This action cannot be undone locally.")) return;

    setLoading(true); setErr(null);
    try {
      await saveFile(true);
    } catch (e: any) {
      setErr(e?.message || "Failed to encrypt");
    } finally {
      setLoading(false);
    }
  }

  // pick monaco language by path
  const language = useMemo(() => {
    const low = path.toLowerCase();
    if (low.endsWith(".yaml") || low.endsWith(".yml")) return "yaml";
    if (low.endsWith(".json")) return "json";
    if (low.endsWith(".sh")) return "shell";
    if (low.includes(".env")) return "properties"; // close enough
    return "plaintext";
  }, [path]);

  const editor = (
    <Editor
      key={`${id}-${language}`}
      value={content}
      onChange={(v) => setContent(v ?? "")}
      language={language}
      theme="vs-dark"
      options={{
        readOnly: loading,
        wordWrap: "on",
        lineNumbers: "on",
        minimap: { enabled: false },
        scrollBeyondLastLine: false,
        fontFamily: "ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace",
        fontSize: 13,
        padding: { top: 8 },
      }}
      height="100%"
    />
  );

  const editorShell = (
    <div className="w-full flex-1 min-h-0 border border-slate-800 rounded overflow-hidden">
      <div className="h-full">{editor}</div>
    </div>
  );

  return (
    <Card className={`bg-slate-900/40 border-slate-800 h-full flex flex-col ${fullscreen ? "fixed inset-2 z-50" : ""}`}>
      <CardHeader className="pb-2 shrink-0">
        <CardTitle className="text-sm text-slate-200">Editor</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 min-h-0 flex flex-col gap-3">
        <div className="flex gap-2 shrink-0">
          <Input value={path} onChange={e => setPath(e.target.value)} placeholder="docker-compose/host/stack/compose.yaml" className="flex-1" />
          <div className="flex gap-1">
            {/* Toggle fullscreen */}
            <Button
              type="button"
              variant="outline"
              className="border-slate-700"
              onClick={() => setFullscreen(f => !f)}
              title={fullscreen ? "Exit full screen" : "Full screen editor"}
            >
              {fullscreen ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
            </Button>
            {/* SOPS actions: show if encrypted, else encrypt */}
            {isSops ? (
              <Button onClick={revealSops} variant="outline" className="border-indigo-700 text-indigo-200" title="Decrypt and reveal SOPS content" disabled={loading}>
                <Shield className="h-4 w-4 mr-1" /> Reveal SOPS
              </Button>
            ) : (
              <Button onClick={encryptSops} variant="outline" className="border-amber-700 text-amber-200 disabled:opacity-60" title="Encrypt this file with SOPS" disabled={loading || path.endsWith("/")}>
                <ShieldOff className="h-4 w-4 mr-1" /> Encrypt with SOPS
              </Button>
            )}
          </div>
        </div>
        {err && <div className="text-xs text-rose-300 shrink-0">Error: {err}</div>}
        {decryptView && <div className="text-xs text-amber-300 shrink-0">Warning: Decrypted secrets are visible in your browser until you navigate away.</div>}

        {/* Editor */}
        {editorShell}

        <div className="flex items-center justify-end shrink-0">
          <Button onClick={() => saveFile()} disabled={loading || path.endsWith("/")}>
            <Save className="h-4 w-4 mr-1" /> Save
          </Button>
        </div>
        <div className="text-xs text-slate-500 -mt-2">
          Files ending with <code>_private.env</code> or <code>_secret.env</code> will auto-encrypt with SOPS (if the server has a key).
        </div>
      </CardContent>
    </Card>
  );
}
