package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dd-ui/common"
	"dd-ui/services"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
)

// errorLog logs error messages
func errorLog(format string, v ...interface{}) {
	common.ErrorLog(format, v...)
}

// infoLog logs info messages
func infoLog(format string, v ...interface{}) {
	common.InfoLog(format, v...)
}

// parseStringArray converts PostgreSQL array to []string
func parseStringArray(arr interface{}) []string {
	if arr == nil {
		return []string{}
	}
	switch v := arr.(type) {
	case []byte:
		var result []string
		if err := json.Unmarshal(v, &result); err == nil {
			return result
		}
	case pq.StringArray:
		return []string(v)
	case []string:
		return v
	}
	return []string{}
}

// Group represents a host group
type Group struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	ParentID    *int64            `json:"parent_id,omitempty"`
	Vars        map[string]string `json:"vars,omitempty"`
	Owner       string            `json:"owner"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	
	// Computed fields
	HostCount   int      `json:"host_count"`
	StackCount  int      `json:"stack_count"`
	Children    []Group  `json:"children,omitempty"`
	HostIDs     []int64  `json:"host_ids,omitempty"`
}

// GroupMembership represents a host's membership in a group
type GroupMembership struct {
	GroupID       int64  `json:"group_id"`
	GroupName     string `json:"group_name"`
	DirectMember  bool   `json:"direct_member"`
	InheritedFrom *int64 `json:"inherited_from,omitempty"`
}

// SetupGroupRoutes configures all group-related routes
func SetupGroupRoutes(r chi.Router) {
	r.Route("/groups", func(r chi.Router) {
		r.Get("/", listGroups)
		r.Post("/", createGroup)
		r.Get("/tree", getGroupTree)
		
		r.Route("/{groupName}", func(r chi.Router) {
			r.Get("/", getGroup)
			r.Put("/", updateGroup)
			r.Delete("/", deleteGroup)
			
			// Host membership
			r.Get("/hosts", getGroupHosts)
			r.Post("/hosts", addHostsToGroup)
			r.Delete("/hosts/{hostname}", removeHostFromGroup)
			
			// Stack count
			r.Get("/stacks/count", getGroupStackCount)
		})
	})
}

// listGroups returns all groups from inventory file
func listGroups(w http.ResponseWriter, r *http.Request) {
	// Use inventory manager as source of truth
	invMgr := services.GetInventoryManager()
	groups, err := invMgr.GetGroups()
	if err != nil {
		errorLog("Failed to get groups from inventory: %v", err)
		http.Error(w, "Failed to get groups", http.StatusInternalServerError)
		return
	}
	
	// Convert to API format
	apiGroups := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		apiGroups = append(apiGroups, map[string]interface{}{
			"name":          g.Name,
			"description":   g.Description,
			"tags":          g.Tags,
			"alt_name":      g.AltName,
			"tenant":        g.Tenant,
			"allowed_users": g.AllowedUsers,
			"owner":         g.Owner,
			"env":           g.Env,
			"hosts":         g.Hosts,
			"children":      g.Children,
			"host_count":    len(g.Hosts),
		})
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiGroups)
}

// getGroupTree returns groups in a tree structure from inventory
func getGroupTree(w http.ResponseWriter, r *http.Request) {
	// Use inventory manager as source of truth
	invMgr := services.GetInventoryManager()
	groups, err := invMgr.GetGroups()
	if err != nil {
		errorLog("Failed to get groups from inventory: %v", err)
		http.Error(w, "Failed to get groups", http.StatusInternalServerError)
		return
	}
	
	// Build tree structure from groups
	groupMap := make(map[string]*services.InventoryGroup)
	var rootGroups []services.InventoryGroup
	
	// First pass: index all groups
	for i := range groups {
		g := groups[i]
		groupMap[g.Name] = &g
	}
	
	// Second pass: build parent-child relationships
	for _, g := range groups {
		hasParent := false
		// Check if this group is a child of any other group
		for _, potential := range groups {
			for _, child := range potential.Children {
				if child == g.Name {
					hasParent = true
					break
				}
			}
			if hasParent {
				break
			}
		}
		
		if !hasParent {
			rootGroups = append(rootGroups, g)
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rootGroups)
}

// getGroup returns a specific group with its details from inventory
func getGroup(w http.ResponseWriter, r *http.Request) {
	groupName := chi.URLParam(r, "groupName")
	
	// Use inventory manager to get group details
	invMgr := services.GetInventoryManager()
	groups, err := invMgr.GetGroups()
	if err != nil {
		errorLog("Failed to get groups from inventory: %v", err)
		http.Error(w, "Failed to get groups", http.StatusInternalServerError)
		return
	}
	
	// Find the specific group
	for _, g := range groups {
		if g.Name == groupName {
			// Return the group with all its details
			result := map[string]interface{}{
				"name":        g.Name,
				"description": g.Description,
				"tags":        g.Tags,
				"alt_name":    g.AltName,
				"tenant":      g.Tenant,
				"allowed_users": g.AllowedUsers,
				"owner":       g.Owner,
				"env":         g.Env,
				"hosts":       g.Hosts,
				"children":    g.Children,
				"host_count":  len(g.Hosts),
				"vars":        g.Vars,
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
			return
		}
	}
	
	http.Error(w, "Group not found", http.StatusNotFound)
}

// createGroup creates a new group in the inventory file
func createGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string            `json:"name"`
		Description  string            `json:"description"`
		Tags         []string          `json:"tags"`
		Parent       string            `json:"parent"`
		AltName      string            `json:"alt_name"`
		Tenant       string            `json:"tenant"`
		AllowedUsers []string          `json:"allowed_users"`
		Owner        string            `json:"owner"`
		Env          map[string]string `json:"env"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	
	if req.Owner == "" {
		req.Owner = "unassigned"
	}
	
	// Use inventory manager to create group
	invMgr := services.GetInventoryManager()
	metadata := services.GroupMetadata{
		Tags:         req.Tags,
		Description:  req.Description,
		AltName:      req.AltName,
		Tenant:       req.Tenant,
		AllowedUsers: req.AllowedUsers,
		Owner:        req.Owner,
		Env:          req.Env,
	}
	
	err := invMgr.CreateGroup(req.Name, req.Parent, metadata)
	if err != nil {
		errorLog("Failed to create group: %v", err)
		http.Error(w, "Failed to create group", http.StatusInternalServerError)
		return
	}
	
	infoLog("Group %s created successfully", req.Name)
	
	// Return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":    req.Name,
		"message": "Group created successfully",
	})
}

