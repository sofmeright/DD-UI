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

// We try to decrypt when we have AGE private key material.
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

// quick content sniff for .yaml/.yml/.json files
func looksSopsStructured(content string) bool {
	// very loose; good enough for routing to sops -d
	return strings.Contains(content, "\nsops:") || strings.Contains(content, "\"sops\"")
}

// Returns (pathToUse, cleanupFn, wasDecrypted, error)
func decryptIfNeeded(ctx context.Context, full string) (string, func(), bool, error) {
	cleanup := func() {}
	// If we have no keys, we can't decrypt. Just return the original path.
	if !hasSopsKeys() {
		return full, cleanup, false, nil
	}

	low := strings.ToLower(full)
	inputType := ""
	if strings.HasSuffix(low, ".env") {
		inputType = "dotenv"
	}

	// Optimistic: even if heuristics don't trigger, try sops -d (ignore "not encrypted" errors)
	args := []string{"-d"}
	if inputType == "dotenv" {
		args = append(args, "--input-type", "dotenv")
	}
	args = append(args, full)

	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(dctx, "sops", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.ToLower(string(out))
		if strings.Contains(outStr, "file is not encrypted") || strings.Contains(outStr, "sops metadata not found") {
			return full, cleanup, false, nil
		}
		return "", cleanup, false, fmt.Errorf("sops decrypt failed for %s: %v: %s", full, err, strings.TrimSpace(string(out)))
	}

	tmpDir := os.TempDir()
	base := filepath.Base(full)
	tmpPath := filepath.Join(tmpDir, ".ddui-dec-"+base)
	if err := os.WriteFile(tmpPath, out, 0o600); err != nil {
		return "", cleanup, false, fmt.Errorf("write temp decrypted file: %w", err)
	}
	cleanup = func() { _ = os.Remove(tmpPath) }
	return tmpPath, cleanup, true, nil
}

// rewriteComposeWithDecryptedEnvFiles creates a temp directory, copies the compose
// YAML into it, and rewrites any services[*].env_file (string or list) to point to
// decrypted (or plain-copied) temp files. It returns the new compose path and a cleanup().
func rewriteComposeWithDecryptedEnvFiles(ctx context.Context, composePath string) (string, func(), error) {
	cleanup := func() {}

	b, err := os.ReadFile(composePath)
	if err != nil {
		return "", cleanup, err
	}

	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		// Not YAML? Just return original (Compose may still accept it).
		return composePath, cleanup, nil
	}

	// Create a temp project dir to hold rewritten compose + env copies
	tmpDir, err := os.MkdirTemp("", "ddui-compose-")
	if err != nil {
		return "", cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	baseDir := filepath.Dir(composePath)

	// Helper: ensure a referenced env file is materialized in tmpDir (decrypted if needed),
	// and return a relative filename we can put back into YAML.
	type envCopy struct {
		rel string
	}
	materializeEnv := func(ref string) (envCopy, error) {
		target := ref
		if !filepath.IsAbs(target) {
			target = filepath.Join(baseDir, target)
		}
		use := target
		if hasSopsKeys() {
			dec, decClr, decOK, derr := decryptIfNeeded(ctx, target)
			if derr != nil {
				return envCopy{}, derr
			}
			if decOK {
				defer decClr() // we copy the content below; temp from decrypt can be discarded when function exits
				use = dec
			}
		}
		content, rerr := os.ReadFile(use)
		if rerr != nil {
			return envCopy{}, rerr
		}
		// stable unique name inside tmpDir to avoid collisions
		h := sha1.Sum([]byte(target))
		name := filepath.Base(target)
		dst := filepath.Join(tmpDir, fmt.Sprintf("%s.%s", hex.EncodeToString(h[:8]), name))
		if err := os.WriteFile(dst, content, 0o600); err != nil {
			return envCopy{}, err
		}
		return envCopy{rel: filepath.Base(dst)}, nil
	}

	// Walk services -> env_file
	svcs, _ := doc["services"].(map[string]any)
	if svcs != nil {
		for _, v := range svcs {
			m, _ := v.(map[string]any)
			if m == nil {
				continue
			}
			if ef, ok := m["env_file"]; ok {
				switch t := ef.(type) {
				case string:
					cp, err := materializeEnv(t)
					if err != nil {
						return "", cleanup, err
					}
					m["env_file"] = cp.rel
				case []any:
					out := make([]any, 0, len(t))
					for _, item := range t {
						s, _ := item.(string)
						if strings.TrimSpace(s) == "" {
							continue
						}
						cp, err := materializeEnv(s)
						if err != nil {
							return "", cleanup, err
						}
						out = append(out, cp.rel)
					}
					m["env_file"] = out
				}
			}
		}
	}

	// Write modified compose into tmpDir
	outBytes, _ := yaml.Marshal(doc)
	newCompose := filepath.Join(tmpDir, filepath.Base(composePath))
	if err := os.WriteFile(newCompose, outBytes, 0o600); err != nil {
		return "", cleanup, err
	}

	return newCompose, cleanup, nil
}

