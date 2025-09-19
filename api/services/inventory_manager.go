package services

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dd-ui/common"
	"github.com/goccy/go-yaml"
)

// InventoryManager manages the Ansible inventory file as the single source of truth
type InventoryManager struct {
	path string
	data []byte // Raw YAML content to preserve formatting
	mu   sync.RWMutex
}

// Metadata structures for DD-UI specific fields
type HostMetadata struct {
	Tags         []string          `yaml:"dd_ui_tags,omitempty"`
	Description  string            `yaml:"dd_ui_description,omitempty"`
	AltName      string            `yaml:"dd_ui_alt_name,omitempty"`
	Tenant       string            `yaml:"dd_ui_tenant,omitempty"`
	AllowedUsers []string          `yaml:"dd_ui_allowed_users,omitempty"`
	Owner        string            `yaml:"dd_ui_owner,omitempty"`
	Env          map[string]string `yaml:"dd_ui_env,omitempty"` // Environment variables
}

type GroupMetadata struct {
	Tags         []string          `yaml:"dd_ui_tags,omitempty"`
	Description  string            `yaml:"dd_ui_description,omitempty"`
	AltName      string            `yaml:"dd_ui_alt_name,omitempty"`
	Tenant       string            `yaml:"dd_ui_tenant,omitempty"`
	AllowedUsers []string          `yaml:"dd_ui_allowed_users,omitempty"`
	Owner        string            `yaml:"dd_ui_owner,omitempty"`
	Env          map[string]string `yaml:"dd_ui_env,omitempty"` // Environment variables
}

// InventoryHost represents a host with all its metadata
type InventoryHost struct {
	Name        string            `json:"name"`
	Addr        string            `json:"addr"`
	Vars        map[string]any    `json:"vars,omitempty"`
	Groups      []string          `json:"groups,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Description string            `json:"description,omitempty"`
	AltName     string            `json:"alt_name,omitempty"`
	Tenant      string            `json:"tenant,omitempty"`
	AllowedUsers []string         `json:"allowed_users,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// InventoryGroup represents a group with all its metadata
type InventoryGroup struct {
	Name         string            `json:"name"`
	Vars         map[string]any    `json:"vars,omitempty"`
	Hosts        []string          `json:"hosts,omitempty"`
	Children     []string          `json:"children,omitempty"`
	ParentGroups []string          `json:"parent_groups,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Description  string            `json:"description,omitempty"`
	AltName      string            `json:"alt_name,omitempty"`
	Tenant       string            `json:"tenant,omitempty"`
	AllowedUsers []string          `json:"allowed_users,omitempty"`
	Owner        string            `json:"owner,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
}

// Ansible inventory structure
type ansibleInventory struct {
	All    *ansibleGroup            `yaml:"all,omitempty"`
	Groups map[string]*ansibleGroup `yaml:",inline"`
}

type ansibleGroup struct {
	Hosts    map[string]map[string]any `yaml:"hosts,omitempty"`
	Vars     map[string]any            `yaml:"vars,omitempty"`
	Children map[string]*ansibleGroup  `yaml:"children,omitempty"`
}

var (
	invManager     *InventoryManager
	invManagerOnce sync.Once
	ErrNotFound    = errors.New("not found")
)

// GetInventoryManager returns the singleton inventory manager
func GetInventoryManager() *InventoryManager {
	invManagerOnce.Do(func() {
		// Build path from IAC root and inventory file
		iacRoot := common.Env("DD_UI_IAC_ROOT", "/data")
		invFile := common.Env("DD_UI_INVENTORY_FILE", "")
		
		var path string
		if invFile != "" {
			// Use explicit inventory file path relative to IAC root
			path = iacRoot + "/" + invFile
			if _, err := os.Stat(path); err != nil {
				common.ErrorLog("Specified inventory file not found: %s", path)
				path = ""
			}
		}
		
		if path == "" {
			// Fall back to searching for inventory file
			path = findInventoryPath()
		}
		
		invManager = &InventoryManager{path: path}
		if path != "" && invManager.Load() == nil {
			common.InfoLog("InventoryManager: Using inventory file: %s", path)
		} else {
			common.ErrorLog("InventoryManager: Failed to load inventory from %s", path)
		}
	})
	return invManager
}

// Load reads the inventory file into memory
func (im *InventoryManager) Load() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.path == "" {
		return errors.New("no inventory path configured")
	}

	data, err := os.ReadFile(im.path)
	if err != nil {
		return fmt.Errorf("failed to read inventory: %w", err)
	}

	im.data = data
	return nil
}

