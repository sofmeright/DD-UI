// src/api/scan_iac.go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"          // needed by toString + empty-dir error
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	iacDefaultRootEnv = "DDUI_IAC_ROOT"
	iacDirNameEnv     = "DDUI_IAC_DIRNAME"
	iacDefaultRoot    = "/data"          // mount your repo at /data
	iacDefaultDirName = "docker-compose" // per your layout
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
    root := strings.TrimSpace(env(iacDefaultRootEnv, iacDefaultRoot))
    dirname := strings.TrimSpace(env(iacDirNameEnv, iacDefaultDirName))
    base := filepath.Join(root, dirname)

    log.Printf("iac: scan start root=%q dir=%q base=%q", root, dirname, base)

    repoID, err := upsertIacRepoLocal(ctx, root)
    if err != nil {
        log.Printf("iac: repo upsert failed root=%q err=%v", root, err)
        return 0, 0, err
    }

    // If base missing or not a dir
    if fi, err := os.Stat(base); err != nil {
        if os.IsNotExist(err) {
            _, _ = db.Exec(ctx, `UPDATE iac_repos SET last_scan_at=now() WHERE id=$1`, repoID)
            log.Printf("iac: base path not found; nothing to scan (base=%q)", base)
            return 0, 0, nil
        }
        log.Printf("iac: stat base failed base=%q err=%v", base, err)
        return 0, 0, err
    } else if !fi.IsDir() {
        log.Printf("iac: base is not a directory base=%q", base)
        return 0, 0, fmt.Errorf("%s is not a directory", base)
    }

    var keepStackIDs []int64
    stacksFound := 0
    servicesSaved := 0

    walkFn := func(p string, d fs.DirEntry, _ error) error {
        if d == nil || !d.IsDir() {
            return nil
        }
        rel, _ := filepath.Rel(root, p)
        parts := strings.Split(filepath.ToSlash(rel), "/")
        if len(parts) < 3 || parts[0] != dirname {
            return nil
        }

        scopeName := parts[1]
        stackName := parts[2]
        if scopeName == "" || stackName == "" {
            return nil
        }

        scopeKind := "group"
        if _, err := GetHostByName(ctx, scopeName); err == nil {
            scopeKind = "host"
        }

        composeFile := findOne(p, []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"})
        deployKind := "unmanaged"
        if composeFile != "" { deployKind = "compose" }
        if existsAny(p, []string{"deploy.sh", "pre.sh", "post.sh"}) { deployKind = "script" }

        envFiles := listEnvFiles(p)
        sopsStatus := summarizeSops(envFiles)

        stackID, err := upsertIacStack(ctx, repoID, scopeKind, scopeName, stackName,
            filepath.ToSlash(filepath.Join(dirname, scopeName, stackName)),
            composeFile, deployKind, "", sopsStatus, true)
        if err != nil {
            log.Printf("iac: upsert stack failed scope=%s name=%s path=%q err=%v", scopeName, stackName, p, err)
            return fs.SkipDir
        }
        keepStackIDs = append(keepStackIDs, stackID)
        stacksFound++

        log.Printf("iac: stack found scope_kind=%s scope=%s stack=%s deploy=%s compose=%q sops=%s",
            scopeKind, scopeName, stackName, deployKind, composeFile, sopsStatus)

        // track files
        for _, ef := range envFiles {
            sum, sz := sha256File(ef.fullPath)
            _ = upsertIacFile(ctx, stackID, "env", relFrom(root, ef.fullPath), ef.sops, sum, sz)
            log.Printf("iac: file env stack=%s/%s rel=%q sops=%v size=%d", scopeName, stackName, relFrom(root, ef.fullPath), ef.sops, sz)
        }
        if composeFile != "" {
            full := filepath.Join(p, composeFile)
            sum, sz := sha256File(full)
            _ = upsertIacFile(ctx, stackID, "compose", filepath.ToSlash(filepath.Join(dirname, scopeName, stackName, composeFile)), false, sum, sz)
            log.Printf("iac: file compose stack=%s/%s rel=%q size=%d", scopeName, stackName, filepath.ToSlash(filepath.Join(dirname, scopeName, stackName, composeFile)), sz)
        }
        for _, s := range []string{"deploy.sh", "pre.sh", "post.sh"} {
            full := filepath.Join(p, s)
            if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
                sum, sz := sha256File(full)
                _ = upsertIacFile(ctx, stackID, "script", relFrom(root, full), false, sum, sz)
                log.Printf("iac: file script stack=%s/%s rel=%q size=%d", scopeName, stackName, relFrom(root, full), sz)
            }
        }

        // compose → services
        if composeFile != "" {
            b, _ := os.ReadFile(filepath.Join(p, composeFile))
            cdoc := &composeDoc{}
            _ = yaml.Unmarshal(b, cdoc)
            pullPolicy := strings.TrimSpace(cdoc.XPull)

            names := make([]string, 0, len(cdoc.Services))
            for k := range cdoc.Services { names = append(names, k) }
            sort.Strings(names)

            for _, svcName := range names {
                svc := cdoc.Services[svcName]
                if svc == nil { continue }
                lbls := normLabels(svc.Labels)
                envKeys, envF := normEnv(svc.Environment, svc.EnvFile, p, envFiles)
                ports := normPorts(svc.Ports)
                vols := normVolumes(svc.Volumes)
                if err := upsertIacService(ctx, IacServiceRow{
                    StackID: stackID, ServiceName: svcName, ContainerName: svc.ContainerName,
                    Image: svc.Image, Labels: lbls, EnvKeys: envKeys, EnvFiles: envF,
                    Ports: ports, Volumes: vols, Deploy: svc.Deploy,
                }); err != nil {
                    log.Printf("iac: upsert service failed stack=%s/%s svc=%s err=%v", scopeName, stackName, svcName, err)
                } else {
                    servicesSaved++
                }
            }
            if pullPolicy != "" {
                _, _ = db.Exec(ctx, `UPDATE iac_stacks SET pull_policy=$1 WHERE id=$2`, pullPolicy, stackID)
            }
        }

        return fs.SkipDir
    }

    if err := filepath.WalkDir(base, walkFn); err != nil {
        if !os.IsNotExist(err) {
            log.Printf("iac: walk failed base=%q err=%v", base, err)
            return 0, 0, err
        }
    }

    _, _ = pruneIacStacksNotIn(ctx, repoID, keepStackIDs)
    _, _ = db.Exec(ctx, `UPDATE iac_repos SET last_scan_at=now() WHERE id=$1`, repoID)

    log.Printf("iac: scan done stacks=%d services=%d", stacksFound, servicesSaved)
    return stacksFound, servicesSaved, nil
}

