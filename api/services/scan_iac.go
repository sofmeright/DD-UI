// src/api/scan_iac.go
package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"dd-ui/common"
	"dd-ui/database"
	"github.com/goccy/go-yaml"
)

var (
	IacDefaultRootEnv = "DD_UI_IAC_ROOT"
	IacDirNameEnv     = "DD_UI_IAC_DIRNAME"
	IacDefaultRoot    = "/data"          // mount your repo at /data
	IacDefaultDirName = "docker-compose" // per your layout
)

type composeDoc struct {
	Services map[string]*composeSvc `yaml:"services"`
	XPull    string                 `yaml:"x-pull-policy"`
}

type composeSvc struct {
	Image         string         `yaml:"image"`
	ContainerName string         `yaml:"container_name"`
	Labels        any            `yaml:"labels"`
	Environment   any            `yaml:"environment"`
	EnvFile       any            `yaml:"env_file"`
	Ports         any            `yaml:"ports"`
	Volumes       any            `yaml:"volumes"`
	Deploy        map[string]any `yaml:"deploy"`
}

// Entry point called by API
func ScanIacLocal(ctx context.Context) (int, int, error) {
	root := strings.TrimSpace(common.Env(IacDefaultRootEnv, IacDefaultRoot))
	dirname := strings.TrimSpace(common.Env(IacDirNameEnv, IacDefaultDirName))
	base := filepath.Join(root, dirname)

	common.InfoLog("iac: scan start root=%q base=%q", root, base)

	repoID, err := UpsertIacRepoLocal(ctx, root)
	if err != nil {
		common.ErrorLog("iac: repo upsert failed root=%q err=%v", root, err)
		return 0, 0, err
	}

	// Discover: docker-compose/<scopeName>/<stackName>
	var keepStackIDs []int64
	stacksFound := 0
	servicesSaved := 0

	// If the base doesn't exist or isn't a dir, treat as "nothing to scan".
	if fi, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			common.InfoLog("iac: base dir %q missing; nothing to scan", base)
			_, _ = common.DB.Exec(ctx, `UPDATE iac_repos SET last_scan_at=now() WHERE id=$1`, repoID)
			return 0, 0, nil
		}
		common.ErrorLog("iac: stat base err=%v", err)
		return 0, 0, err
	} else if !fi.IsDir() {
		err := fmt.Errorf("%s is not a directory", base)
		common.ErrorLog("iac: %v", err)
		return 0, 0, err
	}

	walkFn := func(p string, d fs.DirEntry, _ error) error {
		if d == nil || !d.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(root, p)
		parts := strings.Split(filepath.ToSlash(rel), "/")

		// We only process *stack directories*: docker-compose/<scope>/<stack>
		if len(parts) < 3 || parts[0] != dirname {
			return nil
		}

		scopeName := parts[1]
		stackName := parts[2]
		if scopeName == "" || stackName == "" {
			return nil
		}

		// Determine scope_kind by checking if scopeName is a known host
		scopeKind := "group"
		if _, err := database.GetHostByName(ctx, scopeName); err == nil {
			scopeKind = "host"
		}

		composeFile := findOne(p, []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"})
		deployKind := "unmanaged"
		if composeFile != "" {
			deployKind = "compose"
		}
		if existsAny(p, []string{"deploy.sh", "pre.sh", "post.sh"}) {
			deployKind = "script" // if both exist we keep compose
		}

		// env files (record + sops detection)
		envFiles := listEnvFiles(p)
		sopsStatus := summarizeSops(envFiles)

		// Check if this directory actually has any IaC content before creating a stack record
		if !directoryHasIacContent(p, composeFile, envFiles) {
			common.DebugLog("iac: skipping empty directory %s/%s (no IaC content found)", scopeName, stackName)
			return fs.SkipDir
		}

		stackID, err := UpsertIacStack(ctx, repoID, scopeKind, scopeName, stackName,
			filepath.ToSlash(filepath.Join(dirname, scopeName, stackName)),
			composeFile, deployKind, "", sopsStatus, true)
		if err != nil {
			common.ErrorLog("iac: stack upsert failed scope=%s stack=%s path=%s err=%v", scopeName, stackName, p, err)
			return fs.SkipDir
		}
		common.InfoLog("iac: stack %s/%s id=%d deploy=%s compose=%q sops=%s", scopeName, stackName, stackID, deployKind, composeFile, sopsStatus)

		keepStackIDs = append(keepStackIDs, stackID)
		stacksFound++

		// Track files
		for _, ef := range envFiles {
			sum, sz := sha256File(ef.fullPath)
			if err := UpsertIacFile(ctx, stackID, "env", relFrom(root, ef.fullPath), ef.sops, sum, sz); err != nil {
				common.ErrorLog("iac: upsert file(env) failed stack_id=%d file=%s err=%v", stackID, ef.fullPath, err)
			}
		}
		if composeFile != "" {
			full := filepath.Join(p, composeFile)
			sum, sz := sha256File(full)
			if err := UpsertIacFile(ctx, stackID, "compose", filepath.ToSlash(filepath.Join(dirname, scopeName, stackName, composeFile)), false, sum, sz); err != nil {
				common.ErrorLog("iac: upsert file(compose) failed stack_id=%d file=%s err=%v", stackID, full, err)
			}
		}
		for _, s := range []string{"deploy.sh", "pre.sh", "post.sh"} {
			full := filepath.Join(p, s)
			if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
				sum, sz := sha256File(full)
				if err := UpsertIacFile(ctx, stackID, "script", relFrom(root, full), false, sum, sz); err != nil {
					common.ErrorLog("iac: upsert file(script) failed stack_id=%d file=%s err=%v", stackID, full, err)
				}
			}
		}

		// Parse compose → services
		if composeFile != "" {
			b, _ := os.ReadFile(filepath.Join(p, composeFile))
			cdoc := &composeDoc{}
			_ = yaml.Unmarshal(b, cdoc)
			pullPolicy := strings.TrimSpace(cdoc.XPull)

			// services in deterministic order
			names := make([]string, 0, len(cdoc.Services))
			for k := range cdoc.Services {
				names = append(names, k)
			}
			sort.Strings(names)

			for _, svcName := range names {
				svc := cdoc.Services[svcName]
				if svc == nil {
					continue
				}

				lbls := normLabels(svc.Labels)
				envKeys, envF := normEnv(svc.Environment, svc.EnvFile, p, envFiles)
				ports := normPorts(svc.Ports)
				vols := normVolumes(svc.Volumes)

				if err := upsertIacService(ctx, IacServiceRow{
					StackID:       stackID,
					ServiceName:   svcName,
					ContainerName: svc.ContainerName,
					Image:         svc.Image,
					Labels:        lbls,
					EnvKeys:       envKeys,
					EnvFiles:      envF,
					Ports:         ports,
					Volumes:       vols,
					Deploy:        svc.Deploy,
				}); err != nil {
					common.ErrorLog("iac: upsert service failed stack_id=%d svc=%s err=%v", stackID, svcName, err)
				} else {
					servicesSaved++
				}
			}

			// update pull_policy if present
			if pullPolicy != "" {
				_, _ = common.DB.Exec(ctx, `UPDATE iac_stacks SET pull_policy=$1 WHERE id=$2`, pullPolicy, stackID)
			}
		}

		// We are *at* a stack directory; don't descend further into it.
		return fs.SkipDir
	}

	if err := filepath.WalkDir(base, walkFn); err != nil {
		if !os.IsNotExist(err) {
			common.ErrorLog("iac: walk err=%v", err)
			return 0, 0, err
		}
	}

	// prune empty stacks (abandoned stack creation)
	if n, err := pruneEmptyIacStacks(ctx, repoID); err == nil && n > 0 {
		common.InfoLog("iac: pruned %d empty stacks (no files) from repo_id=%d", n, repoID)
	}

	// prune removed stacks for this repo
	if n, err := pruneIacStacksNotIn(ctx, repoID, keepStackIDs); err == nil && n > 0 {
		common.InfoLog("iac: pruned %d stacks no longer present in repo_id=%d", n, repoID)
	}

	_, _ = common.DB.Exec(ctx, `UPDATE iac_repos SET last_scan_at=now() WHERE id=$1`, repoID)

	common.InfoLog("iac: scan done repo_id=%d stacks=%d services=%d", repoID, stacksFound, servicesSaved)
	return stacksFound, servicesSaved, nil
}

