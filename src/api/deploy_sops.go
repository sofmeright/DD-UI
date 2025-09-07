package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

	// Optimistic: always try sops -d; treat "not encrypted" as no-op.
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

// Returns compose files and env files (absolute paths) ready for docker compose,
// plus a cleanup() that removes any temp decrypted files we created.
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

	// Resolve the stack working directory (used for default .env detection)
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	dir, jerr := joinUnder(root, rel)
	if jerr != nil {
		return nil, nil, cleanup, jerr
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
		var role, relp string
		var sops bool
		if err := rows.Scan(&role, &relp, &sops); err != nil {
			return nil, nil, cleanup, err
		}
		full, jerr := joinUnder(root, relp)
		if jerr != nil {
			return nil, nil, cleanup, jerr
		}
		files = append(files, rec{role: strings.ToLower(role), path: full, sops: sops})
	}

	// Track which env files we already added (by cleaned absolute path)
	addedEnv := map[string]bool{}

	// Decrypt compose + tracked env files
	for _, f := range files {
		switch f.role {
		case "compose", "env":
			usePath := f.path
			if hasSopsKeys() {
				dec, clr, decOK, derr := decryptIfNeeded(ctx, f.path)
				if derr != nil {
					cleanup()
					return nil, nil, nil, derr
				}
				if decOK {
					cleanups = append(cleanups, clr)
					usePath = dec
				}
			}
			if f.role == "compose" {
				composes = append(composes, usePath)
			} else {
				envs = append(envs, usePath)
				addedEnv[filepath.Clean(f.path)] = true
			}
		default:
			// scripts/other: not fed to compose directly
		}
	}

	// NEW: Also include the default .env in the stack directory if present,
	// even if it isn't tracked. Put it LAST so it has highest precedence.
	defEnv := filepath.Join(dir, ".env")
	if fi, err := os.Stat(defEnv); err == nil && !fi.IsDir() {
		if !addedEnv[filepath.Clean(defEnv)] {
			usePath := defEnv
			if hasSopsKeys() {
				dec, clr, decOK, derr := decryptIfNeeded(ctx, defEnv)
				if derr != nil {
					cleanup()
					return nil, nil, nil, derr
				}
				if decOK {
					cleanups = append(cleanups, clr)
					usePath = dec
				}
			}
			envs = append(envs, usePath)
		}
	}

	return composes, envs, cleanup, nil
}
