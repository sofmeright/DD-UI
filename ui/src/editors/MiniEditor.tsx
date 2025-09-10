// ui/src/editors/MiniEditor.tsx
import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Shield, ShieldOff, Save } from "lucide-react";

export default function MiniEditor({
  id, initialPath, stackId, ensureStack, refresh,
}: {
  id: string;
  initialPath: string;
  stackId?: number;
  ensureStack: () => Promise<number>;
  refresh: () => void;
}) {
  const [path, setPath] = useState(initialPath);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [sops, setSops] = useState(false);
  const [decryptView, setDecryptView] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => { setPath(initialPath); setContent(""); setErr(null); setDecryptView(false); }, [initialPath]);

  useEffect(() => {
    let cancel = false;
    (async () => {
      if (!stackId) return;
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
    setLoading(true); setErr(null);
    try {
      const idToUse = stackId ?? await ensureStack();
      const sopsFlag = forceSops ?? sops;
      const r = await fetch(`/api/iac/stacks/${idToUse}/file`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path, content, sops: sopsFlag }),
      });
      if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
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
    if (sops) { setErr("File is already marked as SOPS."); return; }
    if (!confirm("Encrypt this file with SOPS? This action cannot be undone locally.")) return;

    setLoading(true); setErr(null);
    try {
      setSops(true);
      await saveFile(true);
    } catch (e: any) {
      setSops(false);
      setErr(e?.message || "Failed to encrypt");
    } finally {
      setLoading(false);
    }
  }

  const isSopsFile =
    path.toLowerCase().includes('_secret.env') ||
    path.toLowerCase().includes('_private.env') ||
    sops;

  return (
    <Card className="bg-slate-900/40 border-slate-800 h-full flex flex-col">
      <CardHeader className="pb-2 shrink-0">
        <CardTitle className="text-sm text-slate-200">Editor</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 min-h-0 flex flex-col gap-3">
        <div className="flex gap-2 shrink-0">
          <Input value={path} onChange={e => setPath(e.target.value)} placeholder="docker-compose/host/stack/compose.yaml" className="flex-1" />
          <div className="flex gap-1">
            {isSopsFile && (
              <Button onClick={revealSops} variant="outline" className="border-indigo-700 text-indigo-200" title="Decrypt and reveal SOPS content" disabled={loading}>
                <Shield className="h-4 w-4 mr-1" />SOPS Reveal
              </Button>
            )}
            {!sops && !isSopsFile && (
              <Button onClick={encryptSops} variant="outline" className="border-amber-700 text-amber-200 disabled:opacity-60" title="Encrypt this file with SOPS" disabled={loading}>
                <ShieldOff className="h-4 w-4 mr-1" />SOPS Encrypt
              </Button>
            )}
          </div>
        </div>
        {err && <div className="text-xs text-rose-300 shrink-0">Error: {err}</div>}
        {decryptView && <div className="text-xs text-amber-300 shrink-0">Warning: Decrypted secrets are visible in your browser until you navigate away.</div>}
        <textarea
          id={id}
          className="w-full flex-1 min-h-0 text-sm bg-slate-950/50 border border-slate-800 rounded p-2 font-mono text-slate-200"
          value={content}
          onChange={e => setContent(e.target.value)}
          placeholder={loading ? "Loading…" : "File content…"}
          disabled={loading}
        />
        <div className="flex items-center justify-between shrink-0">
          <label className="text-sm text-slate-300 inline-flex items-center gap-2">
            <input type="checkbox" checked={sops} onChange={e => setSops(e.target.checked)} />
            Mark as SOPS file
          </label>
          <Button onClick={() => saveFile()} disabled={loading}><Save className="h-4 w-4 mr-1" /> Save</Button>
        </div>
        <div className="text-xs text-slate-500 -mt-2">
          Files ending with <code>_private.env</code> or <code>_secret.env</code> will auto-encrypt with SOPS (if the server has a key).
        </div>
      </CardContent>
    </Card>
  );
}
