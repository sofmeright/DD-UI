package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dd-ui/common"
	"dd-ui/database"
)

// ReadmeGenerator handles README.md generation with embedded deployment information
type ReadmeGenerator struct {
	dataPath string
}

// NewReadmeGen creates a new README generator
func NewReadmeGen(dataPath string) *ReadmeGenerator {
	return &ReadmeGenerator{
		dataPath: dataPath,
	}
}

// UpdateReadme updates the README.md with current deployment information
func (r *ReadmeGenerator) UpdateReadme(ctx context.Context) error {
	readmePath := filepath.Join(r.dataPath, "README.md")
	
	// Read existing README or create new one
	content, err := os.ReadFile(readmePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create initial README
			content = []byte(r.getDefaultReadme())
		} else {
			return fmt.Errorf("failed to read README: %w", err)
		}
	}

	// Update deployment map
	content = r.updateDeploymentMap(ctx, content)
	
	// Update status badges
	content = r.updateStatusBadges(ctx, content)
	
	// Update last sync timestamp
	content = r.updateTimestamp(content)

	// Write updated README
	if err := os.WriteFile(readmePath, content, 0644); err != nil {
		return fmt.Errorf("failed to write README: %w", err)
	}

	common.InfoLog("README.md updated with deployment information")
	return nil
}

// updateDeploymentMap updates the deployment map section in README
func (r *ReadmeGenerator) updateDeploymentMap(ctx context.Context, content []byte) []byte {
	// Get all hosts and their stacks
	hosts, err := database.GetHosts(ctx)
	if err != nil {
		common.ErrorLog("Failed to get hosts for README: %v", err)
		return content
	}

	// Build deployment map
	var deploymentMap strings.Builder
	deploymentMap.WriteString("| Host | Deployed Stacks |\n")
	deploymentMap.WriteString("| ---- | --------------- |\n")

	// Sort hosts by name
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].Hostname < hosts[j].Hostname
	})

	for _, host := range hosts {
		// Get stacks for this host
		stacks, err := database.GetStacksByHost(ctx, host.Hostname)
		if err != nil {
			continue
		}

		if len(stacks) == 0 {
			deploymentMap.WriteString(fmt.Sprintf("| %s | _No stacks_ |\n", host.Hostname))
		} else {
			// Sort stacks by name
			sort.Slice(stacks, func(i, j int) bool {
				return stacks[i].Name < stacks[j].Name
			})

			stackNames := []string{}
			for _, stack := range stacks {
				// Add status indicator if needed
				statusIcon := ""
				if stack.State == "running" {
					statusIcon = "✅ "
				} else if stack.State == "stopped" {
					statusIcon = "🔴 "
				} else if stack.State == "partial" {
					statusIcon = "⚠️ "
				}
				stackNames = append(stackNames, statusIcon+stack.Name)
			}
			deploymentMap.WriteString(fmt.Sprintf("| %s | %s |\n", 
				host.Hostname, 
				strings.Join(stackNames, "<br>")))
		}
	}

	// Replace content between markers
	return r.replaceSection(content, 
		"<!-- START_DEPLOYMENTS_MAP -->",
		"<!-- END_DEPLOYMENTS_MAP -->",
		deploymentMap.String())
}

// updateStatusBadges updates status badge URLs
func (r *ReadmeGenerator) updateStatusBadges(ctx context.Context, content []byte) []byte {
	// Get sync status
	syncStatus, err := database.GetGitSyncStatus(ctx)
	if err != nil {
		return content
	}

	// Build status section
	var statusSection strings.Builder
	statusSection.WriteString("### System Status\n\n")
	
	// Add sync status badge
	if syncStatus["sync_enabled"] == true {
		statusSection.WriteString("![Git Sync](https://img.shields.io/badge/Git_Sync-Enabled-green)\n")
	} else {
		statusSection.WriteString("![Git Sync](https://img.shields.io/badge/Git_Sync-Disabled-red)\n")
	}

	// Add last sync time
	if lastSync, ok := syncStatus["last_sync"].(time.Time); ok {
		timeSince := time.Since(lastSync)
		badge := fmt.Sprintf("![Last Sync](https://img.shields.io/badge/Last_Sync-%s_ago-blue)\n", 
			r.formatDuration(timeSince))
		statusSection.WriteString(badge)
	}

	// Add host count
	hostCount, _ := database.GetHostCount(ctx)
	statusSection.WriteString(fmt.Sprintf("![Hosts](https://img.shields.io/badge/Hosts-%d-blue)\n", hostCount))

	// Add stack count
	stackCount, _ := database.GetStackCount(ctx)
	statusSection.WriteString(fmt.Sprintf("![Stacks](https://img.shields.io/badge/Stacks-%d-purple)\n", stackCount))

	// Add container count
	containerCount, _ := database.GetContainerCount(ctx)
	statusSection.WriteString(fmt.Sprintf("![Containers](https://img.shields.io/badge/Containers-%d-orange)\n", containerCount))

	return r.replaceSection(content,
		"<!-- START_STATUS_BADGES -->",
		"<!-- END_STATUS_BADGES -->",
		statusSection.String())
}