// saveInternal writes data to file without acquiring lock (caller must hold lock)
func (im *InventoryManager) saveInternal() error {
	if im.path == "" {
		return errors.New("no inventory path configured")
	}

	// Ensure directory exists
	dir := filepath.Dir(im.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create backup before saving (if file exists)
	if _, err := os.Stat(im.path); err == nil {
		backupPath := im.path + ".bak"
		if origData, err := os.ReadFile(im.path); err == nil {
			os.WriteFile(backupPath, origData, 0644)
		}
	}

	return os.WriteFile(im.path, im.data, 0644)
}

// Save writes the current inventory data to file
func (im *InventoryManager) Save() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.saveInternal()
}

// Reload re-reads the inventory file
func (im *InventoryManager) Reload() error {
	return im.Load()
}

// GetHosts returns all hosts from the inventory
func (im *InventoryManager) GetHosts() ([]InventoryHost, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	// Return empty list if no data loaded
	if len(im.data) == 0 {
		common.DebugLog("InventoryManager: No inventory data loaded, returning empty hosts list")
		return []InventoryHost{}, nil
	}

	var inv ansibleInventory
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	var hosts []InventoryHost
	hostGroups := make(map[string][]string)

	// Process all groups to build host-group relationships
	im.processGroups(&inv, "", hostGroups)

	// Process hosts from 'all' group
	if inv.All != nil && inv.All.Hosts != nil {
		for name, vars := range inv.All.Hosts {
			host := im.parseHost(name, vars)
			host.Groups = hostGroups[name]
			hosts = append(hosts, host)
		}
	}

	return hosts, nil
}

// GetGroups returns all groups from the inventory
func (im *InventoryManager) GetGroups() ([]InventoryGroup, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	// Return empty list if no data loaded
	if len(im.data) == 0 {
		common.DebugLog("InventoryManager: No inventory data loaded, returning empty groups list")
		return []InventoryGroup{}, nil
	}

	var inv ansibleInventory
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	var groups []InventoryGroup
	
	// Process top-level groups
	if inv.Groups != nil {
		for name, group := range inv.Groups {
			if name != "all" && group != nil {
				g := im.parseGroup(name, group)
				groups = append(groups, g)
			}
		}
	}

	// Process children recursively
	if inv.All != nil && inv.All.Children != nil {
		for name, child := range inv.All.Children {
			if child != nil {
				g := im.parseGroup(name, child)
				groups = append(groups, g)
			}
		}
	}

	return groups, nil
}

// GetHost returns a specific host by name
func (im *InventoryManager) GetHost(name string) (*InventoryHost, error) {
	hosts, err := im.GetHosts()
	if err != nil {
		return nil, err
	}

	for _, h := range hosts {
		if h.Name == name {
			return &h, nil
		}
	}
	return nil, ErrNotFound
}

// GetGroup returns a specific group by name
func (im *InventoryManager) GetGroup(name string) (*InventoryGroup, error) {
	groups, err := im.GetGroups()
	if err != nil {
		return nil, err
	}

	for _, g := range groups {
		if g.Name == name {
			return &g, nil
		}
	}
	return nil, ErrNotFound
}

// UpdateHostMetadata updates DD-UI metadata for a host
func (im *InventoryManager) UpdateHostMetadata(name string, metadata HostMetadata) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Find the host in the inventory structure
	if all, ok := inv["all"].(map[string]any); ok {
		if hosts, ok := all["hosts"].(map[string]any); ok {
			if host, ok := hosts[name].(map[string]any); ok {
				// Update DD-UI fields
				if len(metadata.Tags) > 0 {
					host["dd_ui_tags"] = metadata.Tags
				}
				if metadata.Description != "" {
					host["dd_ui_description"] = metadata.Description
				}
				if metadata.AltName != "" {
					host["dd_ui_alt_name"] = metadata.AltName
				}
				if metadata.Tenant != "" {
					host["dd_ui_tenant"] = metadata.Tenant
				}
				if len(metadata.AllowedUsers) > 0 {
					host["dd_ui_allowed_users"] = metadata.AllowedUsers
				}
				if metadata.Owner != "" {
					host["dd_ui_owner"] = metadata.Owner
				}
				if len(metadata.Env) > 0 {
					host["dd_ui_env"] = metadata.Env
				}
			} else {
				// Host exists but has no vars yet
				hosts[name] = map[string]any{
					"ansible_host": name, // Default to hostname if no ansible_host
				}
				return im.UpdateHostMetadata(name, metadata) // Retry
			}
		}
	}

	// Marshal back to YAML preserving order as much as possible
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	return im.saveInternal()
}

