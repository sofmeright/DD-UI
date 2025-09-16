// types.go - Shared interfaces and types
package utils

import "context"

// StackStager interface for staging stacks
type StackStager interface {
	StageStackForCompose(ctx context.Context, stackID int64) (string, interface{}, func(), error)
}