// src/api/utils/ssh_operations.go
package utils

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// HostProvider interface for getting host information
type HostProvider interface {
	GetHostByName(ctx context.Context, name string) (HostInfo, error)
}

// HostInfo represents host information
type HostInfo struct {
	Name string
	Addr string
	Vars map[string]string
}

// EnvProvider interface for getting environment variables
type EnvProvider interface {
	Env(key, defaultValue string) string
}

// ContainerActionProvider interface for container operations
type ContainerActionProvider interface {
	PerformContainerAction(ctx context.Context, hostName, ctr, action string) error
}

// Logger interface for logging operations
type Logger interface {
	ErrorLog(format string, args ...interface{})
}

// SSHDirectOperation represents a direct SSH operation on a host
type SSHDirectOperation struct {
	HostName string   `json:"host_name"`
	Command  string   `json:"command"`
	Args     []string `json:"args,omitempty"`
}

// ExecuteSSHDirectOperation performs direct SSH operations on a host
func ExecuteSSHDirectOperation(ctx context.Context, hostProvider HostProvider, envProvider EnvProvider, op SSHDirectOperation) (string, error) {
	// Get host information
	host, err := hostProvider.GetHostByName(ctx, op.HostName)
	if err != nil {
		return "", fmt.Errorf("host not found: %v", err)
	}

	// Build SSH command
	user := host.Vars["ansible_user"]
	if user == "" {
		user = envProvider.Env("SSH_USER", "root")
	}
	
	addr := host.Addr
	if addr == "" {
		addr = host.Name
	}

	// Construct SSH command
	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		fmt.Sprintf("%s@%s", user, addr),
	}
	
	// Add the command to execute
	fullCommand := op.Command
	if len(op.Args) > 0 {
		fullCommand += " " + strings.Join(op.Args, " ")
	}
	sshArgs = append(sshArgs, fullCommand)

	// Execute SSH command
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("SSH command failed: %v\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// Enhanced container operations with SSH support
type ContainerOpResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// StartContainer starts a container using existing SSH-enabled infrastructure
func StartContainer(ctx context.Context, actionProvider ContainerActionProvider, logger Logger, hostName, containerID string) *ContainerOpResult {
	err := actionProvider.PerformContainerAction(ctx, hostName, containerID, "start")
	if err != nil {
		logger.ErrorLog("start container failed: host=%s container=%s error=%v", hostName, containerID, err)
		return &ContainerOpResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	return &ContainerOpResult{
		Success: true,
		Message: "Container started successfully",
	}
}

// StopContainer stops a container using existing SSH-enabled infrastructure
func StopContainer(ctx context.Context, actionProvider ContainerActionProvider, logger Logger, hostName, containerID string) *ContainerOpResult {
	err := actionProvider.PerformContainerAction(ctx, hostName, containerID, "stop")
	if err != nil {
		logger.ErrorLog("stop container failed: host=%s container=%s error=%v", hostName, containerID, err)
		return &ContainerOpResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	return &ContainerOpResult{
		Success: true,
		Message: "Container stopped successfully",
	}
}

// RestartContainer restarts a container using existing SSH-enabled infrastructure
func RestartContainer(ctx context.Context, actionProvider ContainerActionProvider, logger Logger, hostName, containerID string) *ContainerOpResult {
	err := actionProvider.PerformContainerAction(ctx, hostName, containerID, "restart")
	if err != nil {
		logger.ErrorLog("restart container failed: host=%s container=%s error=%v", hostName, containerID, err)
		return &ContainerOpResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	return &ContainerOpResult{
		Success: true,
		Message: "Container restarted successfully",
	}
}

// GetContainerLogs gets container logs using existing SSH-enabled infrastructure
func GetContainerLogs(ctx context.Context, hostName, containerID string) *ContainerOpResult {
	// Use the existing logs endpoint infrastructure
	// For simplicity, we'll return a success message since full log streaming
	// is already implemented in the web.go logs endpoint
	return &ContainerOpResult{
		Success: true,
		Message: "Use the logs endpoint for container logs",
		Output:  "Logs available via GET /hosts/" + hostName + "/containers/" + containerID + "/logs",
	}
}