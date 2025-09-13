// ui/src/editors/MiniEditor.tsx
import { useEffect, useMemo, useRef, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Shield, ShieldOff, Eye, EyeOff, Save, Maximize2, Minimize2 } from "lucide-react";
import { Editor } from "@monaco-editor/react";
import type { IacFileMeta } from "@/types";
import { errorLog } from "@/utils/logging";

/**
 * MiniEditor with:
 * - Monaco editor (line numbers, language detection)
 * - SOPS view toggle (Encrypted <-> Decrypted)
 * - SOPS ON/OFF toggle for save behavior (re-encrypt after reveal, or save plaintext)
 * - Fullscreen toggle
 */
export default function MiniEditor({
  id,
  initialPath,
  hostName,
  stackName,
  ensureStack,
  refresh,
  fileMeta, // optional metadata for the active file (includes .sops flag)
}: {
  id: string;
  initialPath: string;
  hostName: string; // Required - hierarchical approach only
  stackName: string; // Required - hierarchical approach only
  ensureStack: () => Promise<number>;
  refresh: () => void;
  fileMeta?: IacFileMeta;
}) {
  const [path, setPath] = useState(initialPath);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);

  // Persisted SOPS flag for the file on disk (from server metadata).
  const [isSopsPersisted, setIsSopsPersisted] = useState<boolean>(!!fileMeta?.sops);

  // Whether we will encrypt on Save (user-controlled toggle; defaults based on metadata or filename).
  const [sopsOnSave, setSopsOnSave] = useState<boolean>(() => {
    if (fileMeta?.sops) return true;
    const p = initialPath.toLowerCase();
    // Sensible defaults for secret-ish names.
    return p.endsWith("_private.env") || p.endsWith("_secret.env");
  });

  // Current view mode: decrypted (shows secrets) vs raw (what's actually stored / or plain)
  // Decrypted only makes sense when sops is on (persisted or chosen for save) and the file exists.
  const [decryptedView, setDecryptedView] = useState<boolean>(false);

  // Fullscreen
  const [fullscreen, setFullscreen] = useState(false);
  const containerRef = useRef<HTMLDivElement | null>(null);

  // Keep local flags in sync when props change (path switched via file list)
  useEffect(() => {
    setPath(initialPath);
    setContent("");
    setDecryptedView(false);
    setIsSopsPersisted(!!fileMeta?.sops);

    // If metadata says it's SOPS, default sopsOnSave to true; else keep name-based heuristic.
    if (fileMeta?.sops) {
      setSopsOnSave(true);
    } else {
      const p = initialPath.toLowerCase();
      setSopsOnSave(p.endsWith("_private.env") || p.endsWith("_secret.env"));
    }
  }, [initialPath, fileMeta]);

  // Helper: choose Monaco language by file path
  const language = useMemo(() => {
    const p = path.toLowerCase();
    if (p.endsWith(".yml") || p.endsWith(".yaml")) return "yaml";
    if (p.endsWith(".json")) return "json";
    if (p.endsWith("dockerfile") || p.endsWith("/dockerfile")) return "dockerfile";
    if (p.endsWith(".sh")) return "shell";
    if (p.endsWith(".env") || p.includes(".env")) return "ini"; // closest for dotenv
    return "plaintext";
  }, [path]);

  // Fetch file (optionally decrypted) - PURE HIERARCHICAL ONLY
  async function loadFile(opts?: { decrypt?: boolean }) {
    setLoading(true);
    try {
      const params = new URLSearchParams({ path });
      if (opts?.decrypt) params.set("decrypt", "1");

      const r = await fetch(`/api/iac/hosts/${encodeURIComponent(hostName)}/stacks/${encodeURIComponent(stackName)}/file?${params.toString()}`, {
        credentials: "include",
        headers: opts?.decrypt ? { "X-Confirm-Reveal": "yes" } : undefined,
      });

      if (r.status === 404) {
        setContent("");
        return;
      }
      const txt = await r.text();
      if (!r.ok) throw new Error(txt || `${r.status} ${r.statusText}`);
      setContent(txt);
    } catch (e) {
      // Keep failure soft to avoid interrupting editing flow
      setContent((prev) => prev); // no-op, preserve any local edits
      errorLog("Editor load failed:", e);
    } finally {
      setLoading(false);
    }
  }

  // Initial and whenever path/view changes
  useEffect(() => {
    loadFile({ decrypt: decryptedView && (isSopsPersisted || sopsOnSave) });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hostName, stackName, path, decryptedView]);

  // Save file with current sopsOnSave preference - PURE HIERARCHICAL ONLY
  async function saveFile() {
    setLoading(true);
    try {
      // If the file is currently SOPS on disk, but the user turned SOPS OFF for save,
      // and they're NOT in decrypted view, fetch decrypted content so we save plaintext.
      let bodyContent = content;
      if (!sopsOnSave && isSopsPersisted && !decryptedView) {
        try {
          const params = new URLSearchParams({ path, decrypt: "1" });
          const rDec = await fetch(`/api/iac/hosts/${encodeURIComponent(hostName)}/stacks/${encodeURIComponent(stackName)}/file?${params.toString()}`, {
            credentials: "include",
            headers: { "X-Confirm-Reveal": "yes" },
          });
          const txtDec = await rDec.text();
          if (!rDec.ok) throw new Error(txtDec || `${rDec.status} ${rDec.statusText}`);
          bodyContent = txtDec; // use decrypted plaintext for saving
        } catch (e) {
          // If decrypt failed, warn and proceed with editor content to avoid data loss.
          alert("Couldn't load decrypted content; saving the current editor text instead.");
        }
      }

      const resp = await fetch(`/api/iac/hosts/${encodeURIComponent(hostName)}/stacks/${encodeURIComponent(stackName)}/file`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path, content: bodyContent, sops: sopsOnSave }),
      });

      const txt = await resp.text();
      if (!resp.ok) throw new Error(txt || `${resp.status} ${resp.statusText}`);

      // The API returns { sops: boolean, ... } – update flags if we can parse it
      try {
        const j = JSON.parse(txt);
        if (typeof j?.sops === "boolean") {
          setIsSopsPersisted(j.sops);
          if (!j.sops && decryptedView) {
            // No longer meaningful to keep "decrypted" view when file isn't SOPS on disk.
            setDecryptedView(false);
          }
        }
      } catch {
        // ignore parse issues; refresh will sync state
      }

      // Ask parent to refresh listing & close editor if it wants to
      refresh();
    } catch (e) {
      alert((e as Error)?.message || "Failed to save");
    } finally {
      setLoading(false);
    }
  }

  // Toggle decrypted/raw view. When turning on, refetch with decrypt=1.
  function toggleDecryptedView() {
    // Only meaningful if SOPS is on (persisted on disk or chosen to be on for save)
    if (!(isSopsPersisted || sopsOnSave)) {
      return;
    }
    setDecryptedView((v) => !v);
  }

  // Toggle encryption for future saves (also allows going back and forth).
  function toggleSopsOnSave() {
    setSopsOnSave((v) => !v);
    // If we turn SOPS OFF while viewing decrypted, it's fine; future saves will store plaintext.
  }

  // Fullscreen helpers
  function toggleFullscreen() {
    setFullscreen((f) => !f);
  }

  return (
    <Card
      ref={containerRef}
      className={
        (fullscreen ? "fixed inset-0 z-50 m-0 rounded-none" : "h-full") +
        " bg-slate-900/40 border-slate-800 flex flex-col"
      }
    >
      <CardHeader className="pb-2 shrink-0">
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="text-sm text-slate-200">Editor</CardTitle>
          <div className="flex items-center gap-1">
            <Button
              size="sm"
              variant="ghost"
              className="h-8 px-2"
              onClick={toggleSopsOnSave}
              title={sopsOnSave ? "SOPS: On (save encrypted)" : "SOPS: Off (save plaintext)"}
            >
              {sopsOnSave ? <Shield className="h-4 w-4 mr-1" /> : <ShieldOff className="h-4 w-4 mr-1" />}
              {sopsOnSave ? "SOPS On" : "SOPS Off"}
            </Button>

            {(isSopsPersisted || sopsOnSave) && (
              <Button
                size="sm"
                variant="outline"
                className="h-8 px-2 border-indigo-700 text-indigo-200"
                onClick={toggleDecryptedView}
                disabled={loading}
                title={decryptedView ? "Hide decrypted view" : "Show decrypted view"}
              >
                {decryptedView ? <EyeOff className="h-4 w-4 mr-1" /> : <Eye className="h-4 w-4 mr-1" />}
                {decryptedView ? "Hide" : "Show"}
              </Button>
            )}

            <Button
              size="sm"
              variant="ghost"
              className="h-8 px-2"
              onClick={toggleFullscreen}
              title={fullscreen ? "Exit full screen" : "Full screen"}
            >
              {fullscreen ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
            </Button>
          </div>
        </div>
      </CardHeader>

      <CardContent className="flex-1 min-h-0 flex flex-col gap-3">
        <div className="flex gap-2 shrink-0">
          <Input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="docker-compose/host/stack/docker-compose.yaml"
            className="flex-1"
          />
        </div>

        <div className="flex-1 min-h-0 rounded border border-slate-800 overflow-hidden">
          <Editor
            key={`${id}:${path}`} // remount when path changes to reset undo stack
            value={content}
            onChange={(val) => setContent(val ?? "")}
            language={language}
            theme="vs-dark"
            options={{
              lineNumbers: "on",
              wordWrap: "on",
              minimap: { enabled: false },
              fontSize: 13,
              scrollBeyondLastLine: false,
              tabSize: 2,
              renderWhitespace: "selection",
              readOnly: loading,
            }}
            height="100%"
          />
        </div>

        <div className="flex items-center justify-end gap-2 shrink-0">
          <Button onClick={saveFile} disabled={loading}>
            <Save className="h-4 w-4 mr-1" /> Save
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
