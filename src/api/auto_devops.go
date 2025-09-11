// src/api/auto_devops.go
package main

import (
	"context"
	"fmt"
)

// shouldAutoApply returns true when it's appropriate to apply changes automatically.
// Policy (independent of image tags):
//   • Stack must be iac_enabled AND have any content (compose/env/scripts/other).
//   • If no successful deployment stamp exists → allow (initial deploy).
//   • If current bundle hash (tracked IaC files) != latest successful stamp hash → allow.
//   • Else → false.
func shouldAutoApply(ctx context.Context, stackID int64) (bool, error) {
	var enabled bool
	if err := db.QueryRow(ctx, `SELECT iac_enabled FROM iac_stacks WHERE id=$1`, stackID).Scan(&enabled); err != nil {
		return false, fmt.Errorf("shouldAutoApply: load stack: %w", err)
	}
	if !enabled {
		return false, nil
	}

	has, err := stackHasContent(ctx, stackID)
	if err != nil {
		return false, fmt.Errorf("shouldAutoApply: check content: %w", err)
	}
	if !has {
		return false, nil
	}

	curHash, err := ComputeCurrentBundleHash(ctx, stackID)
	if err != nil {
		return false, fmt.Errorf("shouldAutoApply: compute bundle hash: %w", err)
	}

	stamp, err := GetLatestDeploymentStamp(ctx, stackID)
	if err != nil {
		// Either table missing or simply no successful stamp yet → initial deploy allowed
		return true, nil
	}
	if stamp.DeploymentHash != curHash {
		return true, nil
	}
	return false, nil
}
