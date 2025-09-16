package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/services"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// LogEntry represents a single log entry
type LogEntry struct {
	ID          int64             `json:"id,omitempty"`
	Timestamp   string            `json:"timestamp"`
	HostName    string            `json:"hostname"`
	StackName   string            `json:"stack_name,omitempty"`
	ServiceName string            `json:"service_name"`
	ContainerID string            `json:"container_id"`
	ContainerName string          `json:"container_name,omitempty"`
	Level       string            `json:"level"`
	Source      string            `json:"source"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// LogFilter represents filtering options for logs
type LogFilter struct {
	HostNames    []string  `json:"hostnames,omitempty"`
	StackNames   []string  `json:"stacks,omitempty"`
	ServiceNames []string  `json:"services,omitempty"`
	Containers   []string  `json:"containers,omitempty"`
	Levels       []string  `json:"levels,omitempty"`
	Since        time.Time `json:"since,omitempty"`
	Until        time.Time `json:"until,omitempty"`
	Search       string    `json:"search,omitempty"`
	Limit        int       `json:"limit,omitempty"`
	Follow       bool      `json:"follow,omitempty"`
}

var (
	// Global log subscribers
	logSubscribers = make(map[string]chan LogEntry)
	subMutex       sync.RWMutex
)

// HandleLogStream handles SSE streaming of logs
func HandleLogStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Parse filters from query parameters
	filter := parseLogFilters(r)
	
	common.DebugLog("Log stream request with filter: %+v", filter)

	// Create a unique subscriber ID
	subID := fmt.Sprintf("%d", time.Now().UnixNano())
	logChan := make(chan LogEntry, 100)
	
	// Register subscriber
	subMutex.Lock()
	logSubscribers[subID] = logChan
	subMutex.Unlock()
	
	// Cleanup on disconnect
	defer func() {
		subMutex.Lock()
		delete(logSubscribers, subID)
		subMutex.Unlock()
		close(logChan)
		common.DebugLog("Log stream subscriber %s disconnected", subID)
	}()

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"message\":\"Connected to log stream\"}\n\n")
	w.(http.Flusher).Flush()

	// If not following, send historical logs and return
	if !filter.Follow {
		sendHistoricalLogs(w, filter)
		return
	}

	// Start collecting logs from all hosts
	ctx := r.Context()
	go collectLogsFromHosts(ctx, filter)

	// Stream logs to client
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send keepalive
			fmt.Fprintf(w, ": keepalive\n\n")
			w.(http.Flusher).Flush()
		case log := <-logChan:
			// Apply filters
			if !matchesFilter(log, filter) {
				continue
			}
			
			// Send log entry
			data, err := json.Marshal(log)
			if err != nil {
				common.ErrorLog("Failed to marshal log entry: %v", err)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			w.(http.Flusher).Flush()
		}
	}
}

// collectLogsFromHosts collects logs from all Docker hosts
func collectLogsFromHosts(ctx context.Context, filter LogFilter) {
	hosts := services.GetHosts()
	
	var wg sync.WaitGroup
	for _, host := range hosts {
		// Check if host is in filter
		if len(filter.HostNames) > 0 && !contains(filter.HostNames, host.Name) {
			continue
		}
		
		wg.Add(1)
		go func(h common.Host) {
			defer wg.Done()
			collectHostLogs(ctx, h, filter)
		}(host)
	}
	
	wg.Wait()
}

// collectHostLogs collects logs from containers on a specific host
func collectHostLogs(ctx context.Context, host common.Host, filter LogFilter) {
	common.DebugLog("Starting log collection for host %s", host.Name)
	
	// Create Docker client for this host
	hostRow := database.HostRow{Name: host.Name, Addr: host.Addr, Vars: host.Vars}
	cli, err := services.DockerClientForHost(hostRow)
	if err != nil {
		common.ErrorLog("Failed to create Docker client for %s: %v", host.Name, err)
		return
	}
	defer cli.Close()

	// List all containers
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		common.ErrorLog("Failed to list containers on %s: %v", host.Name, err)
		return
	}

	// Start log collection for each container
	for _, cnt := range containers {
		// Check container filter
		if len(filter.Containers) > 0 {
			containerName := strings.TrimPrefix(cnt.Names[0], "/")
			if !contains(filter.Containers, containerName) {
				continue
			}
		}

		// Get stack name from labels
		stackName := cnt.Labels["com.docker.compose.project"]
		if len(filter.StackNames) > 0 && !contains(filter.StackNames, stackName) {
			continue
		}

		// Start streaming logs for this container
		go streamContainerLogs(ctx, cli, host.Name, cnt, stackName)
	}
}

// streamContainerLogs streams logs from a single container
func streamContainerLogs(ctx context.Context, cli *client.Client, hostName string, cnt types.Container, stackName string) {
	containerName := strings.TrimPrefix(cnt.Names[0], "/")
	serviceName := cnt.Labels["com.docker.compose.service"]
	if serviceName == "" {
		serviceName = containerName
	}

	common.DebugLog("Starting log stream for container %s on host %s", containerName, hostName)

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Tail:       "50", // Start with last 50 lines
	}

	reader, err := cli.ContainerLogs(ctx, cnt.ID, options)
	if err != nil {
		common.ErrorLog("Failed to get logs for container %s: %v", containerName, err)
		return
	}
	defer reader.Close()

	// Read and parse logs
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			// Context canceled is expected when client disconnects
			common.DebugLog("Log stream context canceled for container %s on host %s", containerName, hostName)
			return
		default:
			n, err := reader.Read(buf)
			if err != nil {
				if err == io.EOF {
					// EOF is normal when container stops or logs end
					common.DebugLog("Log stream ended for container %s on host %s", containerName, hostName)
				} else if ctx.Err() != nil {
					// Context was canceled - this is expected, don't log as error
					common.DebugLog("Log stream canceled for container %s on host %s", containerName, hostName)
				} else {
					// This is an actual error
					common.ErrorLog("Error reading logs from %s on host %s: %v", containerName, hostName, err)
				}
				return
			}

			if n > 0 {
				// Parse Docker log format (first 8 bytes are header)
				if n > 8 {
					message := string(buf[8:n])
					
					// Parse timestamp and message
					parts := strings.SplitN(message, " ", 2)
					timestamp := time.Now().Format(time.RFC3339)
					logMessage := message
					
					if len(parts) >= 2 {
						// Try to parse timestamp
						if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
							timestamp = t.Format(time.RFC3339)
							logMessage = parts[1]
						}
					}

					// Detect log level from message
					level := detectLogLevel(logMessage)

					// Create log entry
					entry := LogEntry{
						Timestamp:     timestamp,
						HostName:      hostName,
						StackName:     stackName,
						ServiceName:   serviceName,
						ContainerID:   cnt.ID[:12],
						ContainerName: containerName,
						Level:         level,
						Source:        "stdout",
						Message:       strings.TrimSpace(logMessage),
						Labels:        cnt.Labels,
					}

					// Broadcast to all subscribers
					broadcastLog(entry)
				}
			}
		}
	}
}

// detectLogLevel attempts to detect the log level from the message
func detectLogLevel(message string) string {
	msgLower := strings.ToLower(message)
	
	if strings.Contains(msgLower, "error") || strings.Contains(msgLower, "fatal") || strings.Contains(msgLower, "panic") {
		return "ERROR"
	}
	if strings.Contains(msgLower, "warn") || strings.Contains(msgLower, "warning") {
		return "WARN"
	}
	if strings.Contains(msgLower, "debug") || strings.Contains(msgLower, "trace") {
		return "DEBUG"
	}
	return "INFO"
}

// broadcastLog sends a log entry to all subscribers
func broadcastLog(entry LogEntry) {
	subMutex.RLock()
	defer subMutex.RUnlock()
	
	for _, ch := range logSubscribers {
		select {
		case ch <- entry:
		default:
			// Channel full, skip
		}
	}
}

// matchesFilter checks if a log entry matches the given filter
func matchesFilter(entry LogEntry, filter LogFilter) bool {
	// Check levels
	if len(filter.Levels) > 0 && !contains(filter.Levels, entry.Level) {
		return false
	}
	
	// Check search
	if filter.Search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(filter.Search)) {
		return false
	}
	
	// Check hosts
	if len(filter.HostNames) > 0 && !contains(filter.HostNames, entry.HostName) {
		return false
	}
	
	// Check stacks
	if len(filter.StackNames) > 0 && !contains(filter.StackNames, entry.StackName) {
		return false
	}
	
	// Check containers
	if len(filter.Containers) > 0 && !contains(filter.Containers, entry.ContainerName) {
		return false
	}
	
	return true
}

// parseLogFilters parses filter parameters from request
func parseLogFilters(r *http.Request) LogFilter {
	filter := LogFilter{
		Follow: r.URL.Query().Get("follow") == "true",
		Limit:  100,
	}
	
	// Parse comma-separated values
	if hosts := r.URL.Query().Get("hostnames"); hosts != "" {
		filter.HostNames = strings.Split(hosts, ",")
	}
	
	if stacks := r.URL.Query().Get("stacks"); stacks != "" {
		filter.StackNames = strings.Split(stacks, ",")
	}
	
	if containers := r.URL.Query().Get("containers"); containers != "" {
		filter.Containers = strings.Split(containers, ",")
	}
	
	if levels := r.URL.Query().Get("levels"); levels != "" {
		filter.Levels = strings.Split(levels, ",")
	}
	
	filter.Search = r.URL.Query().Get("search")
	
	return filter
}

// sendHistoricalLogs sends historical logs from the database
func sendHistoricalLogs(w http.ResponseWriter, filter LogFilter) {
	ctx := context.Background()
	
	// Query historical logs from database
	query := `
		SELECT id, timestamp, hostname, stack_name, service_name, 
		       container_id, level, source, message
		FROM container_logs
		WHERE timestamp > NOW() - INTERVAL '1 hour'
	`
	
	rows, err := common.DB.Query(ctx, query)
	if err != nil {
		common.ErrorLog("Failed to query historical logs: %v", err)
		return
	}
	defer rows.Close()
	
	count := 0
	for rows.Next() {
		var entry LogEntry
		err := rows.Scan(
			&entry.ID,
			&entry.Timestamp,
			&entry.HostName,
			&entry.StackName,
			&entry.ServiceName,
			&entry.ContainerID,
			&entry.Level,
			&entry.Source,
			&entry.Message,
		)
		if err != nil {
			common.ErrorLog("Failed to scan log row: %v", err)
			continue
		}
		
		if matchesFilter(entry, filter) {
			data, _ := json.Marshal(entry)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			count++
			
			if count >= filter.Limit {
				break
			}
		}
	}
	
	w.(http.Flusher).Flush()
	common.DebugLog("Sent %d historical log entries", count)
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// HandleGetLogSources returns available log sources (hosts, stacks, containers)
func HandleGetLogSources(w http.ResponseWriter, r *http.Request) {
	sources := struct {
		Hosts      []string `json:"hosts"`
		Stacks     []string `json:"stacks"`
		Containers []struct {
			Name  string `json:"name"`
			Host  string `json:"host"`
			Stack string `json:"stack,omitempty"`
		} `json:"containers"`
	}{}

	// Get hosts
	hosts := services.GetHosts()
	for _, h := range hosts {
		sources.Hosts = append(sources.Hosts, h.Name)
	}

	// Get containers from all hosts
	ctx := context.Background()
	for _, host := range hosts {
		hostRow := database.HostRow{Name: host.Name, Addr: host.Addr, Vars: host.Vars}
		cli, err := services.DockerClientForHost(hostRow)
		if err != nil {
			continue
		}
		defer cli.Close()

		containers, err := cli.ContainerList(ctx, container.ListOptions{All: false})
		if err != nil {
			continue
		}

		for _, cnt := range containers {
			containerName := strings.TrimPrefix(cnt.Names[0], "/")
			stackName := cnt.Labels["com.docker.compose.project"]
			
			sources.Containers = append(sources.Containers, struct {
				Name  string `json:"name"`
				Host  string `json:"host"`
				Stack string `json:"stack,omitempty"`
			}{
				Name:  containerName,
				Host:  host.Name,
				Stack: stackName,
			})
			
			// Add unique stack names
			if stackName != "" && !contains(sources.Stacks, stackName) {
				sources.Stacks = append(sources.Stacks, stackName)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sources)
}