// updateGroup updates an existing group
func updateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	groupID, _ := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Tags        []string          `json:"tags"`
		ParentID    *int64            `json:"parent_id"`
		Vars        map[string]string `json:"vars"`
		Owner       string            `json:"owner"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	
	varsJSON, _ := json.Marshal(req.Vars)
	
	_, err := common.DB.Exec(ctx, `
		UPDATE groups 
		SET name = $2, description = $3, tags = $4, parent_id = $5, vars = $6, owner = $7
		WHERE id = $1
	`, groupID, req.Name, req.Description, req.Tags, req.ParentID, varsJSON, req.Owner)
	
	if err != nil {
		errorLog("Failed to update group: %v", err)
		http.Error(w, "Failed to update group", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Group updated successfully",
	})
}

// deleteGroup deletes a group
func deleteGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	groupID, _ := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	
	_, err := common.DB.Exec(ctx, "DELETE FROM groups WHERE id = $1", groupID)
	if err != nil {
		errorLog("Failed to delete group: %v", err)
		http.Error(w, "Failed to delete group", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Group deleted successfully",
	})
}

// getGroupHosts returns all hosts in a group from inventory
func getGroupHosts(w http.ResponseWriter, r *http.Request) {
	groupName := chi.URLParam(r, "groupName")
	
	// Use inventory manager to get group details
	invMgr := services.GetInventoryManager()
	groups, err := invMgr.GetGroups()
	if err != nil {
		errorLog("Failed to get groups from inventory: %v", err)
		http.Error(w, "Failed to get groups", http.StatusInternalServerError)
		return
	}
	
	// Find the specific group
	var targetGroup *services.InventoryGroup
	for _, g := range groups {
		if g.Name == groupName {
			targetGroup = &g
			break
		}
	}
	
	if targetGroup == nil {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}
	
	// Get all hosts to get their details
	allHosts, err := invMgr.GetHosts()
	if err != nil {
		errorLog("Failed to get hosts from inventory: %v", err)
		http.Error(w, "Failed to get hosts", http.StatusInternalServerError)
		return
	}
	
	// Build list of hosts in this group
	var groupHosts []map[string]interface{}
	for _, hostname := range targetGroup.Hosts {
		for _, host := range allHosts {
			if host.Name == hostname {
				groupHosts = append(groupHosts, map[string]interface{}{
					"name":    host.Name,
					"addr":    host.Addr,
					"address": host.Addr, // Compatibility alias
					"vars":    host.Vars,
					"groups":  host.Groups,
					"tags":    host.Tags,
					"owner":   host.Owner,
				})
				break
			}
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groupHosts)
}

// addHostsToGroup adds hosts to a group in the inventory
func addHostsToGroup(w http.ResponseWriter, r *http.Request) {
	groupName := chi.URLParam(r, "groupName")
	
	var req struct {
		Hosts []string `json:"hosts"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	
	invMgr := services.GetInventoryManager()
	
	for _, hostname := range req.Hosts {
		err := invMgr.AddHostToGroup(hostname, groupName)
		if err != nil {
			errorLog("Failed to add host %s to group: %v", hostname, err)
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Hosts added to group successfully",
	})
}

// removeHostFromGroup removes a host from a group in inventory
func removeHostFromGroup(w http.ResponseWriter, r *http.Request) {
	groupName := chi.URLParam(r, "groupName")
	hostname := chi.URLParam(r, "hostname")
	
	invMgr := services.GetInventoryManager()
	err := invMgr.RemoveHostFromGroup(hostname, groupName)
	
	if err != nil {
		errorLog("Failed to remove host from group: %v", err)
		http.Error(w, "Failed to remove host from group", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Host removed from group successfully",
	})
}

// getGroupStackCount returns the count of stacks for a group
func getGroupStackCount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	groupID, _ := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	
	// Get group name first
	var groupName string
	err := common.DB.QueryRow(ctx, "SELECT name FROM groups WHERE id = $1", groupID).Scan(&groupName)
	if err != nil {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}
	
	// Count stacks for this group
	var count int
	err = common.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM iac_stacks 
		WHERE scope_kind = 'group' AND scope_name = $1
	`, groupName).Scan(&count)
	
	if err != nil {
		errorLog("Failed to get stack count: %v", err)
		count = 0
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"count": count,
	})
}