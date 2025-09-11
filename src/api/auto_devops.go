// src/api/auto_devops.go
package main

import (
	"context"
	"fmt"
)

// shouldAutoDeployNow returns true when it is appropriate to auto-apply a deploy *now*.
// It composes your existing policy toggle (shouldAutoApply in web.go) with
// “did the IaC bundle change?” semantics.
//
// Rules:
//   • Global/host/group/stack policy must allow auto (via existing shouldAutoApply).
//   • Stack must have content.
//   • If no successful deployment stamp exists → allow (initial deploy).
//   • If latest successful stamp hash != current bundle hash → allow.
//   • Else → false.
func shouldAutoDeployNow(ctx context.Context, stackID int64) (bool, error) {
	// First: respect your policy toggle (already implemented in web.go).
	policyOK, err := shouldAutoApply(ctx, stackID)
	if err != nil {
		return false, fmt.Errorf("shouldAutoDeployNow/policy: %w", err)
	}
	if !policyOK {
		return false, nil
	}

	// Must have something to deploy.
	has, err := stackHasContent(ctx, stackID)
	if err != nil {
		return false, fmt.Errorf("shouldAutoDeployNow/content: %w", err)
	}
	if !has {
		return false, nil
	}

	// Hash the current tracked bundle (compose/env/scripts/etc).
	curHash, err := ComputeCurrentBundleHash(ctx, stackID)
	if err != nil {
		return false, fmt.Errorf("shouldAutoDeployNow/hash: %w", err)
	}

	// Compare with latest successful stamp.
	stamp, err := GetLatestDeploymentStamp(ctx, stackID)
	if err != nil {
		// No stamps yet or feature not migrated → treat as initial deploy allowed.
		return true, nil
	}
	return stamp.DeploymentHash != curHash, nil
}