/* ---------- helpers ---------- */

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
	_, _ = h.Write([]byte{}) // no-op
	// simple read
	b, _ := os.ReadFile(p)
	_, _ = h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), fi.Size()
}

// ---- normalizers ----

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

func normLabels(v any) map[string]string {
	out := map[string]string{}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			out[k] = toString(val)
		}
	case map[string]string:
		return t
	case []any:
		// ["k=v","x=y"]
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

func normEnv(env any, envFile any, stackDir string, discovered []envFileMeta) ([]string, []IacEnvFile) {
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

	// Map env_file → discovered meta
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
		// try match to discovered in this stack dir
		sops := false
		for _, d := range discovered {
			if d.rel == base {
				sops = d.sops
				break
			}
		}
		files = append(files, IacEnvFile{Path: filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(stackDir)), filepath.Base(stackDir), base)), Sops: sops})
	}

	// return sorted keys
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, files
}

func normPorts(v any) []map[string]any {
	out := []map[string]any{}
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			switch p := it.(type) {
			case string:
				// "80:8080/tcp" or "8080"
				host, cont, proto := splitPortString(p)
				out = append(out, map[string]any{"published": host, "target": cont, "protocol": proto})
			case map[string]any:
				m := map[string]any{}
				for k, v := range p {
					m[strings.ToLower(k)] = v
				}
				// docker compose uses target/published/protocol
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

func splitPortString(s string) (host, cont, proto string) {
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

func normVolumes(v any) []map[string]any {
	out := []map[string]any{}
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			switch vv := it.(type) {
			case string:
				// "/host:/ctr:ro"
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

func splitVolString(s string) (src, dst, mode string) {
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
