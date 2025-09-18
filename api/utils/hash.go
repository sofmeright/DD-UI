// src/api/utils/hash.go
package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Stack drift cache data structure
type StackDriftCache struct {
	StackID            int64             `json:"stack_id"`
	BundleHash         string            `json:"bundle_hash"`
	DockerConfigCache  map[string]string `json:"docker_config_cache"`
	LastUpdated        time.Time         `json:"last_updated"`
}

func ComputeCurrentBundleHash(ctx context.Context, stager StackStager, stackID int64) (string, error) {
	// common.DebugLog("Computing bundle hash for stack ID %d", stackID) // Comment out - needs to be injected
	
	// Stage all files with SOPS decryption
	stageDir, _, cleanup, err := stager.StageStackForCompose(ctx, stackID)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return "", fmt.Errorf("failed to stage stack for bundle hash: %w", err)
	}
	
	// Hash the decrypted content in staging directory
	bundleHash, err := HashDirectoryContents(stageDir)
	if err != nil {
		return "", fmt.Errorf("failed to hash directory contents: %w", err)
	}
	
	// common.DebugLog("Stack ID %d bundle hash: %s", stackID, bundleHash) // Comment out - needs to be injected
	return bundleHash, nil
}

// HashDirectoryContents computes a hash of all files in a directory
func HashDirectoryContents(dirPath string) (string, error) {
	var fileHashes []string
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			return nil
		}
		
		// Get relative path for consistent hashing
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		
		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		
		// Hash filename + content
		h := sha256.New()
		h.Write([]byte(relPath))
		h.Write(content)
		fileHash := hex.EncodeToString(h.Sum(nil))
		
		fileHashes = append(fileHashes, fmt.Sprintf("%s:%s", relPath, fileHash))
		return nil
	})
	
	if err != nil {
		return "", err
	}
	
	// Sort for consistent ordering
	sort.Strings(fileHashes)
	
	// Hash the combined file hashes
	h := sha256.New()
	for _, fh := range fileHashes {
		io.WriteString(h, fh)
	}
	
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GetActualDockerConfigHashes gets Docker config hashes from container labels
func GetActualDockerConfigHashes(ctx context.Context, stackName string, cli *client.Client) (map[string]string, error) {
	projectLabel := ComposeProjectLabelFromStack(stackName)
	
	// Filter containers by project
	ff := filters.NewArgs()
	ff.Add("label", "com.docker.compose.project="+projectLabel)
	
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: ff,
	})
	if err != nil {
		return nil, err
	}
	
	// Extract config hashes (lightweight operation)
	hashes := make(map[string]string)
	for _, cont := range containers {
		serviceName := ""
		configHash := ""
		
		if cont.Labels != nil {
			serviceName = cont.Labels["com.docker.compose.service"]
			configHash = cont.Labels["com.docker.compose.config-hash"]
		}
		
		if serviceName != "" && configHash != "" {
			hashes[serviceName] = configHash
		}
	}
	
	// common.DebugLog("Stack %s actual Docker config hashes: %v", stackName, hashes) // Comment out - needs to be injected
	return hashes, nil
}

// GetStoredBundleHash retrieves cached bundle hash from database
func GetStoredBundleHash(ctx context.Context, db *pgxpool.Pool, stackID int64) (string, error) {
	var bundleHash string
	err := db.QueryRow(ctx, `
		SELECT bundle_hash 
		FROM stack_drift_cache 
		WHERE stack_id = $1
	`, stackID).Scan(&bundleHash)
	
	if err != nil {
		// No cache entry exists
		return "", nil
	}
	
	return bundleHash, nil
}