/* ---------- helpers [unchanged except imports] ---------- */

type envFileMeta struct {
	fullPath string
	sops     bool
	rel      string
}

func listEnvFiles(dir string) []envFileMeta {
	var out []envFileMeta
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".env") && !strings.Contains(name, ".env.") && !strings.HasSuffix(name, "_secret.env") && !strings.HasSuffix(name, "_private.env") && !strings.HasSuffix(name, "_secure.env") && name != ".env" {
			continue
		}
		full := filepath.Join(dir, name)
		b, _ := os.ReadFile(full)
		out = append(out, envFileMeta{
			fullPath: full,
			sops:     looksSops(b),
			rel:      filepath.Base(full),
		})
	}
	return out
}

func summarizeSops(envs []envFileMeta) string {
	if len(envs) == 0 {
		return "none"
	}
	total := 0
	enc := 0
	for _, e := range envs {
		total++
		if e.sops {
			enc++
		}
	}
	if enc == 0 {
		return "none"
	}
	if enc == total {
		return "all"
	}
	return "partial"
}

func findOne(dir string, cand []string) string {
	for _, n := range cand {
		if fi, err := os.Stat(filepath.Join(dir, n)); err == nil && !fi.IsDir() {
			return n
		}
	}
	return ""
}

func existsAny(dir string, cand []string) bool {
	for _, n := range cand {
		if fi, err := os.Stat(filepath.Join(dir, n)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func relFrom(root, full string) string {
	r, _ := filepath.Rel(root, full)
	return filepath.ToSlash(r)
}

var sopsMarker = regexp.MustCompile(`(?i)\bsops\s*:\b|ENC\[|AGE-ENCRYPTED`)

func looksSops(b []byte) bool {
	peek := b
	if len(b) > 4096 {
		peek = b[:4096]
	}
	return sopsMarker.Match(peek)
}

func sha256File(p string) (hexsum string, size int64) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0
	}
	defer f.Close()
	fi, _ := f.Stat()
	h := sha256.New()
	b, _ := os.ReadFile(p)
	_, _ = h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), fi.Size()
}