// updateTimestamp updates the last updated timestamp
func (r *ReadmeGenerator) updateTimestamp(content []byte) []byte {
	timestamp := fmt.Sprintf("_Last updated: %s_", time.Now().Format("2006-01-02 15:04:05 MST"))
	
	return r.replaceSection(content,
		"<!-- START_TIMESTAMP -->",
		"<!-- END_TIMESTAMP -->",
		timestamp)
}

// replaceSection replaces content between start and end markers
func (r *ReadmeGenerator) replaceSection(content []byte, startMarker, endMarker, newContent string) []byte {
	contentStr := string(content)
	
	// Find markers
	startIdx := strings.Index(contentStr, startMarker)
	endIdx := strings.Index(contentStr, endMarker)
	
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		// Markers not found or invalid - append to end
		if startIdx == -1 && endIdx == -1 {
			contentStr += fmt.Sprintf("\n\n%s\n%s\n%s\n", startMarker, newContent, endMarker)
		}
		return []byte(contentStr)
	}

	// Replace content between markers
	before := contentStr[:startIdx+len(startMarker)]
	after := contentStr[endIdx:]
	
	return []byte(before + "\n" + newContent + after)
}

// formatDuration formats a duration in a human-readable way
func (r *ReadmeGenerator) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	} else {
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}

// getDefaultReadme returns the default README template
func (r *ReadmeGenerator) getDefaultReadme() string {
	return `# DD-UI Repository

This repository is managed by [DD-UI](https://github.com/yourusername/dd-ui) - Docker Deployment UI.

## Overview

DD-UI automatically syncs the following files from this repository:
- ` + "`inventory`" + ` - Ansible-compatible inventory file
- ` + "`docker-compose/*`" + ` - Docker Compose stack configurations

<!-- START_STATUS_BADGES -->
<!-- END_STATUS_BADGES -->

## Deployment Map

The following table shows all hosts and their deployed Docker Compose stacks:

<!-- START_DEPLOYMENTS_MAP -->
<!-- END_DEPLOYMENTS_MAP -->

## Repository Structure

` + "```" + `
/
├── inventory                 # Ansible inventory file
├── docker-compose/          # Docker Compose stacks
│   └── <hostname>/         # Host-specific stacks
│       └── <stackname>/    # Individual stack configurations
│           ├── docker-compose.yml
│           └── .env        # Environment variables
└── README.md               # This file (auto-updated)
` + "```" + `

## Features

- **Selective Sync**: Only ` + "`inventory`" + ` and ` + "`docker-compose/*`" + ` files are tracked
- **Ansible Compatible**: The inventory file maintains Ansible compatibility
- **Automatic Updates**: This README is automatically updated with deployment status
- **Conflict Resolution**: Smart handling of merge conflicts
- **SOPS Encryption**: Supports encrypted secrets in .env files

<!-- START_TIMESTAMP -->
<!-- END_TIMESTAMP -->
`
}

// GenerateAndCommit generates README and commits if there are changes
func (r *ReadmeGenerator) GenerateAndCommit(ctx context.Context) error {
	// Update README
	if err := r.UpdateReadme(ctx); err != nil {
		return fmt.Errorf("failed to update README: %w", err)
	}

	// Check if Git sync is enabled
	gitSync := GetGitSync()
	if gitSync.config != nil && gitSync.config.SyncEnabled {
		// Commit and push changes
		if err := gitSync.Push(ctx, "Update README.md with deployment status", "system"); err != nil {
			return fmt.Errorf("failed to push README updates: %w", err)
		}
	}

	return nil
}