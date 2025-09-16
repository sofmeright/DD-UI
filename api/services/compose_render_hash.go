// src/api/compose_render_hash.go
package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"dd-ui/common"
	"gopkg.in/yaml.v3"
)

// computeRenderedConfigHash runs `docker compose config --hash` against the staged
// compose set and produces a stable hash by sorting and hashing all lines.
// On failure, returns empty string.
func computeRenderedConfigHash(ctx context.Context, stageDir string, projectName string, files []string) string {
	args := []string{"compose", "-p", projectName}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--hash")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	trimmed := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			trimmed = append(trimmed, ln)
		}
	}
	sort.Strings(trimmed)
	h := sha256.New()
	for _, ln := range trimmed {
		h.Write([]byte(ln))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// parseServiceConfigHashes extracts service-specific config hashes from `docker compose config --hash` output
func parseServiceConfigHashes(ctx context.Context, stageDir string, projectName string, files []string) (map[string]string, error) {
	args := []string{"compose", "-p", projectName}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--hash")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose config --hash failed: %v", err)
	}

	serviceHashes := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format is typically "service_name sha256:hash"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			serviceName := parts[0]
			hash := strings.TrimPrefix(parts[1], "sha256:")
			serviceHashes[serviceName] = hash
		}
	}
	return serviceHashes, nil
}

// computeComposeFilesHash returns sha256 over the concatenated bytes
// of all staged compose files (in the given order).
func computeComposeFilesHash(stageDir string, files []string) (string, error) {
	h := sha256.New()
	for _, f := range files {
		fp := f
		if !filepath.IsAbs(fp) {
			fp = filepath.Join(stageDir, f)
		}
		fd, err := os.Open(fp)
		if err != nil {
			return "", err
		}
		_, _ = io.Copy(h, fd)
		_ = fd.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RenderedService is the post-interpolation, post-SOPS view used by the UI.

// renderComposeServices processes SOPS-decrypted compose files and resolves variables
// following Docker Compose official precedence order:
// 1. Root .env file (project-level environment file interpolation)
// 2. Service-level environment: variables
// 3. Service-specific env_file: files (in order they appear)
// 4. Default values from ${VAR:-default} syntax
func renderComposeServices(ctx context.Context, stageDir, projectName string, files []string) ([]common.RenderedService, error) {
	// Create temporary directory for decrypted compose files
	tempDir, err := os.MkdirTemp("", "ddui-render-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Step 1: Decrypt all files and parse environment sources
	envFiles := make(map[string]map[string]string) // filename -> key-value pairs
	var rootEnv map[string]string                  // .env file variables
	var composeFiles []string
	
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return nil, fmt.Errorf("read stage dir: %v", err)
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		sourcePath := filepath.Join(stageDir, entry.Name())
		
		if strings.HasSuffix(entry.Name(), ".env") {
			// Files in stageDir are already decrypted - read as plain text
			rawContent, err := os.ReadFile(sourcePath)
			if err != nil {
				common.DebugLog("Failed to read env file %s: %v", entry.Name(), err)
				continue // Skip problematic env files but don't fail entirely
			}
			common.DebugLog("renderComposeServices: reading %s from %s, got %d bytes", entry.Name(), sourcePath, len(rawContent))
			if len(rawContent) > 0 {
				sample := string(rawContent)
				if len(sample) > 200 {
					sample = sample[:200] + "..."
				}
				common.DebugLog("renderComposeServices: %s content sample: %q", entry.Name(), sample)
			}
			content := filterDotenvSopsKeys(rawContent)
			envVars := parseEnvFileContent(content)
			envFiles[entry.Name()] = envVars
			
			// Check if this is the root .env file
			if entry.Name() == ".env" {
				rootEnv = envVars
				common.DebugLog("Parsed root .env file with %d variables (already decrypted in staging)", len(envVars))
				for k, v := range envVars {
					common.DebugLog("  Root .env: %s = %s", k, v)
				}
			} else {
				common.DebugLog("Parsed env file %s with %d variables (already decrypted in staging)", entry.Name(), len(envVars))
				for k, v := range envVars {
					common.DebugLog("  %s: %s = %s", entry.Name(), k, v)
				}
			}
			
			// Write env file to temp directory
			destPath := filepath.Join(tempDir, entry.Name())
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				return nil, fmt.Errorf("write env %s: %v", entry.Name(), err)
			}
		} else if strings.Contains(entry.Name(), "compose") || strings.HasSuffix(entry.Name(), ".yml") || strings.HasSuffix(entry.Name(), ".yaml") {
			// Files in stageDir are already decrypted - read as plain text
			content, err := os.ReadFile(sourcePath)
			if err != nil {
				common.DebugLog("Failed to read compose file %s: %v", entry.Name(), err)
				continue // Skip problematic compose files
			}
			common.DebugLog("Processed compose file %s (already decrypted in staging)", entry.Name())
			
			// Write compose file to temp directory
			destPath := filepath.Join(tempDir, entry.Name())
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				return nil, fmt.Errorf("write compose %s: %v", entry.Name(), err)
			}
			composeFiles = append(composeFiles, entry.Name())
		} else {
			// Copy other files as-is
			content, err := os.ReadFile(sourcePath)
			if err != nil {
				common.DebugLog("Failed to read file %s: %v", entry.Name(), err)
				continue
			}
			destPath := filepath.Join(tempDir, entry.Name())
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				return nil, fmt.Errorf("write file %s: %v", entry.Name(), err)
			}
		}
	}

	if len(composeFiles) == 0 {
		return nil, fmt.Errorf("no compose files found in %s", stageDir)
	}

	// Step 2: Try docker compose config first (fastest path when it works)
	args := []string{"compose", "-p", projectName}
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--format", "json")

	common.DebugLog("Running docker compose config in %s with files: %v", tempDir, composeFiles)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	
	if err == nil {
		// Docker compose config succeeded - use its output
		var payload struct {
			Services map[string]struct {
				Image         string            `json:"image"`
				ContainerName string            `json:"container_name"`
				Environment   map[string]string `json:"environment"`
			} `json:"services"`
		}
		if err := json.Unmarshal(out, &payload); err == nil {
			var rs []common.RenderedService
			for name, sv := range payload.Services {
				rs = append(rs, common.RenderedService{
					ServiceName:   name,
					ContainerName: strings.TrimSpace(sv.ContainerName),
					Image:         strings.TrimSpace(sv.Image),
				})
				common.DebugLog("Docker rendered service %s: image=%s, container=%s", name, sv.Image, sv.ContainerName)
			}
			sort.Slice(rs, func(i, j int) bool { return rs[i].ServiceName < rs[j].ServiceName })
			return rs, nil
		}
	}
	
	common.DebugLog("Docker compose config failed or parsing failed: %v, using manual parsing", err)
	
	// Step 3: Manual parsing with proper variable resolution
	return parseComposeWithVariableResolution(ctx, tempDir, composeFiles, rootEnv, envFiles)
}

