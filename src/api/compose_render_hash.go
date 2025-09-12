// compose_render_hash.go
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
)

// computeRenderedConfigHash runs `docker compose config --hash` against the staged
// compose set and produces a single stable hash by sorting and hashing all lines.
// On failure, returns empty string.
func computeRenderedConfigHash(ctx context.Context, stageDir string, projectName string, files []string) string {
	lines, ok := renderedConfigHashLines(ctx, stageDir, projectName, files)
	if !ok {
		return ""
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, ln := range lines {
		h.Write([]byte(ln))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// renderedConfigHashLines returns the raw "service: hash" lines from `compose config --hash`.
func renderedConfigHashLines(ctx context.Context, stageDir string, projectName string, files []string) ([]string, bool) {
	args := []string{"compose", "-p", projectName}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--hash")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, false
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, false
	}
	lines := strings.Split(raw, "\n")
	trimmed := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			trimmed = append(trimmed, ln)
		}
	}
	return trimmed, true
}

// computeRenderedServiceHashes parses the `compose config --hash` output into service->hash.
func computeRenderedServiceHashes(ctx context.Context, stageDir string, projectName string, files []string) (map[string]string, bool) {
	lines, ok := renderedConfigHashLines(ctx, stageDir, projectName, files)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(lines))
	for _, ln := range lines {
		// format: "service: HASH"
		col := strings.IndexByte(ln, ':')
		if col <= 0 {
			continue
		}
		svc := strings.TrimSpace(ln[:col])
		hash := strings.TrimSpace(ln[col+1:])
		out[svc] = hash
	}
	return out, true
}

// RenderedServiceBrief is returned to UI via EnhancedIacStackOut (post-interpolation/SOPS).
type RenderedServiceBrief struct {
	ServiceName   string `json:"service_name"`
	ContainerName string `json:"container_name,omitempty"`
	Image         string `json:"image,omitempty"`
	Hash          string `json:"hash,omitempty"`
}

// renderComposeServices runs `docker compose config --format json` and extracts services.
func renderComposeServices(ctx context.Context, stageDir string, projectName string, files []string) ([]RenderedServiceBrief, bool) {
	args := []string{"compose", "-p", projectName}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--format", "json")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, false
	}

	// The JSON shape is roughly: {"services":{"svc":{"image":"...","container_name":"..."}}}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		return nil, false
	}
	services, _ := root["services"].(map[string]any)

	var list []RenderedServiceBrief
	for svcName, v := range services {
		m, _ := v.(map[string]any)
		img, _ := m["image"].(string)
		cn, _ := m["container_name"].(string)
		list = append(list, RenderedServiceBrief{
			ServiceName:   svcName,
			ContainerName: strings.TrimSpace(cn),
			Image:         strings.TrimSpace(img),
		})
	}
	// stable order
	sort.Slice(list, func(i, j int) bool { return list[i].ServiceName < list[j].ServiceName })
	return list, true
}

// helper if needed somewhere else
func hashBytesSorted(lines [][]byte) string {
	sort.Slice(lines, func(i, j int) bool { return bytes.Compare(lines[i], lines[j]) < 0 })
	h := sha256.New()
	for _, b := range lines {
		h.Write(b)
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