// UpdateGroupMetadata updates DD-UI metadata for a group
func (im *InventoryManager) UpdateGroupMetadata(name string, metadata GroupMetadata) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Find the group in the inventory structure
	if group, ok := inv[name].(map[string]any); ok {
		if vars, ok := group["vars"].(map[string]any); ok {
			// Update DD-UI fields in vars
			if len(metadata.Tags) > 0 {
				vars["dd_ui_tags"] = metadata.Tags
			}
			if metadata.Description != "" {
				vars["dd_ui_description"] = metadata.Description
			}
			if metadata.AltName != "" {
				vars["dd_ui_alt_name"] = metadata.AltName
			}
			if metadata.Tenant != "" {
				vars["dd_ui_tenant"] = metadata.Tenant
			}
			if len(metadata.AllowedUsers) > 0 {
				vars["dd_ui_allowed_users"] = metadata.AllowedUsers
			}
			if metadata.Owner != "" {
				vars["dd_ui_owner"] = metadata.Owner
			}
			if len(metadata.Env) > 0 {
				vars["dd_ui_env"] = metadata.Env
			}
		} else {
			// Group has no vars yet, create them
			group["vars"] = map[string]any{}
			return im.UpdateGroupMetadata(name, metadata) // Retry
		}
	} else {
		return fmt.Errorf("group %s not found", name)
	}

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	return im.saveInternal()
}

// AddHostToGroup adds a host to a group
func (im *InventoryManager) AddHostToGroup(hostname, groupname string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	// First, ensure the host exists in all.hosts
	all, ok := inv["all"].(map[string]any)
	if !ok {
		return fmt.Errorf("inventory missing 'all' group")
	}
	allHosts, ok := all["hosts"].(map[string]any)
	if !ok {
		return fmt.Errorf("'all' group missing hosts section")
	}
	if _, exists := allHosts[hostname]; !exists {
		return fmt.Errorf("host %s not found in inventory", hostname)
	}

	// Find or create the group
	group, ok := inv[groupname].(map[string]any)
	if !ok {
		// Create new group
		inv[groupname] = map[string]any{
			"hosts": map[string]any{
				hostname: map[string]any{},
			},
		}
	} else {
		// Add host to existing group
		hosts, ok := group["hosts"].(map[string]any)
		if !ok {
			group["hosts"] = map[string]any{}
			hosts = group["hosts"].(map[string]any)
		}
		hosts[hostname] = map[string]any{}
	}

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	return im.saveInternal()
}

// RemoveHostFromGroup removes a host from a group
func (im *InventoryManager) RemoveHostFromGroup(hostname, groupname string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	if group, ok := inv[groupname].(map[string]any); ok {
		if hosts, ok := group["hosts"].(map[string]any); ok {
			delete(hosts, hostname)
			
			// If group is now empty, optionally remove it
			if len(hosts) == 0 && group["vars"] == nil && group["children"] == nil {
				delete(inv, groupname)
			}
		}
	}

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	return im.saveInternal()
}

// CreateGroup creates a new group with metadata
func (im *InventoryManager) CreateGroup(name string, parent string, metadata GroupMetadata) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Initialize empty inventory if no data exists
	var inv map[string]any
	if len(im.data) == 0 {
		common.InfoLog("InventoryManager: Initializing new inventory file")
		inv = map[string]any{
			"all": map[string]any{
				"hosts": map[string]any{},
				"children": map[string]any{},
			},
		}
	} else {
		if err := yaml.Unmarshal(im.data, &inv); err != nil {
			return fmt.Errorf("failed to parse inventory: %w", err)
		}
	}

	// Check if group already exists
	if _, exists := inv[name]; exists {
		return fmt.Errorf("group %s already exists", name)
	}

	// Create new group
	newGroup := map[string]any{
		"hosts": map[string]any{},
		"vars":  map[string]any{},
	}

	// Add metadata to vars
	vars := newGroup["vars"].(map[string]any)
	if len(metadata.Tags) > 0 {
		vars["dd_ui_tags"] = metadata.Tags
	}
	if metadata.Description != "" {
		vars["dd_ui_description"] = metadata.Description
	}
	if metadata.AltName != "" {
		vars["dd_ui_alt_name"] = metadata.AltName
	}
	if metadata.Tenant != "" {
		vars["dd_ui_tenant"] = metadata.Tenant
	}
	if len(metadata.AllowedUsers) > 0 {
		vars["dd_ui_allowed_users"] = metadata.AllowedUsers
	}
	if metadata.Owner != "" {
		vars["dd_ui_owner"] = metadata.Owner
	}
	if len(metadata.Env) > 0 {
		vars["dd_ui_env"] = metadata.Env
	}

	inv[name] = newGroup

	// If parent specified, add as child reference (not the full group)
	if parent != "" && parent != "all" {
		if parentGroup, ok := inv[parent].(map[string]any); ok {
			children, ok := parentGroup["children"].(map[string]any)
			if !ok {
				parentGroup["children"] = map[string]any{}
				children = parentGroup["children"].(map[string]any)
			}
			// Just add empty map as reference - the actual group is defined at top level
			children[name] = map[string]any{}
		}
	}

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	return im.saveInternal()
}

