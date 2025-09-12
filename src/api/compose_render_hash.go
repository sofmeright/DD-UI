// compose_render_hash.go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"sort"
	"strings"
)

// computeRenderedConfigHash runs `docker compose config --hash` against the staged
// compose set and produces a single stable hash by sorting and hashing all lines.
// On failure, returns empty string.
func computeRenderedConfigHash(ctx context.Context, stageDir string, projectName string, files []string) string {
	args := []string{"compose", "-p", projectName}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--hash")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	trimmed := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			trimmed = append(trimmed, ln)
		}
	}
	sort.Strings(trimmed)
	h := sha256.New()
	for _, ln := range trimmed {
		h.Write([]byte(ln))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
