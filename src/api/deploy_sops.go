// src/api/deploy_sops.go
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

/* ---------------- SOPS helpers ---------------- */

func hasSopsKeys() bool {
	ageKey := strings.TrimSpace(os.Getenv("SOPS_AGE_KEY"))
	ageKeyFile := strings.TrimSpace(os.Getenv("SOPS_AGE_KEY_FILE"))
	
	if ageKey != "" {
		debugLog("SOPS keys: SOPS_AGE_KEY is set (length: %d)", len(ageKey))
		return true
	}
	if ageKeyFile != "" {
		if st, err := os.Stat(ageKeyFile); err == nil && !st.IsDir() {
			debugLog("SOPS keys: SOPS_AGE_KEY_FILE exists at %s", ageKeyFile)
			return true
		} else {
			debugLog("SOPS keys: SOPS_AGE_KEY_FILE set to %s but file not found or is directory: %v", ageKeyFile, err)
		}
	}
	debugLog("SOPS keys: no SOPS keys found (SOPS_AGE_KEY empty, SOPS_AGE_KEY_FILE empty or missing)")
	return false
}

// Looks for SOPS markers to decide if we should even try to decrypt.
func looksSopsEncrypted(path, inputType string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		debugLog("SOPS detection: failed to read file %s: %v", path, err)
		return false
	}
	s := strings.ToLower(string(b))
	
	if inputType == "dotenv" {
		// SOPS dotenv adds metadata keys like: sops_mac, sops_version, sops_age__*, etc.
		hasMac := strings.Contains(s, "sops_mac=")
		hasVersion := strings.Contains(s, "sops_version=")
		hasAge := strings.Contains(s, "sops_age__")
		result := hasMac || hasVersion || hasAge
		debugLog("SOPS detection for dotenv %s: sops_mac=%v, sops_version=%v, sops_age=%v -> encrypted=%v", path, hasMac, hasVersion, hasAge, result)
		return result
	}
	// YAML/JSON compose: top-level "sops:" (yaml) or a "sops" object (json)
	hasSopsColon := strings.Contains(s, "\nsops:")
	hasSopsIndent := strings.Contains(s, "\n sops:")
	hasSopsJson := strings.Contains(s, `"sops"`)
	result := hasSopsColon || hasSopsIndent || hasSopsJson
	debugLog("SOPS detection for compose %s: \\nsops=%v, \\n sops=%v, \"sops\"=%v -> encrypted=%v", path, hasSopsColon, hasSopsIndent, hasSopsJson, result)
	if !result {
		// Show a sample of the content for debugging
		sample := string(b)
		if len(sample) > 200 {
			sample = sample[:200] + "..."
		}
		debugLog("SOPS detection: file %s content sample: %s", path, sample)
	}
	return result
}

// Read plaintext for file `full`. If SOPS can decrypt it, return decrypted bytes;
// if it's not encrypted, return the plain bytes. `inputType` is "" or "dotenv".
func readDecryptedOrPlain(ctx context.Context, full, inputType string) ([]byte, bool, error) {
	// If we don't have keys, or the file doesn't look SOPS-encrypted, read as plain.
	if !hasSopsKeys() || !looksSopsEncrypted(full, inputType) {
		b, err := os.ReadFile(full)
		return b, false, err
	}

	args := []string{"-d"}
	if inputType == "dotenv" {
		args = append(args, "--input-type", "dotenv")
	}
	args = append(args, full)

	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	out, err := exec.CommandContext(dctx, "sops", args...).CombinedOutput()
	if err == nil {
		return out, true, nil
	}

	// If sops complains about missing metadata, treat it as plaintext.
	low := strings.ToLower(string(out))
	if strings.Contains(low, "file is not encrypted") ||
		strings.Contains(low, "sops metadata not found") {
		b, rerr := os.ReadFile(full)
		return b, false, rerr
	}

	// Extra safeguard: even with a weird error, if the file doesn't actually
	// contain SOPS markers, treat as plain.
	if !looksSopsEncrypted(full, inputType) {
		b, rerr := os.ReadFile(full)
		return b, false, rerr
	}

	// Real decrypt error on an actually SOPS-encrypted file.
	return nil, false, fmt.Errorf("sops decrypt failed for %s: %v: %s", full, err, strings.TrimSpace(string(out)))
}