// CreateHost adds a new host to the inventory (in all.hosts)
func (im *InventoryManager) CreateHost(name, ansibleHost string, metadata HostMetadata) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Validate host name - no spaces or special characters
	if name == "" || strings.Contains(name, " ") {
		return fmt.Errorf("invalid host name: must not be empty or contain spaces")
	}

	// Normalize description to prevent YAML issues
	if metadata.Description != "" {
		metadata.Description = strings.ReplaceAll(metadata.Description, "\r\n", "\n")
		metadata.Description = strings.ReplaceAll(metadata.Description, "\r", "\n")
	}

	// Parse existing inventory
	var inv map[string]any
	if len(im.data) == 0 {
		common.InfoLog("InventoryManager: Initializing new inventory file for host creation")
		inv = map[string]any{
			"all": map[string]any{
				"hosts": map[string]any{},
			},
		}
	} else {
		if err := yaml.Unmarshal(im.data, &inv); err != nil {
			return fmt.Errorf("failed to parse inventory: %w", err)
		}
	}

	// Ensure all group exists
	all, ok := inv["all"].(map[string]any)
	if !ok {
		all = map[string]any{
			"hosts": map[string]any{},
		}
		inv["all"] = all
	}

	// Ensure hosts section exists
	hosts, ok := all["hosts"].(map[string]any)
	if !ok {
		hosts = map[string]any{}
		all["hosts"] = hosts
	}

	// Check if host already exists
	if _, exists := hosts[name]; exists {
		return fmt.Errorf("host %s already exists", name)
	}

	// Create new host entry
	newHost := map[string]any{
		"ansible_host": ansibleHost,
	}

	// Add DD-UI metadata fields
	if len(metadata.Tags) > 0 {
		newHost["dd_ui_tags"] = metadata.Tags
	}
	if metadata.Description != "" {
		newHost["dd_ui_description"] = metadata.Description
	}
	if metadata.AltName != "" {
		newHost["dd_ui_alt_name"] = metadata.AltName
	}
	if metadata.Tenant != "" {
		newHost["dd_ui_tenant"] = metadata.Tenant
	}
	if len(metadata.AllowedUsers) > 0 {
		newHost["dd_ui_allowed_users"] = metadata.AllowedUsers
	}
	if metadata.Owner != "" {
		newHost["dd_ui_owner"] = metadata.Owner
	} else if def := common.Env("DD_UI_DEFAULT_OWNER", ""); def != "" {
		newHost["dd_ui_owner"] = def
	}
	if len(metadata.Env) > 0 {
		newHost["dd_ui_env"] = metadata.Env
	}

	// Add host to inventory
	hosts[name] = newHost

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	common.InfoLog("InventoryManager: Created host %s with IP %s", name, ansibleHost)
	return im.saveInternal()
}

