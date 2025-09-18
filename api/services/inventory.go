// src/api/inventory.go
package services

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"github.com/goccy/go-yaml"
)

var (
	invMu   sync.RWMutex
	invPath string
	hosts   []common.Host
)

func InitInventory() error {
	p := common.Env("DD_UI_INVENTORY_PATH", "")
	if p == "" {
		p = findInventoryPath()
		if p == "" {
			// Try a few times in case the mount is slow
			for i := 0; i < 5; i++ {
				common.InfoLog("Inventory file not found (attempt %d/5), waiting...", i+1)
				time.Sleep(2 * time.Second)
				p = findInventoryPath()
				if p != "" {
					break
				}
			}
			if p == "" {
				return errors.New("no inventory file found; set DD_UI_INVENTORY_PATH or mount /data/inventory")
			}
		}
	}
	invPath = p
	return loadInventory(invPath)
}

func ReloadInventory() error {
	if invPath == "" {
		return errors.New("inventory not initialized")
	}
	return loadInventory(invPath)
}

// Optional: allow POST /api/inventory/reload with a new path.
func ReloadInventoryWithPath(p string) error {
	if p == "" {
		return ReloadInventory()
	}
	invPath = p
	return loadInventory(invPath)
}

func GetHosts() []common.Host {
	invMu.RLock()
	defer invMu.RUnlock()
	out := make([]common.Host, len(hosts))
	copy(out, hosts)
	return out
}

func findInventoryPath() string {
	cands := []string{"/data/inventory", "/data/inventory.yml", "/data/inventory.yaml"}
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func loadInventory(p string) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	kind, parsed, derr := detectInventoryFormat(b)
	if derr != nil {
		return derr
	}

	// Persist to DB (implemented in db_hosts.go)
	if err := database.ImportInventoryToDB(context.Background(), parsed); err != nil {
		return err
	}

	// Keep an in-memory copy for /api/hosts
	invMu.Lock()
	hosts = parsed
	invMu.Unlock()

	common.InfoLog("inventory: loaded %d hosts from %s (%s)", len(parsed), p, kind)
	return nil
}

// ---- autodetect (YAML first)

func detectInventoryFormat(b []byte) (string, []common.Host, error) {
	if hs, err := parseYAMLInventory(b); err == nil && len(hs) > 0 {
		return "yaml", hs, nil
	}
	if hs, err := parseINIInventory(b); err == nil && len(hs) > 0 {
		return "ini", hs, nil
	}
	// leniency: top-level "hosts:" only
	type flatY struct {
		Hosts map[string]map[string]any `yaml:"hosts"`
	}
	var fy flatY
	if err := yaml.Unmarshal(b, &fy); err == nil && len(fy.Hosts) > 0 {
		y := yamlInventory{}
		y.All.Hosts = fy.Hosts
		return "yaml", mapYamlToHosts(y), nil
	}
	return "", nil, errors.New("unable to parse inventory as YAML or INI")
}

// YAML: all.hosts.<name>.ansible_host
type yamlInventory struct {
	All struct {
		Hosts map[string]map[string]any `yaml:"hosts"`
	} `yaml:"all"`
}

func parseYAMLInventory(b []byte) ([]common.Host, error) {
	var y yamlInventory
	if err := yaml.Unmarshal(b, &y); err != nil {
		return nil, err
	}
	if len(y.All.Hosts) == 0 {
		return nil, errors.New("yaml: no hosts found")
	}
	return mapYamlToHosts(y), nil
}

func mapYamlToHosts(y yamlInventory) []common.Host {
	out := make([]common.Host, 0, len(y.All.Hosts))
	for name, vars := range y.All.Hosts {
		h := common.Host{Name: name, Vars: map[string]string{}}
		for k, v := range vars {
			if k == "ansible_host" {
				if s, ok := v.(string); ok {
					h.Addr = s
				}
				continue
			}
			h.Vars[k] = stringify(v)
		}

		// --- NEW: owner from vars["owner"] or DD_UI_DEFAULT_OWNER
		if o, ok := h.Vars["owner"]; ok && o != "" {
			h.Owner = o
		} else if def := common.Env("DD_UI_DEFAULT_OWNER", ""); def != "" {
			h.Owner = def
		}

		out = append(out, h)
	}
	return out
}

// Minimal INI-ish fallback: "name ansible_host=1.2.3.4 foo=bar"
func parseINIInventory(b []byte) ([]common.Host, error) {
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	var out []common.Host
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) == 0 {
			continue
		}
		h := common.Host{Name: fs[0], Vars: map[string]string{}}
		for _, f := range fs[1:] {
			kv := strings.SplitN(f, "=", 2)
			if len(kv) != 2 {
				continue
			}
			k, v := kv[0], kv[1]
			if k == "ansible_host" {
				h.Addr = v
			} else {
				h.Vars[k] = v
			}
		}

		// --- NEW: owner from vars["owner"] or DD_UI_DEFAULT_OWNER
		if o, ok := h.Vars["owner"]; ok && o != "" {
			h.Owner = o
		} else if def := common.Env("DD_UI_DEFAULT_OWNER", ""); def != "" {
			h.Owner = def
		}

		out = append(out, h)
	}
	if len(out) == 0 {
		return nil, errors.New("ini: no hosts found")
	}
	return out, nil
}

func stringify(v any) string { return fmt.Sprintf("%v", v) }