// ---- normalizers (unchanged) ----

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}
func normLabels(v any) map[string]string { /* unchanged */ 
	out := map[string]string{}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			out[k] = toString(val)
		}
	case map[string]string:
		return t
	case []any:
		for _, it := range t {
			if s, ok := it.(string); ok {
				kv := strings.SplitN(s, "=", 2)
				if len(kv) == 2 {
					out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
		}
	}
	return out
}
func normEnv(env any, envFile any, stackDir string, discovered []envFileMeta) ([]string, []IacEnvFile) { /* unchanged */ 
	keys := map[string]struct{}{}
	switch t := env.(type) {
	case map[string]any:
		for k := range t {
			keys[k] = struct{}{}
		}
	case []any:
		for _, it := range t {
			if s, ok := it.(string); ok {
				k := s
				if i := strings.IndexByte(s, '='); i > 0 {
					k = s[:i]
				}
				keys[strings.TrimSpace(k)] = struct{}{}
			}
		}
	}
	var files []IacEnvFile
	paths := []string{}
	switch t := envFile.(type) {
	case string:
		paths = []string{t}
	case []any:
		for _, it := range t {
			if s, ok := it.(string); ok {
				paths = append(paths, s)
			}
		}
	}
	for _, p := range paths {
		base := filepath.Base(p)
		sops := false
		for _, d := range discovered {
			if d.rel == base {
				sops = d.sops
				break
			}
		}
		files = append(files, IacEnvFile{Path: filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(stackDir)), filepath.Base(stackDir), base)), Sops: sops})
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, files
}
func normPorts(v any) []map[string]any { /* unchanged */ 
	out := []map[string]any{}
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			switch p := it.(type) {
			case string:
				host, cont, proto := splitPortString(p)
				out = append(out, map[string]any{"published": host, "target": cont, "protocol": proto})
			case map[string]any:
				m := map[string]any{}
				for k, v := range p {
					m[strings.ToLower(k)] = v
				}
				out = append(out, map[string]any{
					"published": m["published"],
					"target":    m["target"],
					"protocol":  m["protocol"],
				})
			}
		}
	}
	return out
}
func splitPortString(s string) (host, cont, proto string) { /* unchanged */ 
	proto = "tcp"
	if i := strings.IndexByte(s, '/'); i > 0 {
		proto = strings.ToLower(strings.TrimSpace(s[i+1:]))
		s = s[:i]
	}
	if j := strings.IndexByte(s, ':'); j > 0 {
		return strings.TrimSpace(s[:j]), strings.TrimSpace(s[j+1:]), proto
	}
	return "", strings.TrimSpace(s), proto
}
func normVolumes(v any) []map[string]any { /* unchanged */ 
	out := []map[string]any{}
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			switch vv := it.(type) {
			case string:
				src, dst, mode := splitVolString(vv)
				out = append(out, map[string]any{"source": src, "target": dst, "mode": mode})
			case map[string]any:
				m := map[string]any{}
				for k, v := range vv {
					m[strings.ToLower(k)] = v
				}
				out = append(out, map[string]any{
					"source": m["source"],
					"target": m["target"],
					"mode":   m["read_only"],
				})
			}
		}
	}
	return out
}
func splitVolString(s string) (src, dst, mode string) { /* unchanged */ 
	mode = ""
	parts := strings.Split(s, ":")
	if len(parts) == 3 {
		return parts[0], parts[1], parts[2]
	}
	if len(parts) == 2 {
		return parts[0], parts[1], ""
	}
	return "", s, ""
}

// directoryHasIacContent checks if a directory contains actual IaC files
func directoryHasIacContent(dir, composeFile string, envFiles []envFileMeta) bool {
	// Has compose file
	if composeFile != "" {
		return true
	}
	
	// Has environment files
	if len(envFiles) > 0 {
		return true
	}
	
	// Has deployment scripts
	if existsAny(dir, []string{"deploy.sh", "pre.sh", "post.sh"}) {
		return true
	}
	
	// Directory is empty or contains no IaC-relevant files
	return false
}
