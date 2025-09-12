// src/api/compose_render_hash.go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// computeRenderedConfigHash runs `docker compose config --hash` against the staged
// compose set and produces a stable hash by sorting and hashing all lines.
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

// computeComposeFilesHash returns sha256 over the concatenated bytes
// of all staged compose files (in the given order).
func computeComposeFilesHash(stageDir string, files []string) (string, error) {
	h := sha256.New()
	for _, f := range files {
		fp := f
		if !filepath.IsAbs(fp) {
			fp = filepath.Join(stageDir, f)
		}
		fd, err := os.Open(fp)
		if err != nil {
			return "", err
		}
		_, _ = io.Copy(h, fd)
		_ = fd.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RenderedService is the post-interpolation, post-SOPS view used by the UI.
type RenderedService struct {
	ServiceName   string `json:"service_name"`
	ContainerName string `json:"container_name,omitempty"`
	Image         string `json:"image,omitempty"`
}

// renderComposeServices runs `docker compose config --format json` and extracts
// the final (rendered) services with resolved image and container_name.
func renderComposeServices(ctx context.Context, stageDir, projectName string, files []string) ([]RenderedService, error) {
	args := []string{"compose", "-p", projectName}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--format", "json")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose config: %v\n%s", err, string(out))
	}

	var payload struct {
		Services map[string]struct {
			Image         string `json:"image"`
			ContainerName string `json:"container_name"`
		} `json:"services"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}

	var rs []RenderedService
	for name, sv := range payload.Services {
		rs = append(rs, RenderedService{
			ServiceName:   name,
			ContainerName: strings.TrimSpace(sv.ContainerName),
			Image:         strings.TrimSpace(sv.Image),
		})
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].ServiceName < rs[j].ServiceName })
	return rs, nil
}
