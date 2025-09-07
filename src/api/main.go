// src/api/main.go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var startedAt = time.Now()

func main() {
	addr := env("DDUI_BIND", ":8080")

	if err := InitAuthFromEnv(); err != nil {
		log.Fatalf("OIDC setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := InitDBFromEnv(ctx); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	if err := InitInventory(); err != nil {
		log.Fatalf("inventory init failed: %v", err)
	}

	// kick off background auto-scanner (Portainer-ish cadence)
	startAutoScanner(ctx)
	startIacAutoScanner(ctx)

	log.Printf("DDUI API on %s (ui=/home/ddui/ui/dist)", addr)
	if err := http.ListenAndServe(addr, makeRouter()); err != nil {
		log.Fatal(err)
	}
}

/* -------- auto-scan loop (all hosts) -------- */

func envBool(key, def string) bool {
	v := strings.ToLower(env(key, def))
	return v == "1" || v == "t" || v == "true" || v == "yes" || v == "on"
}
func envDur(key, def string) time.Duration {
	if d, err := time.ParseDuration(env(key, def)); err == nil {
		return d
	}
	out, _ := time.ParseDuration(def)
	return out
}
func envInt(key string, def int) int {
	if s := env(key, ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

// run one full pass across hosts with limited concurrency
func scanAllOnce(ctx context.Context, perHostTO time.Duration, conc int) {
	hostRows, err := ListHosts(ctx)
	if err != nil {
		log.Printf("scan: list hosts failed: %v", err)
		return
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	var mu sync.Mutex

	var total, scanned, skipped, failed int

	for _, h := range hostRows {
		h := h
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			hctx, cancel := context.WithTimeout(ctx, perHostTO)
			n, err := ScanHostContainers(hctx, h.Name)
			cancel()

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				// treat intentional skips distinctly
				if errors.Is(err, ErrSkipScan) {
					skipped++
					return
				}
				failed++
				log.Printf("scan: host=%s error=%v", h.Name, err)
				return
			}
			scanned++
			total += n
			log.Printf("scan: host=%s saved=%d", h.Name, n)
		}()
	}
	wg.Wait()
	log.Printf("scan: complete hosts=%d scanned=%d skipped=%d total_saved=%d errors=%d",
		len(hostRows), scanned, skipped, total, failed)
}

func startAutoScanner(ctx context.Context) {
	if !envBool("DDUI_SCAN_AUTO", "true") {
		log.Printf("scan: auto disabled (DDUI_SCAN_AUTO=false)")
		return
	}
	interval := envDur("DDUI_SCAN_INTERVAL", "1m")       // Portainer-like
	perHostTO := envDur("DDUI_SCAN_HOST_TIMEOUT", "45s") // per host protection
	conc := envInt("DDUI_SCAN_CONCURRENCY", 3)

	log.Printf("scan: auto enabled interval=%s host_timeout=%s conc=%d", interval, perHostTO, conc)

	// optional boot scan
	if envBool("DDUI_SCAN_ON_START", "true") {
		go scanAllOnce(ctx, perHostTO, conc)
	}

	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-t.C:
				scanAllOnce(ctx, perHostTO, conc)
			case <-ctx.Done():
				log.Printf("scan: auto scanner stopping: %v", ctx.Err())
				return
			}
		}
	}()
}

// ---- IaC auto-scan (local + apply) ----

func startIacAutoScanner(ctx context.Context) {
	if !envBool("DDUI_IAC_SCAN_AUTO", "true") {
		log.Printf("iac: auto disabled (DDUI_IAC_SCAN_AUTO=false)")
		return
	}
	interval := envDur("DDUI_IAC_SCAN_INTERVAL", "90s") // default 1m30s
	log.Printf("iac: auto enabled interval=%s", interval)

	// initial scan on boot (non-fatal)
	go func() {
		if _, _, err := ScanIacLocal(ctx); err != nil {
			log.Printf("iac: initial scan failed: %v", err)
		}
		if err := applyAutoDevOps(ctx); err != nil {
			log.Printf("iac: initial apply failed: %v", err)
		}
	}()

	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if _, _, err := ScanIacLocal(ctx); err != nil {
					log.Printf("iac: periodic scan failed: %v", err)
				}
				if err := applyAutoDevOps(ctx); err != nil {
					log.Printf("iac: apply failed: %v", err)
				}
			case <-ctx.Done():
				log.Printf("iac: auto scanner stopping: %v", ctx.Err())
				return
			}
		}
	}()
}

/* --- Auto DevOps evaluator:
     - Default disabled.
     - If .env DDUI_DEVOPS_APPLY is present at stack > host > global, it overrides DB flag.
     - Else DB iac_enabled used.
     - When effective true: deploy stack (compose/script), which idempotently fixes drift & creates missing.
*/

func applyAutoDevOps(ctx context.Context) error {
	rows, err := db.Query(ctx, `SELECT id FROM iac_stacks`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}

		// Must have content (compose/services) or there's nothing to deploy
		has, _ := stackHasContent(ctx, id)
		if !has {
			continue
		}

		// Respect the *effective* Auto DevOps policy (global env + DB overrides)
		allowed, err := shouldAutoApply(ctx, id)
		if err != nil || !allowed {
			continue
		}

		// Kick the deploy (manual=false -> gated in deployStack, which is fine)
		dctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		_ = deployStack(dctx, id) // best effort; idempotent for compose
		cancel()
	}
	return nil
}
