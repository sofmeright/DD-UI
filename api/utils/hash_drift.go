// hash_drift.go - Simple wrappers for hash-based drift detection
package utils

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/docker/docker/client"
)

// StageStackFunc is a function type for staging stack operations
type StageStackFunc func(ctx context.Context, stackID int64) (string, interface{}, func(), error)

// DetectDriftViaHashesWithStager - Wrapper that accepts a staging function and DB
func DetectDriftViaHashesWithStager(ctx context.Context, db *pgxpool.Pool, stackID int64, stackName string, cli *client.Client, stageFunc StageStackFunc) (bool, string, error) {
	stager := &funcStackStager{stageFunc: stageFunc}
	return DetectDriftViaHashesWithDeps(ctx, db, stager, stackID, stackName, cli)
}

// OnSuccessfulDeploymentWithStager - Wrapper that accepts a staging function and DB
func OnSuccessfulDeploymentWithStager(ctx context.Context, db *pgxpool.Pool, stackID int64, stackName string, cli *client.Client, stageFunc StageStackFunc) error {
	stager := &funcStackStager{stageFunc: stageFunc}
	return OnSuccessfulDeploymentWithDeps(ctx, db, stager, stackID, stackName, cli)
}

// funcStackStager adapts a function to the StackStager interface
type funcStackStager struct {
	stageFunc StageStackFunc
}

func (f *funcStackStager) StageStackForCompose(ctx context.Context, stackID int64) (string, interface{}, func(), error) {
	return f.stageFunc(ctx, stackID)
}

// DetectDriftViaHashesWithDeps - Version with dependency injection (renamed from utils/hash.go)
func DetectDriftViaHashesWithDeps(ctx context.Context, db *pgxpool.Pool, stager StackStager, stackID int64, stackName string, cli *client.Client) (bool, string, error) {
	// This calls the implementation from utils/hash.go
	return detectDriftViaHashesImpl(ctx, db, stager, stackID, stackName, cli)
}

// OnSuccessfulDeploymentWithDeps - Version with dependency injection (renamed from utils/hash.go)
func OnSuccessfulDeploymentWithDeps(ctx context.Context, db *pgxpool.Pool, stager StackStager, stackID int64, stackName string, cli *client.Client) error {
	// This calls the implementation from utils/hash.go
	return onSuccessfulDeploymentImpl(ctx, db, stager, stackID, stackName, cli)
}