// UpdateStoredBundleHash updates the cached bundle hash
func UpdateStoredBundleHash(ctx context.Context, db *pgxpool.Pool, stackID int64, bundleHash string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO stack_drift_cache (stack_id, bundle_hash, docker_config_cache, last_updated)
		VALUES ($1, $2, '{}', NOW())
		ON CONFLICT (stack_id) 
		DO UPDATE SET 
			bundle_hash = EXCLUDED.bundle_hash,
			last_updated = NOW()
	`, stackID, bundleHash)
	
	return err
}

// GetCachedDockerConfigHashes retrieves cached Docker config hashes
func GetCachedDockerConfigHashes(ctx context.Context, db *pgxpool.Pool, stackID int64) (map[string]string, error) {
	var cacheJSON string
	err := db.QueryRow(ctx, `
		SELECT docker_config_cache::text 
		FROM stack_drift_cache 
		WHERE stack_id = $1
	`, stackID).Scan(&cacheJSON)
	
	if err != nil {
		// No cache entry
		return make(map[string]string), nil
	}
	
	var hashes map[string]string
	if err := json.Unmarshal([]byte(cacheJSON), &hashes); err != nil {
		return make(map[string]string), nil
	}
	
	return hashes, nil
}

// StoreCachedDockerConfigHashes stores Docker config hashes in cache
func StoreCachedDockerConfigHashes(ctx context.Context, db *pgxpool.Pool, stackID int64, hashes map[string]string) error {
	cacheJSON, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	
	_, err = db.Exec(ctx, `
		INSERT INTO stack_drift_cache (stack_id, bundle_hash, docker_config_cache, last_updated)
		VALUES ($1, '', $2, NOW())
		ON CONFLICT (stack_id)
		DO UPDATE SET 
			docker_config_cache = EXCLUDED.docker_config_cache,
			last_updated = NOW()
	`, stackID, string(cacheJSON))
	
	return err
}

// ClearCachedDockerConfigHashes clears Docker config cache when bundle changes
func ClearCachedDockerConfigHashes(ctx context.Context, db *pgxpool.Pool, stackID int64) error {
	_, err := db.Exec(ctx, `
		UPDATE stack_drift_cache 
		SET docker_config_cache = '{}', last_updated = NOW()
		WHERE stack_id = $1
	`, stackID)
	
	return err
}

// HashMapsEqual compares two hash maps
func HashMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	
	return true
}

// DetectDriftViaHashes implements the two-tier hash-based drift detection
func detectDriftViaHashesImpl(ctx context.Context, db *pgxpool.Pool, stager StackStager, stackID int64, stackName string, cli *client.Client) (bool, string, error) {
	// common.DebugLog("Stack %s (ID %d): starting hash-based drift detection", stackName, stackID) // Comment out - needs to be injected
	
	// TIER 1: IaC File Change Detection
	currentBundleHash, err := ComputeCurrentBundleHash(ctx, stager, stackID)
	if err != nil {
		return false, "", err
	}
	
	storedBundleHash, err := GetStoredBundleHash(ctx, db, stackID)
	if err != nil {
		return false, "", err
	}
	
	// common.DebugLog("Stack %s: bundle hash current=%s, stored=%s", stackName, currentBundleHash, storedBundleHash) // Comment out - needs to be injected
	
	// IaC files changed?
	if currentBundleHash != storedBundleHash {
		// common.DebugLog("Stack %s: bundle hash changed, clearing Docker config cache", stackName) // Comment out - needs to be injected
		
		// Clear cached Docker hashes - forces container recheck
		if err := ClearCachedDockerConfigHashes(ctx, db, stackID); err != nil {
			return false, "", err
		}
		
		// Update stored bundle hash
		if err := UpdateStoredBundleHash(ctx, db, stackID, currentBundleHash); err != nil {
			return false, "", err
		}
		
		return true, "IaC files changed since last deployment", nil
	}
	
	// TIER 2: Container Configuration Change Detection  
	cachedDockerHashes, err := GetCachedDockerConfigHashes(ctx, db, stackID)
	if err != nil {
		return false, "", err
	}
	
	actualDockerHashes, err := GetActualDockerConfigHashes(ctx, stackName, cli)
	if err != nil {
		// common.DebugLog("Stack %s: Docker API failed, using cached hashes: %v", stackName, err) // Comment out - needs to be injected
		return false, "Unable to verify container state", nil
	}
	
	// common.DebugLog("Stack %s: Docker config hashes cached=%v, actual=%v", stackName, cachedDockerHashes, actualDockerHashes) // Comment out - needs to be injected
	
	// Container configs changed?
	if !HashMapsEqual(cachedDockerHashes, actualDockerHashes) {
		// Update cache with current reality
		if err := StoreCachedDockerConfigHashes(ctx, db, stackID, actualDockerHashes); err != nil {
			return false, "", err
		}
		
		return true, "Container configurations changed", nil
	}
	
	// common.DebugLog("Stack %s: drift detection via hashes: drift=false, reason=No drift detected", stackName) // Comment out - needs to be injected
	return false, "No drift detected", nil
}

// OnSuccessfulDeployment updates drift cache after successful deployment
func onSuccessfulDeploymentImpl(ctx context.Context, db *pgxpool.Pool, stager StackStager, stackID int64, stackName string, cli *client.Client) error {
	// common.DebugLog("Stack %s (ID %d): updating drift cache after successful deployment", stackName, stackID) // Comment out - needs to be injected
	
	// Calculate and store bundle hash after successful deployment
	bundleHash, err := ComputeCurrentBundleHash(ctx, stager, stackID)
	if err != nil {
		return err
	}
	
	// Get Docker config hashes from newly deployed containers
	dockerHashes, err := GetActualDockerConfigHashes(ctx, stackName, cli)
	if err != nil {
		return err
	}
	
	// Store both in cache
	return UpdateStackDriftCache(ctx, db, stackID, bundleHash, dockerHashes)
}

// UpdateStackDriftCache updates both bundle hash and Docker config hashes
func UpdateStackDriftCache(ctx context.Context, db *pgxpool.Pool, stackID int64, bundleHash string, dockerHashes map[string]string) error {
	cacheJSON, err := json.Marshal(dockerHashes)
	if err != nil {
		return err
	}
	
	_, err = db.Exec(ctx, `
		INSERT INTO stack_drift_cache (stack_id, bundle_hash, docker_config_cache, last_updated)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (stack_id)
		DO UPDATE SET 
			bundle_hash = EXCLUDED.bundle_hash,
			docker_config_cache = EXCLUDED.docker_config_cache,
			last_updated = NOW()
	`, stackID, bundleHash, string(cacheJSON))
	
	return err
}