// parseComposeWithVariableResolution manually parses compose files with proper variable resolution
func parseComposeWithVariableResolution(ctx context.Context, tempDir string, composeFiles []string, rootEnv map[string]string, envFiles map[string]map[string]string) ([]common.RenderedService, error) {
	common.DebugLog("Using manual compose parsing with variable resolution")
	
	var rs []common.RenderedService
	
	for _, filename := range composeFiles {
		content, err := os.ReadFile(filepath.Join(tempDir, filename))
		if err != nil {
			common.DebugLog("Failed to read compose file %s: %v", filename, err)
			continue
		}
		
		// Parse the compose file structure
		var compose struct {
			Services map[string]struct {
				Image       string            `json:"image" yaml:"image"`
				ContainerName string          `json:"container_name" yaml:"container_name"`
				Environment []interface{}     `json:"environment" yaml:"environment"`
				EnvFile     []string          `json:"env_file" yaml:"env_file"`
			} `json:"services" yaml:"services"`
		}
		
		// Try JSON first, then YAML
		if err := json.Unmarshal(content, &compose); err != nil {
			// Try YAML parsing
			if err := yaml.Unmarshal(content, &compose); err != nil {
				common.DebugLog("Failed to parse compose file %s as JSON or YAML: %v", filename, err)
				continue
			}
			common.DebugLog("Successfully parsed %s as YAML", filename)
		} else {
			common.DebugLog("Successfully parsed %s as JSON", filename)
		}
		
		for serviceName, service := range compose.Services {
			// Parse service-level environment variables
			serviceEnv := parseServiceEnvironment(service.Environment)
			
			// Collect service-specific env files in order
			var serviceEnvFiles []map[string]string
			for _, envFileName := range service.EnvFile {
				if envMap, exists := envFiles[envFileName]; exists {
					serviceEnvFiles = append(serviceEnvFiles, envMap)
				}
			}
			
			// Resolve variables using Docker Compose precedence
			common.DebugLog("Resolving variables for service %s", serviceName)
			common.DebugLog("  Original image: %s", service.Image)
			common.DebugLog("  Original container_name: %s", service.ContainerName)
			
			image := resolveVariablesWithPrecedence(service.Image, rootEnv, serviceEnv, serviceEnvFiles)
			containerName := resolveVariablesWithPrecedence(service.ContainerName, rootEnv, serviceEnv, serviceEnvFiles)
			if containerName == "" {
				containerName = serviceName // Default container name is service name
			}
			
			common.DebugLog("  Resolved image: %s", image)
			common.DebugLog("  Resolved container_name: %s", containerName)
			
			rs = append(rs, common.RenderedService{
				ServiceName:   serviceName,
				ContainerName: strings.TrimSpace(containerName),
				Image:         strings.TrimSpace(image),
			})
			common.DebugLog("Manual parsed service %s: image=%s, container=%s", serviceName, image, containerName)
		}
	}
	
	sort.Slice(rs, func(i, j int) bool { return rs[i].ServiceName < rs[j].ServiceName })
	return rs, nil
}