// UpdateHost updates an existing host in the inventory
func (im *InventoryManager) UpdateHost(name, ansibleHost string, metadata HostMetadata) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Normalize description to prevent YAML issues
	if metadata.Description != "" {
		metadata.Description = strings.ReplaceAll(metadata.Description, "\r\n", "\n")
		metadata.Description = strings.ReplaceAll(metadata.Description, "\r", "\n")
	}

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Find the host in all.hosts
	all, ok := inv["all"].(map[string]any)
	if !ok {
		return fmt.Errorf("inventory missing 'all' group")
	}

	hosts, ok := all["hosts"].(map[string]any)
	if !ok {
		return fmt.Errorf("'all' group missing hosts section")
	}

	host, ok := hosts[name].(map[string]any)
	if !ok {
		return fmt.Errorf("host %s not found", name)
	}

	// Update ansible_host if provided
	if ansibleHost != "" {
		host["ansible_host"] = ansibleHost
	}

	// Update DD-UI metadata fields
	// Clear existing DD-UI fields first to handle removals
	keysToRemove := []string{}
	for k := range host {
		if strings.HasPrefix(k, "dd_ui_") {
			keysToRemove = append(keysToRemove, k)
		}
	}
	for _, k := range keysToRemove {
		delete(host, k)
	}

	// Add updated metadata
	if len(metadata.Tags) > 0 {
		host["dd_ui_tags"] = metadata.Tags
	}
	if metadata.Description != "" {
		host["dd_ui_description"] = metadata.Description
	}
	if metadata.AltName != "" {
		host["dd_ui_alt_name"] = metadata.AltName
	}
	if metadata.Tenant != "" {
		host["dd_ui_tenant"] = metadata.Tenant
	}
	if len(metadata.AllowedUsers) > 0 {
		host["dd_ui_allowed_users"] = metadata.AllowedUsers
	}
	if metadata.Owner != "" {
		host["dd_ui_owner"] = metadata.Owner
	}
	if len(metadata.Env) > 0 {
		host["dd_ui_env"] = metadata.Env
	}

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	common.InfoLog("InventoryManager: Updated host %s", name)
	return im.saveInternal()
}

// DeleteHost removes a host from the inventory completely
func (im *InventoryManager) DeleteHost(name string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Remove from all.hosts
	if all, ok := inv["all"].(map[string]any); ok {
		if hosts, ok := all["hosts"].(map[string]any); ok {
			if _, exists := hosts[name]; !exists {
				return fmt.Errorf("host %s not found", name)
			}
			delete(hosts, name)
		}
	}

	// Remove from all groups
	for groupName, group := range inv {
		if groupName == "all" {
			continue // Already handled above
		}
		
		if g, ok := group.(map[string]any); ok {
			if hosts, ok := g["hosts"].(map[string]any); ok {
				delete(hosts, name)
			}
		}
	}

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	common.InfoLog("InventoryManager: Deleted host %s", name)
	return im.saveInternal()
}

// DeleteGroup removes a group from inventory
func (im *InventoryManager) DeleteGroup(name string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Remove group
	delete(inv, name)

	// Remove from any parent's children
	for _, group := range inv {
		if g, ok := group.(map[string]any); ok {
			if children, ok := g["children"].(map[string]any); ok {
				delete(children, name)
			}
		}
	}

	// Marshal back to YAML
	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}

	im.data = data
	return im.saveInternal()
}

// Helper functions

func (im *InventoryManager) parseHost(name string, vars map[string]any) InventoryHost {
	host := InventoryHost{
		Name: name,
		Vars: make(map[string]any),
		Env:  make(map[string]string),
	}

	for k, v := range vars {
		switch k {
		case "ansible_host":
			if s, ok := v.(string); ok {
				host.Addr = s
			}
		case "dd_ui_tags":
			if tags, ok := v.([]any); ok {
				for _, t := range tags {
					if s, ok := t.(string); ok {
						host.Tags = append(host.Tags, s)
					}
				}
			}
		case "dd_ui_description":
			if s, ok := v.(string); ok {
				host.Description = s
			}
		case "dd_ui_alt_name":
			if s, ok := v.(string); ok {
				host.AltName = s
			}
		case "dd_ui_tenant":
			if s, ok := v.(string); ok {
				host.Tenant = s
			}
		case "dd_ui_allowed_users":
			if users, ok := v.([]any); ok {
				for _, u := range users {
					if s, ok := u.(string); ok {
						host.AllowedUsers = append(host.AllowedUsers, s)
					}
				}
			}
		case "dd_ui_owner":
			if s, ok := v.(string); ok {
				host.Owner = s
			}
		case "dd_ui_env":
			if env, ok := v.(map[string]any); ok {
				for ek, ev := range env {
					if s, ok := ev.(string); ok {
						host.Env[ek] = s
					}
				}
			}
		default:
			// Store other vars
			host.Vars[k] = v
		}
	}

	// Default owner if not set
	if host.Owner == "" {
		if def := common.Env("DD_UI_DEFAULT_OWNER", ""); def != "" {
			host.Owner = def
		}
	}

	return host
}