// Returns compose files and env files (absolute paths) ready for docker compose,
// plus a cleanup() that removes any temp decrypted files we created.
//
// Notes:
//  - Compose files themselves may be SOPS-encrypted: we decrypt them first.
//  - Any services[*].env_file entries are rewritten to point to decrypted temp copies.
//  - We still include the default <stackDir>/.env for variable substitution.
func prepareStackFilesForCompose(ctx context.Context, stackID int64) (composes []string, envs []string, cleanup func(), err error) {
	cleanups := []func(){}

	cleanup = func() {
		for _, f := range cleanups {
			f()
		}
	}

	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return nil, nil, cleanup, err
	}

	// Figure out the stack directory (for default .env)
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	stackDir := ""
	if strings.TrimSpace(rel) != "" {
		if d, jerr := joinUnder(root, rel); jerr == nil {
			stackDir = d
		}
	}

	// role: compose|env|script|other
	rows, err := db.Query(ctx, `SELECT role, rel_path, COALESCE(sops,false) FROM iac_stack_files WHERE stack_id=$1`, stackID)
	if err != nil {
		return nil, nil, cleanup, err
	}
	defer rows.Close()

	type rec struct {
		role string
		path string
		sops bool
	}
	var files []rec
	for rows.Next() {
		var role, rp string
		var sops bool
		if err := rows.Scan(&role, &rp, &sops); err != nil {
			return nil, nil, cleanup, err
		}
		full, jerr := joinUnder(root, rp)
		if jerr != nil {
			return nil, nil, cleanup, jerr
		}
		files = append(files, rec{role: strings.ToLower(role), path: full, sops: sops})
	}

	// Deduplicate by original path
	composeSeen := map[string]bool{}
	envSeen := map[string]bool{}

	for _, f := range files {
		switch f.role {
		case "compose":
			// 1) decrypt compose file if needed
			use := f.path
			if hasSopsKeys() {
				dec, clr, decOK, derr := decryptIfNeeded(ctx, f.path)
				if derr != nil {
					cleanup()
					return nil, nil, nil, derr
				}
				if decOK {
					cleanups = append(cleanups, clr)
					use = dec
				}
			}
			// 2) rewrite compose to point env_file entries to decrypted copies
			newCompose, rwClr, rerr := rewriteComposeWithDecryptedEnvFiles(ctx, use)
			if rerr != nil {
				cleanup()
				return nil, nil, nil, rerr
			}
			cleanups = append(cleanups, rwClr)
			if !composeSeen[f.path] {
				composes = append(composes, newCompose)
				composeSeen[f.path] = true
			}
		case "env":
			// These are env files referenced via global --env-file for substitution.
			use := f.path
			if hasSopsKeys() {
				dec, clr, decOK, derr := decryptIfNeeded(ctx, f.path)
				if derr != nil {
					cleanup()
					return nil, nil, nil, derr
				}
				if decOK {
					cleanups = append(cleanups, clr)
					use = dec
				}
			}
			if !envSeen[f.path] {
				envs = append(envs, use)
				envSeen[f.path] = true
			}
		default:
			// scripts/other not passed to compose directly
		}
	}

	// Include default <stackDir>/.env (for substitution) even if not tracked in IaC.
	if stackDir != "" {
		defEnv := filepath.Join(stackDir, ".env")
		if _, statErr := os.Stat(defEnv); statErr == nil && !envSeen[defEnv] {
			use := defEnv
			if hasSopsKeys() {
				dec, clr, decOK, derr := decryptIfNeeded(ctx, defEnv)
				if derr != nil {
					cleanup()
					return nil, nil, nil, derr
				}
				if decOK {
					cleanups = append(cleanups, clr)
					use = dec
				}
			}
			envs = append(envs, use)
			envSeen[defEnv] = true
		}
	}

	return composes, envs, cleanup, nil
}