// parseServiceEnvironment converts Docker Compose environment format to map[string]string
func parseServiceEnvironment(env []interface{}) map[string]string {
	result := make(map[string]string)
	for _, item := range env {
		switch v := item.(type) {
		case string:
			// Format: "KEY=value" or just "KEY" (inherits from shell)
			if idx := strings.Index(v, "="); idx > 0 {
				key := strings.TrimSpace(v[:idx])
				value := strings.TrimSpace(v[idx+1:])
				result[key] = value
			}
			// Skip "KEY" format without value - would inherit from shell
		case map[string]interface{}:
			// Format: {KEY: value}
			for k, val := range v {
				if s, ok := val.(string); ok {
					result[k] = s
				}
			}
		}
	}
	return result
}

// resolveVariablesWithPrecedence resolves Docker Compose variables following official precedence:
// 1. Root .env file (project-level environment file interpolation)
// 2. Service-level environment: variables  
// 3. Service-specific env_file: files (in order they appear)
// 4. Default values from ${VAR:-default} syntax
func resolveVariablesWithPrecedence(input string, rootEnv, serviceEnv map[string]string, serviceEnvFiles []map[string]string) string {
	if input == "" {
		return input
	}
	
	result := input
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start
		
		varExpr := result[start+2 : end]
		var varName, defaultVal string
		
		if idx := strings.Index(varExpr, ":-"); idx > 0 {
			varName = varExpr[:idx]
			defaultVal = varExpr[idx+2:]
		} else {
			varName = varExpr
		}
		
		var value string
		var source string
		
		// 1. Root .env file (project-level, highest priority)
		if rootEnv != nil {
			if v, exists := rootEnv[varName]; exists {
				value = v
				source = "root .env"
			}
		}
		
		// 2. Service-level environment: variables
		if value == "" && serviceEnv != nil {
			if v, exists := serviceEnv[varName]; exists {
				value = v
				source = "service environment"
			}
		}
		
		// 3. Service-specific env_file: files (in order they appear)
		if value == "" {
			for i, envFile := range serviceEnvFiles {
				if v, exists := envFile[varName]; exists {
					value = v
					source = fmt.Sprintf("service env_file[%d]", i)
					break
				}
			}
		}
		
		// 4. Default values from ${VAR:-default} syntax (lowest priority)
		if value == "" {
			value = defaultVal
			if defaultVal != "" {
				source = "default value"
			}
		}
		
		if source != "" {
			common.DebugLog("Resolved %s=%s from %s", varName, value, source)
		} else if value == "" {
			common.DebugLog("Variable %s not found in any source, leaving unresolved", varName)
		}
		
		result = result[:start] + value + result[end+1:]
	}
	
	return result
}

// parseEnvFileContent parses key=value pairs from .env file content
func parseEnvFileContent(content []byte) map[string]string {
	vars := make(map[string]string)
	common.DebugLog("parseEnvFileContent: received %d bytes of content", len(content))
	if len(content) > 0 {
		common.DebugLog("parseEnvFileContent: content sample (first 200 chars): %q", string(content[:min(200, len(content))]))
	}
	lines := strings.Split(string(content), "\n")
	common.DebugLog("parseEnvFileContent: split into %d lines", len(lines))
	for i, line := range lines {
		common.DebugLog("parseEnvFileContent: line %d: %q", i, line)
		line = strings.TrimSpace(line)
		common.DebugLog("parseEnvFileContent: line %d after trim: %q", i, line)
		if line == "" || strings.HasPrefix(line, "#") {
			common.DebugLog("parseEnvFileContent: line %d skipped (empty or comment)", i)
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			// Remove quotes if present
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
			common.DebugLog("parseEnvFileContent: found var: %s = %s", key, value)
			vars[key] = value
		} else {
			common.DebugLog("parseEnvFileContent: line %d has no valid = assignment: %q", i, line)
		}
	}
	return vars
}
