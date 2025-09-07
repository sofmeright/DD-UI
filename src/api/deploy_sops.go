package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

/* ---------------- SOPS helpers ---------------- */

func hasSopsKeys() bool {
	if strings.TrimSpace(os.Getenv("SOPS_AGE_KEY")) != "" {
		return true
	}
	if fp := strings.TrimSpace(os.Getenv("SOPS_AGE_KEY_FILE")); fp != "" {
		if st, err := os.Stat(fp); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// Read plaintext for file `full`. If SOPS can decrypt it, return decrypted bytes;
// if it's not encrypted, return the plain bytes. `inputType` is "" or "dotenv".
func readDecryptedOrPlain(ctx context.Context, full, inputType string) ([]byte, bool, error) {
	tryDecrypt := hasSopsKeys()
	if tryDecrypt {
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
		low := strings.ToLower(string(out))
		if strings.Contains(low, "file is not encrypted") || strings.Contains(low, "sops metadata not found") {
			// fall through to plain read
		} else {
			return nil, false, fmt.Errorf("sops decrypt failed for %s: %v: %s", full, err, strings.TrimSpace(string(out)))
		}
	}
	b, rerr := os.ReadFile(full)
	return b, false, rerr
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
			// not a KEY=VAL line; keep as-is
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

func materializeFile(workspace, srcPath string, content []byte) (abs string, err error) {
	name := fmt.Sprintf("%s-%s", shortHash(srcPath), filepath.Base(srcPath))
	abs = filepath.Join(workspace, name)
	return abs, os.WriteFile(abs, content, 0o600)
}

/* --------- Compose overlay (no edits to original) --------- */

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

// buildOverrideYAML builds a minimal compose override with only env_file overrides.
func buildOverrideYAML(envMap map[string][]string) ([]byte, error) {
	if len(envMap) == 0 {
		return nil, nil
	}
	type svc struct {
		EnvFile []string `yaml:"env_file,omitempty"`
	}
	type root struct {
		Version  string           `yaml:"version,omitempty"`
		Services map[string]*svc  `yaml:"services"`
	}
	doc := root{Services: map[string]*svc{}}
	for name, refs := range envMap {
		// Absolute paths only (we materialize decrypted copies and point to them)
		doc.Services[name] = &svc{EnvFile: refs}
	}
	return yaml.Marshal(doc)
}

/* -------- Public: Compose inputs with SOPS auto-decrypt -------- */

// prepareStackFilesForCompose returns:
//   - composes: original compose file paths + one extra override file (last) if needed
//   - envs:     decrypted copies of substitution .env files to pass via --env-file
//   - cleanup:  removes the temporary workspace
//
// Design:
//   • Original compose files are NOT modified.
//   • We generate a tiny override compose (last -f) that replaces services[*].env_file
//     with absolute paths to decrypted temp copies.
//   • Substitution .env files (IaC files with role=env + default <stackDir>/.env)
//     are decrypted to temp and passed via repeated --env-file flags.
func prepareStackFilesForCompose(ctx context.Context, stackID int64) (composes []string, envs []string, cleanup func(), err error) {
	workspace, err := os.MkdirTemp("", "ddui-deploy-")
	if err != nil {
		return nil, nil, func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(workspace) }

	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return nil, nil, cleanup, err
	}
	// stack dir (for default .env interpolation)
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	stackDir := ""
	if strings.TrimSpace(rel) != "" {
		if d, jerr := joinUnder(root, rel); jerr == nil {
			stackDir = d
		}
	}

	// Gather tracked files
	rows, err := db.Query(ctx, `SELECT role, rel_path FROM iac_stack_files WHERE stack_id=$1`, stackID)
	if err != nil {
		return nil, nil, cleanup, err
	}
	defer rows.Close()

	type rec struct{ role, path string }
	var files []rec
	for rows.Next() {
		var role, rp string
		if err := rows.Scan(&role, &rp); err != nil {
			return nil, nil, cleanup, err
		}
		full, jerr := joinUnder(root, rp)
		if jerr != nil {
			return nil, nil, cleanup, jerr
		}
		files = append(files, rec{role: strings.ToLower(role), path: full})
	}

	// 1) Original compose files (unchanged)
	var composePaths []string
	for _, f := range files {
		if f.role == "compose" {
			composePaths = append(composePaths, f.path)
		}
	}
	composes = append(composes, composePaths...) // keep original order

	// 2) Service env_file overrides (decrypt each, point override to abs temp copies)
	overrideMap := map[string][]string{} // service -> []abs paths (decrypted)
	for _, cpath := range composePaths {
		// Read compose (decrypt if SOPS-structured)
		plain, _, rerr := readDecryptedOrPlain(ctx, cpath, "")
		if rerr != nil {
			return nil, nil, cleanup, rerr
		}
		refMap, perr := parseEnvFileRefs(plain)
		if perr != nil {
			return nil, nil, cleanup, perr
		}
		if len(refMap) == 0 {
			continue
		}
		baseDir := filepath.Dir(cpath)
		for svc, refs := range refMap {
			for _, ref := range refs {
				target := ref
				if !filepath.IsAbs(target) {
					target = filepath.Join(baseDir, ref)
				}
				content, _, derr := readDecryptedOrPlain(ctx, target, "dotenv")
				if derr != nil {
					return nil, nil, cleanup, derr
				}
				content = filterDotenvSopsKeys(content)  
				tmpAbs, werr := materializeFile(workspace, target, content)
				if werr != nil {
					return nil, nil, cleanup, werr
				}
				overrideMap[svc] = append(overrideMap[svc], tmpAbs)
			}
		}
	}

	// If we have any service env overrides, write a single override file and append it last.
	if len(overrideMap) > 0 {
		ovYAML, merr := buildOverrideYAML(overrideMap)
		if merr != nil {
			return nil, nil, cleanup, merr
		}
		ovPath := filepath.Join(workspace, "override.envfiles.yaml")
		if err := os.WriteFile(ovPath, ovYAML, 0o600); err != nil {
			return nil, nil, cleanup, err
		}
		composes = append(composes, ovPath) // must be last so it overrides originals
	}

	// 3) Substitution envs: include IaC role=env and default <stackDir>/.env
	seen := map[string]bool{}
	for _, f := range files {
		if f.role != "env" {
			continue
		}
		if seen[f.path] {
			continue
		}
		content, _, derr := readDecryptedOrPlain(ctx, f.path, "dotenv")
		if derr != nil {
			return nil, nil, cleanup, derr
		}
		content = filterDotenvSopsKeys(content)  
		tmpAbs, werr := materializeFile(workspace, f.path, content)
		if werr != nil {
			return nil, nil, cleanup, werr
		}
		envs = append(envs, tmpAbs)
		seen[f.path] = true
	}
	if stackDir != "" {
		defEnv := filepath.Join(stackDir, ".env")
		if _, statErr := os.Stat(defEnv); statErr == nil && !seen[defEnv] {
			content, _, derr := readDecryptedOrPlain(ctx, defEnv, "dotenv")
			if derr != nil {
				return nil, nil, cleanup, derr
			}
			content = filterDotenvSopsKeys(content)  
			tmpAbs, werr := materializeFile(workspace, defEnv, content)
			if werr != nil {
				return nil, nil, cleanup, werr
			}
			envs = append(envs, tmpAbs)
		}
	}

	return composes, envs, cleanup, nil
}
