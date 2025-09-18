// src/api/utils/ssh_transport.go
package utils

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"github.com/docker/docker/client"
)

// SSHConnection holds an SSH connection and its details
type SSHConnection struct {
	client   *ssh.Client
	hostAddr string
	user     string
	lastUsed time.Time
	mu       sync.RWMutex
}

// SSHConnectionPool manages SSH connections
type SSHConnectionPool struct {
	connections map[string]*SSHConnection
	mu          sync.RWMutex
}

var SSHPool = &SSHConnectionPool{
	connections: make(map[string]*SSHConnection),
}

// GetSSHConnection gets or creates an SSH connection
func (p *SSHConnectionPool) GetSSHConnection(user, host, keyFile string) (*ssh.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := fmt.Sprintf("%s@%s", user, host)
	
	// Check if we have a valid connection
	if conn, exists := p.connections[key]; exists {
		conn.mu.RLock()
		if conn.client != nil {
			// Test connection
			session, err := conn.client.NewSession()
			if err == nil {
				session.Close()
				conn.lastUsed = time.Now()
				conn.mu.RUnlock()
				// common.DebugLog("SSH: Reusing existing connection to %s", key) // Comment out - needs to be injected
				return conn.client, nil
			}
		}
		conn.mu.RUnlock()
		// Connection is stale, remove it
		delete(p.connections, key)
	}

	// Create new connection
	// common.DebugLog("SSH: Creating new connection to %s", key) // Comment out - needs to be injected
	sshClient, err := CreateSSHClient(user, host, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %v", err)
	}

	conn := &SSHConnection{
		client:   sshClient,
		hostAddr: host,
		user:     user,
		lastUsed: time.Now(),
	}
	p.connections[key] = conn

	return sshClient, nil
}

// CreateSSHClient creates a new SSH client connection
func CreateSSHClient(user, host, keyFile string) (*ssh.Client, error) {
	// Read private key
	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key file %s: %v", keyFile, err)
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH private key: %v", err)
	}

	// SSH client configuration
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Based on SSH_STRICT_HOST_KEY=false
		Timeout:         10 * time.Second,
	}

	// Connect to SSH server
	addr := host
	if !strings.Contains(addr, ":") {
		addr = addr + ":22" // Default SSH port
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH server %s: %v", addr, err)
	}

	// common.DebugLog("SSH: Successfully connected to %s@%s", user, host) // Comment out - needs to be injected
	return client, nil
}

// SSHTransport implements http.RoundTripper for SSH tunneling
type SSHTransport struct {
	sshClient *ssh.Client
	transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper
func (t *SSHTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Create a custom transport that dials through SSH
	transport := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			// Create SSH tunnel to Docker socket
			conn, err := t.sshClient.Dial("unix", "/var/run/docker.sock")
			if err != nil {
				return nil, fmt.Errorf("failed to create SSH tunnel to Docker socket: %v", err)
			}
			return conn, nil
		},
		// Set reasonable timeouts
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Execute the request through SSH tunnel
	return transport.RoundTrip(req)
}

// CreateSSHDockerClient creates a Docker client that uses SSH transport
func CreateSSHDockerClient(user, host, keyFile string) (*client.Client, func(), error) {
	// Get SSH connection from pool
	sshClient, err := SSHPool.GetSSHConnection(user, host, keyFile)
	if err != nil {
		return nil, nil, err
	}

	// Create SSH transport
	sshTransport := &SSHTransport{
		sshClient: sshClient,
	}

	// Create HTTP client with SSH transport
	httpClient := &http.Client{
		Transport: sshTransport,
		Timeout:   30 * time.Second,
	}

	// Create Docker client with custom HTTP client
	dockerClient, err := client.NewClientWithOpts(
		client.WithHost("unix:///var/run/docker.sock"), // This will be tunneled through SSH
		client.WithHTTPClient(httpClient),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	cleanup := func() {
		if dockerClient != nil {
			dockerClient.Close()
		}
		// Don't close SSH connection as it's pooled
	}

	// common.DebugLog("SSH: Created Docker client with SSH transport for %s@%s", user, host) // Comment out - needs to be injected
	return dockerClient, cleanup, nil
}

// ParseSSHURL extracts user and host from ssh:// URL
// ParseSSHURL parses an SSH URL to extract user and host
func ParseSSHURL(sshURL string) (user, host string, err error) {
	if !strings.HasPrefix(sshURL, "ssh://") {
		return "", "", fmt.Errorf("invalid SSH URL format: %s", sshURL)
	}

	// Remove ssh:// prefix
	address := strings.TrimPrefix(sshURL, "ssh://")
	
	// Split user@host
	parts := strings.SplitN(address, "@", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("SSH URL must contain user@host format: %s", sshURL)
	}

	return parts[0], parts[1], nil
}