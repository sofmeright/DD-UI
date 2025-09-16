// common/types.go - Shared types used across packages
package common

// Host represents an inventory host
type Host struct {
	Name   string            `json:"name"`
	Addr   string            `json:"addr"`           // from ansible_host
	Vars   map[string]string `json:"vars,omitempty"` // extra vars (stored as JSONB)
	Groups []string          `json:"groups,omitempty"`
	Owner  string            `json:"owner,omitempty"`
}

// RenderedService represents a rendered Docker Compose service
type RenderedService struct {
	ServiceName   string `json:"service_name"`
	ContainerName string `json:"container_name,omitempty"`
	Image         string `json:"image,omitempty"`
}