// Drop SOPS metadata keys from dotenv content and normalize "export KEY=..." to "KEY=...".
func filterDotenvSopsKeys(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			out = append(out, ln)
			continue
		}
		s := strings.TrimSpace(strings.TrimPrefix(t, "export "))
		eq := strings.IndexByte(s, '=')
		if eq <= 0 {
			out = append(out, ln)
			continue
		}
		key := strings.TrimSpace(s[:eq])
		// strip SOPS metadata lines (sops_age__..., sops_mac, sops_version, etc.)
		if strings.HasPrefix(strings.ToLower(key), "sops_") {
			continue
		}
		val := s[eq+1:]
		out = append(out, fmt.Sprintf("%s=%s", key, val))
	}
	return []byte(strings.Join(out, "\n"))
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:8])
}

/* ---------------- small fs helpers ---------------- */

func ensureDir(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	_ = os.Chmod(path, mode)
	return nil
}

// safe join under a root (prevents escaping with ..)
func joinUnderLocal(root, rel string) (string, error) {
	clean := filepath.Clean("/" + rel)
	clean = strings.TrimPrefix(clean, "/")
	full := filepath.Join(root, clean)
	r, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(r, "..") {
		return "", fmt.Errorf("outside root")
	}
	return full, nil
}

func writeFileSecure(dest string, content []byte, mode os.FileMode) error {
	if err := ensureDir(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dest, content, mode)
}

func copyRegularFile(src, dst string, mode os.FileMode) error {
	if err := ensureDir(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

/* --------- Compose helpers (parse env_file refs) --------- */

// parseEnvFileRefs extracts service->env_file refs (as string lists) from a compose YAML blob.
func parseEnvFileRefs(yml []byte) (map[string][]string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(yml, &doc); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	svcs, _ := doc["services"].(map[string]any)
	for name, raw := range svcs {
		m, _ := raw.(map[string]any)
		if m == nil {
			continue
		}
		var refs []string
		switch v := m["env_file"].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				refs = []string{v}
			}
		case []any:
			for _, it := range v {
				if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
					refs = append(refs, s)
				}
			}
		}
		if len(refs) > 0 {
			out[name] = refs
		}
	}
	return out, nil
}

/* -------- Public: scope-aware staging with SOPS auto-decrypt -------- */

