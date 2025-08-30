// src/api/main.go
package main

import (
	"context"
	"log"
	"net/http"
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
	var total, errs int
	var mu sync.Mutex

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
				errs++
				log.Printf("scan: host=%s error=%v", h.Name, err)
				return
			}
			total += n
			log.Printf("scan: host=%s saved=%d", h.Name, n)
		}()
	}
	wg.Wait()
	log.Printf("scan: complete hosts=%d total_saved=%d errors=%d", len(hostRows), total, errs)
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
				// periodic scan
				scanAllOnce(ctx, perHostTO, conc)
			case <-ctx.Done():
				log.Printf("scan: auto scanner stopping: %v", ctx.Err())
				return
			}
		}
	}()
}
