package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ctxManualKey marks a deploy as "manual", which bypasses Auto DevOps gating.
type ctxManualKey struct{}

// deployStack stages a mirror of the stack into a scope-aware builds dir,
// auto-decrypts any SOPS-protected env files into that stage (same names),
// then runs `docker compose up -d` with the staged compose files.
// Originals are never modified and plaintext only lives in the stage dir.
//
// IMPORTANT: Non-manual invocations are **gated** by shouldAutoApply(ctx, stackID).
// Manual invocations bypass Auto DevOps (still require files to exist).
func deployStack(ctx context.Context, stackID int64) error {
	// Auto-DevOps gate (unless manual override)
	if man, _ := ctx.Value(ctxManualKey{}).(bool); !man {
		allowed, aerr := shouldAutoApply(ctx, stackID)
		if aerr != nil {
			return aerr
		}
		if !allowed {
			log.Printf("deploy: stack %d skipped (auto_devops disabled by effective policy)", stackID)
			return nil
		}
	}

	// Working dir for compose (stack root on disk)
	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return err
	}
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		return errors.New("deploy: stack has no rel_path")
	}

	// Stage files for compose
	stageDir, stagedComposes, cleanup, derr := stageStackForCompose(ctx, stackID)
	if derr != nil {
		return derr
	}
	defer func() { if cleanup != nil { cleanup() } }()

	// Nothing to deploy? No-op (kept for clarity)
	if len(stagedComposes) == 0 {
		log.Printf("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// Build desired spec (resolved, SOPS-safe) and generate DDUI overlay with labels
	spec, specErr := buildDesiredSpec(ctx, stackID)
	if specErr != nil {
		return specErr
	}
	overlayPath, ovErr := writeDDUIOverlay(ctx, stageDir, stackID, spec)
	if ovErr != nil {
		return ovErr
	}
	stagedComposes = append(stagedComposes, overlayPath)

	// docker compose -f <files...> up -d --remove-orphans
	args := []string{"compose"}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("deploy: docker compose failed: %v\n%s", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s)", stackID, len(stagedComposes), stageDir, root)
	return nil
}

// writeDDUIOverlay writes a last-applied Compose file that injects deterministic
// DDUI labels into every service. It also persists the enrollment state (UID+spec hash).
func writeDDUIOverlay(ctx context.Context, stageDir string, stackID int64, spec *desiredSpec) (string, error) {
	type svcOverlay struct {
		Labels map[string]string `yaml:"labels,omitempty"`
	}
	type overlay struct {
		Version  string                `yaml:"version,omitempty"`
		Services map[string]svcOverlay `yaml:"services"`
	}
	ov := overlay{Version: "3.8", Services: map[string]svcOverlay{}}

	// sort for stable output
	svcNames := make([]string, 0, len(spec.Services))
	for name := range spec.Services {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

	for _, name := range svcNames {
		s := spec.Services[name]
		specHash := computeServiceSpecDigest(spec.Project, spec.FilesDigest, name, s)

		// Reuse prior UID if the spec digest hasn't changed.
		var prevUID, prevDigest string
		_ = db.QueryRow(ctx, `
			SELECT last_deploy_uid, last_spec_digest
			FROM iac_service_state
			WHERE stack_id=$1 AND service_name=$2
		`, stackID, name).Scan(&prevUID, &prevDigest)

		uid := prevUID
		if uid == "" || prevDigest != specHash {
			uid = randomHex(16) // new enrollment or digest changed
		}

		lbls := map[string]string{
			dduiLabelManaged: "true",
			dduiLabelStackID: fmt.Sprintf("%d", stackID),
			dduiLabelService: name,
			dduiLabelSpec:    specHash,
			dduiLabelUID:     uid,
			// Compatibility labels (surfaced in UI/filters)
			"DDUI_CONTAINER_UID": uid,
			"DDUI_SPEC_DIGEST":   specHash,
		}
		ov.Services[name] = svcOverlay{Labels: lbls}

		// Remember enrollment in DB
		if err := upsertServiceState(ctx, stackID, name, uid, specHash); err != nil {
			return "", err
		}
	}

	data, err := yaml.Marshal(&ov)
	if err != nil {
		return "", err
	}
	path := filepath.Join(stageDir, "ddui.overlay.yml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// upsertServiceState tracks service enrollment to make pruning safe and drift deterministic.
func upsertServiceState(ctx context.Context, stackID int64, service, uid, specDigest string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO iac_service_state (stack_id, service_name, last_deploy_uid, last_spec_digest, enrolled, first_seen)
		VALUES ($1,$2,$3,$4,TRUE,now())
		ON CONFLICT (stack_id, service_name)
		DO UPDATE SET last_deploy_uid=EXCLUDED.last_deploy_uid,
		              last_spec_digest=EXCLUDED.last_spec_digest,
		              enrolled=TRUE,
		              updated_at=now();
	`, stackID, service, uid, specDigest)
	return err
}

// randomHex returns lowercase hex of n random bytes.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// extremely unlikely; fall back to time+pid mix (still ok for a non-security UID)
		t := time.Now().UnixNano()
		copy(b, []byte(fmt.Sprintf("%x", t)))
	}
	return hex.EncodeToString(b)
}
