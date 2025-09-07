package main

import (
	"context"
	"errors"
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

	// Try to decide if this actually needs decryption. For .env we look for ENC[...
	// For structured formats we look for a 'sops' block. If unsure, we still try sops -d.
	b, _ := os.ReadFile(full)
	needs := false
	if inputType == "dotenv" {
		needs = strings.Contains(string(b), "ENC[")
	} else {
		needs = looksSopsStructured(string(b))
	}

	// Optimistic: even if heuristics didn't trigger, still try sops -d; treat "not encrypted" as no-op.
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

	// role: compose|env|script|other
	rows, err := db.Query(ctx, `SELECT role, rel_path, COALESCE(sops,false) FROM iac_files WHERE stack_id=$1`, stackID)
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
		var role, rel string
		var sops bool
		if err := rows.Scan(&role, &rel, &sops); err != nil {
			return nil, nil, cleanup, err
		}
		full, jerr := joinUnder(root, rel)
		if jerr != nil {
			return nil, nil, cleanup, jerr
		}
		files = append(files, rec{role: strings.ToLower(role), path: full, sops: sops})
	}

	// Decrypt only what docker compose reads directly: compose files + env files.
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
			}
		default:
			// scripts/other: not fed into compose directly
		}
	}

	return composes, envs, cleanup, nil
}
