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
	
	// Get initial mod time
	if invPath != "" {
		if stat, err := os.Stat(invPath); err == nil {
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
	if invPath == "" {
		return
	}
	
	stat, err := os.Stat(invPath)
	if err != nil {
		common.DebugLog("Inventory watcher: failed to stat %s: %v", invPath, err)
		return
	}
	
	modTime := stat.ModTime()
	if modTime.After(lastModTime) {
		common.InfoLog("Inventory file changed, reloading...")
		if err := ReloadInventory(); err != nil {
			common.ErrorLog("Failed to reload inventory: %v", err)
		} else {
			common.InfoLog("Inventory reloaded successfully")
			lastModTime = modTime
		}
	}
}