func (im *InventoryManager) parseGroup(name string, group *ansibleGroup) InventoryGroup {
	g := InventoryGroup{
		Name:  name,
		Vars:  make(map[string]any),
		Hosts: []string{},
		Env:   make(map[string]string),
	}

	// Process hosts
	if group.Hosts != nil {
		for hostname := range group.Hosts {
			g.Hosts = append(g.Hosts, hostname)
		}
	}

	// Process children
	if group.Children != nil {
		for childname := range group.Children {
			g.Children = append(g.Children, childname)
		}
	}

	// Process vars
	if group.Vars != nil {
		for k, v := range group.Vars {
			switch k {
			case "dd_ui_tags":
				if tags, ok := v.([]any); ok {
					for _, t := range tags {
						if s, ok := t.(string); ok {
							g.Tags = append(g.Tags, s)
						}
					}
				}
			case "dd_ui_description":
				if s, ok := v.(string); ok {
					g.Description = s
				}
			case "dd_ui_alt_name":
				if s, ok := v.(string); ok {
					g.AltName = s
				}
			case "dd_ui_tenant":
				if s, ok := v.(string); ok {
					g.Tenant = s
				}
			case "dd_ui_allowed_users":
				if users, ok := v.([]any); ok {
					for _, u := range users {
						if s, ok := u.(string); ok {
							g.AllowedUsers = append(g.AllowedUsers, s)
						}
					}
				}
			case "dd_ui_owner":
				if s, ok := v.(string); ok {
					g.Owner = s
				}
			case "dd_ui_env":
				if env, ok := v.(map[string]any); ok {
					for ek, ev := range env {
						if s, ok := ev.(string); ok {
							g.Env[ek] = s
						}
					}
				}
			default:
				// Store other vars
				g.Vars[k] = v
			}
		}
	}

	// Default owner if not set
	if g.Owner == "" {
		if def := common.Env("DD_UI_DEFAULT_OWNER", ""); def != "" {
			g.Owner = def
		}
	}

	return g
}

func (im *InventoryManager) processGroups(inv *ansibleInventory, parentName string, hostGroups map[string][]string) {
	// Process top-level groups
	for name, group := range inv.Groups {
		if name != "all" && group != nil && group.Hosts != nil {
			for hostname := range group.Hosts {
				hostGroups[hostname] = append(hostGroups[hostname], name)
			}
		}
	}

	// Process 'all' group children
	if inv.All != nil && inv.All.Children != nil {
		im.processChildGroups(inv.All.Children, hostGroups)
	}
}

func (im *InventoryManager) processChildGroups(children map[string]*ansibleGroup, hostGroups map[string][]string) {
	for name, group := range children {
		if group != nil && group.Hosts != nil {
			for hostname := range group.Hosts {
				hostGroups[hostname] = append(hostGroups[hostname], name)
			}
		}
		// Recursively process nested children
		if group != nil && group.Children != nil {
			im.processChildGroups(group.Children, hostGroups)
		}
	}
}

// GetRawYAML returns the raw YAML content for direct editing
func (im *InventoryManager) GetRawYAML() (string, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return string(im.data), nil
}

// SetRawYAML updates the raw YAML content (validates before saving)
func (im *InventoryManager) SetRawYAML(content string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Validate it's valid YAML
	var test map[string]any
	if err := yaml.Unmarshal([]byte(content), &test); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	im.data = []byte(content)
	return im.Save()
}

// ExportForAnsible exports inventory without DD-UI fields for pure Ansible use
func (im *InventoryManager) ExportForAnsible(w io.Writer) error {
	im.mu.RLock()
	defer im.mu.RUnlock()

	var inv map[string]any
	if err := yaml.Unmarshal(im.data, &inv); err != nil {
		return fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Remove all dd_ui_* fields recursively
	cleanInventory := im.removeDDUIFields(inv)

	// Write clean inventory
	encoder := yaml.NewEncoder(w)
	defer encoder.Close()
	return encoder.Encode(cleanInventory)
}

func (im *InventoryManager) removeDDUIFields(data any) any {
	switch v := data.(type) {
	case map[string]any:
		clean := make(map[string]any)
		for k, val := range v {
			if !strings.HasPrefix(k, "dd_ui_") {
				clean[k] = im.removeDDUIFields(val)
			}
		}
		return clean
	case []any:
		clean := make([]any, len(v))
		for i, val := range v {
			clean[i] = im.removeDDUIFields(val)
		}
		return clean
	default:
		return v
	}
}