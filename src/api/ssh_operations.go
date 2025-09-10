// src/api/ssh_operations.go
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// SSHContainerOperation represents a container operation that can be performed over SSH
type SSHContainerOperation struct {
	HostName    string `json:"host_name"`
	ContainerID string `json:"container_id"`
	Operation   string `json:"operation"` // start, stop, restart, logs, exec
	Command     string `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
}

// ExecuteSSHContainerOperation performs container operations over SSH
func ExecuteSSHContainerOperation(ctx context.Context, op SSHContainerOperation) (string, error) {
	// Get host information
	host, err := GetHostByName(ctx, op.HostName)
	if err != nil {
		return "", fmt.Errorf("host not found: %v", err)
	}

	// Get Docker connection details
	dockerURL, sshCmd := dockerURLFor(host)
	
	// Create Docker client with SSH
	cli, done, err := dockerClientForURL(ctx, dockerURL, sshCmd)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Docker over SSH: %v", err)
	}
	defer done()

	switch op.Operation {
	case "start":
		return executeContainerStart(ctx, cli, op.ContainerID)
	case "stop":
		return executeContainerStop(ctx, cli, op.ContainerID)
	case "restart":
		return executeContainerRestart(ctx, cli, op.ContainerID)
	case "logs":
		return executeContainerLogs(ctx, cli, op.ContainerID)
	case "exec":
		return executeContainerExec(ctx, cli, op.ContainerID, op.Command, op.Args)
	default:
		return "", fmt.Errorf("unsupported operation: %s", op.Operation)
	}
}

// executeContainerStart starts a container
func executeContainerStart(ctx context.Context, cli dockerClient, containerID string) (string, error) {
	err := cli.ContainerStart(ctx, containerID, container.StartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start container: %v", err)
	}
	return "Container started successfully", nil
}

// executeContainerStop stops a container
func executeContainerStop(ctx context.Context, cli dockerClient, containerID string) (string, error) {
	timeout := int(10) // 10 seconds
	err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil {
		return "", fmt.Errorf("failed to stop container: %v", err)
	}
	return "Container stopped successfully", nil
}

// executeContainerRestart restarts a container
func executeContainerRestart(ctx context.Context, cli dockerClient, containerID string) (string, error) {
	timeout := int(10) // 10 seconds
	err := cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil {
		return "", fmt.Errorf("failed to restart container: %v", err)
	}
	return "Container restarted successfully", nil
}

// executeContainerLogs gets container logs
func executeContainerLogs(ctx context.Context, cli dockerClient, containerID string) (string, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "100", // Last 100 lines
		Timestamps: true,
	}
	
	logs, err := cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %v", err)
	}
	defer logs.Close()

	// Read logs using io.ReadAll
	logContent, err := io.ReadAll(logs.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %v", err)
	}
	return string(logContent), nil
}

// executeContainerExec executes a command in a container
func executeContainerExec(ctx context.Context, cli dockerClient, containerID, command string, args []string) (string, error) {
	// Create exec instance
	execConfig := types.ExecConfig{
		Cmd:          append([]string{command}, args...),
		AttachStdout: true,
		AttachStderr: true,
	}
	
	execResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec instance: %v", err)
	}

	// Start exec
	resp, err := cli.ContainerExecStart(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", fmt.Errorf("failed to start exec: %v", err)
	}
	defer resp.Close()

	// Read output using io.ReadAll
	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read exec output: %v", err)
	}
	return string(output), nil
}

// SSHDirectOperation represents a direct SSH operation on a host
type SSHDirectOperation struct {
	HostName string   `json:"host_name"`
	Command  string   `json:"command"`
	Args     []string `json:"args,omitempty"`
}

// ExecuteSSHDirectOperation performs direct SSH operations on a host
func ExecuteSSHDirectOperation(ctx context.Context, op SSHDirectOperation) (string, error) {
	// Get host information
	host, err := GetHostByName(ctx, op.HostName)
	if err != nil {
		return "", fmt.Errorf("host not found: %v", err)
	}

	// Build SSH command
	user := host.Vars["ansible_user"]
	if user == "" {
		user = env("SSH_USER", "root")
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

// StartContainer starts a container via SSH if needed
func StartContainer(ctx context.Context, hostName, containerID string) *ContainerOpResult {
	op := SSHContainerOperation{
		HostName:    hostName,
		ContainerID: containerID,
		Operation:   "start",
	}
	
	output, err := ExecuteSSHContainerOperation(ctx, op)
	if err != nil {
		log.Printf("start container failed: host=%s container=%s error=%v", hostName, containerID, err)
		return &ContainerOpResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	return &ContainerOpResult{
		Success: true,
		Message: "Container started successfully",
		Output:  output,
	}
}

// StopContainer stops a container via SSH if needed
func StopContainer(ctx context.Context, hostName, containerID string) *ContainerOpResult {
	op := SSHContainerOperation{
		HostName:    hostName,
		ContainerID: containerID,
		Operation:   "stop",
	}
	
	output, err := ExecuteSSHContainerOperation(ctx, op)
	if err != nil {
		log.Printf("stop container failed: host=%s container=%s error=%v", hostName, containerID, err)
		return &ContainerOpResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	return &ContainerOpResult{
		Success: true,
		Message: "Container stopped successfully",
		Output:  output,
	}
}

// RestartContainer restarts a container via SSH if needed
func RestartContainer(ctx context.Context, hostName, containerID string) *ContainerOpResult {
	op := SSHContainerOperation{
		HostName:    hostName,
		ContainerID: containerID,
		Operation:   "restart",
	}
	
	output, err := ExecuteSSHContainerOperation(ctx, op)
	if err != nil {
		log.Printf("restart container failed: host=%s container=%s error=%v", hostName, containerID, err)
		return &ContainerOpResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	return &ContainerOpResult{
		Success: true,
		Message: "Container restarted successfully",
		Output:  output,
	}
}

// GetContainerLogs gets container logs via SSH if needed
func GetContainerLogs(ctx context.Context, hostName, containerID string) *ContainerOpResult {
	op := SSHContainerOperation{
		HostName:    hostName,
		ContainerID: containerID,
		Operation:   "logs",
	}
	
	output, err := ExecuteSSHContainerOperation(ctx, op)
	if err != nil {
		log.Printf("get container logs failed: host=%s container=%s error=%v", hostName, containerID, err)
		return &ContainerOpResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	return &ContainerOpResult{
		Success: true,
		Message: "Logs retrieved successfully",
		Output:  output,
	}
}

// dockerClient interface to match the actual Docker client
type dockerClient interface {
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (types.HijackedResponse, error)
	ContainerExecCreate(ctx context.Context, containerID string, config types.ExecConfig) (types.IDResponse, error)
	ContainerExecStart(ctx context.Context, execID string, config types.ExecStartCheck) (types.HijackedResponse, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
}