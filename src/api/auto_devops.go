package main

import (
	"context"
	"fmt"
)

// shouldAutoApply returns true when it's appropriate to apply changes automatically.
// Policy here is simple and driven by IaC bundle changes:
//   • Stack must be iac_enabled AND have content.
//   • If no successful deployment stamp exists → allow (initial deploy).
//   • If latest successful stamp hash != current bundle hash → allow (files changed).
//   • Else → false (no change to apply).
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
		// If stamps aren’t available (migration missing) or simply no success yet → allow
		return true, nil
	}

	if stamp.DeploymentHash != curHash {
		return true, nil
	}
	return false, nil
}