// stageStackForCompose prepares a scope-aware staging directory that mirrors the IaC layout,
// copying compose/scripts/other files verbatim and materializing any env files decrypted with
// their original names/paths. It returns:
//   - stageStackDir: the directory compose should run in (mirrors the stack's rel_path)
//   - stagedComposes: absolute paths to compose files within the stage tree (pass with -f ...)
//   - cleanup: removes the staging directory
func stageStackForCompose(ctx context.Context, stackID int64) (stageStackDir string, stagedComposes []string, cleanup func(), err error) {
	// Discover stack root + identity
	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return "", nil, func() {}, err
	}

	var (
		rel        string
		scopeKind  string
		scopeName  string
		stackName  string
	)
	_ = db.QueryRow(ctx, `SELECT rel_path, scope_kind::text, scope_name, stack_name FROM iac_stacks WHERE id=$1`, stackID).
		Scan(&rel, &scopeKind, &scopeName, &stackName)
	if strings.TrimSpace(rel) == "" {
		return "", nil, func() {}, fmt.Errorf("deploy: stack has no rel_path")
	}

	// Stage base: DDUI_BUILDS_DIR or system temp
	base := strings.TrimSpace(os.Getenv("DDUI_BUILDS_DIR"))
	if base == "" {
		// a short-lived prefix under /tmp to make cleanups trivial
		tmp, terr := os.MkdirTemp("", "ddui-builds-")
		if terr != nil {
			return "", nil, func() {}, terr
		}
		base = tmp
	}
	if err := ensureDir(base, 0o700); err != nil {
		return "", nil, func() {}, err
	}

	// scope-aware leaf
	scopeDir := filepath.Join(base, strings.ToLower(scopeKind), scopeName, stackName)
	if err := ensureDir(scopeDir, 0o700); err != nil {
		return "", nil, func() {}, err
	}
	buildID := time.Now().UTC().Format("20060102-150405") + "-" + shortHash(fmt.Sprintf("%d", time.Now().UnixNano()))
	leaf := filepath.Join(scopeDir, buildID)
	if err := ensureDir(leaf, 0o700); err != nil {
		return "", nil, func() {}, err
	}

	// the working dir for compose (mirror original rel_path)
	stageStackDir, err = joinUnderLocal(leaf, rel)
	if err != nil {
		return "", nil, func() {}, err
	}
	if err := ensureDir(stageStackDir, 0o700); err != nil {
		return "", nil, func() {}, err
	}

	cleanup = func() { _ = os.RemoveAll(leaf) }

	// Gather tracked files
	rows, err := db.Query(ctx, `SELECT role, rel_path FROM iac_stack_files WHERE stack_id=$1`, stackID)
	if err != nil {
		return "", nil, cleanup, err
	}
	defer rows.Close()

	type rec struct{ role, relPath, srcAbs, dstAbs string }
	var (
		files          []rec
		composePairs   = map[string]string{} // staged compose -> source compose (for ref resolution)
	)
	for rows.Next() {
		var role, rp string
		if err := rows.Scan(&role, &rp); err != nil {
			return "", nil, cleanup, err
		}
		srcAbs, jerr := joinUnder(root, rp)
		if jerr != nil {
			return "", nil, cleanup, jerr
		}
		dstAbs, sj := joinUnderLocal(leaf, rp)
		if sj != nil {
			return "", nil, cleanup, sj
		}
		files = append(files, rec{role: strings.ToLower(role), relPath: rp, srcAbs: srcAbs, dstAbs: dstAbs})
	}

	// Copy files into stage:
	//  - compose/scripts/other: copy plaintext (if compose is SOPS-encrypted, decrypt to plaintext)
	//  - env: decrypt to plaintext and filter sops_* keys
	for _, f := range files {
		switch f.role {
		case "env":
			content, _, derr := readDecryptedOrPlain(ctx, f.srcAbs, "dotenv")
			if derr != nil {
				return "", nil, cleanup, derr
			}
			content = filterDotenvSopsKeys(content)
			if err := writeFileSecure(f.dstAbs, content, 0o600); err != nil {
				return "", nil, cleanup, err
			}
		case "compose":
			plain, _, perr := readDecryptedOrPlain(ctx, f.srcAbs, "")
			if perr != nil {
				return "", nil, cleanup, perr
			}
			if err := writeFileSecure(f.dstAbs, plain, 0o644); err != nil {
				return "", nil, cleanup, err
			}
			stagedComposes = append(stagedComposes, f.dstAbs)
			composePairs[f.dstAbs] = f.srcAbs
		default:
			// scripts/other auxiliary files
			if err := copyRegularFile(f.srcAbs, f.dstAbs, 0o644); err != nil {
				return "", nil, cleanup, err
			}
		}
	}

	// Ensure project .env (default interpolation) is staged & decrypted if present
	origStackDir, err := joinUnder(root, rel)
	if err != nil {
		return "", nil, cleanup, err
	}
	if b, err := os.ReadFile(filepath.Join(origStackDir, ".env")); err == nil && len(b) >= 0 {
		plain, _, derr := readDecryptedOrPlain(ctx, filepath.Join(origStackDir, ".env"), "dotenv")
		if derr == nil {
			plain = filterDotenvSopsKeys(plain)
			_ = writeFileSecure(filepath.Join(stageStackDir, ".env"), plain, 0o600)
		}
	}

	// Also stage service env_file refs that may not be tracked (common pattern).
	for stagedCompose, srcCompose := range composePairs {
		// Parse from plaintext we already wrote
		plain, rerr := os.ReadFile(stagedCompose)
		if rerr != nil {
			return "", nil, cleanup, rerr
		}
		refMap, perr := parseEnvFileRefs(plain)
		if perr != nil {
			return "", nil, cleanup, perr
		}
		if len(refMap) == 0 {
			continue
		}
		origBase := filepath.Dir(srcCompose)
		stageBase := filepath.Dir(stagedCompose)

		for _, refs := range refMap {
			for _, ref := range refs {
				if filepath.IsAbs(ref) {
					// Can't safely redirect absolute paths without editing compose; log and skip.
					// The original absolute path will be used by docker compose.
					warnLog("deploy: absolute env_file path %q in %s cannot be staged transparently", ref, srcCompose)
					continue
				}
				origEnv := filepath.Join(origBase, ref)
				stageEnv, sj := joinUnderLocal(stageBase, ref)
				if sj != nil {
					return "", nil, cleanup, sj
				}
				content, _, derr := readDecryptedOrPlain(ctx, origEnv, "dotenv")
				if derr != nil {
					// If missing, keep going (compose may handle it or it's optional)
					continue
				}
				content = filterDotenvSopsKeys(content)
				if err := writeFileSecure(stageEnv, content, 0o600); err != nil {
					return "", nil, cleanup, err
				}
			}
		}
	}

	return stageStackDir, stagedComposes, cleanup, nil
}
