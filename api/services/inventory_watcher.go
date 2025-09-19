package services

import (
	"context"
	"time"
	"os"
	"dd-ui/common"
)

var (
	lastModTime time.Time
	watcherRunning bool
)

// StartInventoryWatcher monitors the inventory file for changes and reloads automatically
func StartInventoryWatcher(ctx context.Context) {
	if watcherRunning {
		return
	}
	watcherRunning = true
	
	// Get initial mod time from InventoryManager
	invMgr := GetInventoryManager()
	if invMgr.path != "" {
		if stat, err := os.Stat(invMgr.path); err == nil {
			lastModTime = stat.ModTime()
		}
	}
	
	go func() {
		ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				common.InfoLog("Inventory watcher stopped")
				watcherRunning = false
				return
			case <-ticker.C:
				checkAndReloadInventory()
			}
		}
	}()
	
	common.InfoLog("Inventory watcher started (checking every 10s)")
}

func checkAndReloadInventory() {
	// Reload both the old inventory system and the new InventoryManager
	invMgr := GetInventoryManager()
	if invMgr.path == "" {
		return
	}
	
	stat, err := os.Stat(invMgr.path)
	if err != nil {
		common.DebugLog("Inventory watcher: failed to stat %s: %v", invMgr.path, err)
		return
	}
	
	modTime := stat.ModTime()
	if modTime.After(lastModTime) {
		common.InfoLog("Inventory file changed, reloading...")
		
		// Reload the new InventoryManager
		if err := invMgr.Reload(); err != nil {
			common.ErrorLog("Failed to reload InventoryManager: %v", err)
		} else {
			common.InfoLog("InventoryManager reloaded successfully")
		}
		
		// Also reload the old inventory system for backward compatibility
		if err := ReloadInventory(); err != nil {
			common.ErrorLog("Failed to reload old inventory: %v", err)
		}
		
		lastModTime = modTime
	}
}