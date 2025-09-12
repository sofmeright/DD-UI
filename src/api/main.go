// src/api/main.go
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"log"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var startedAt = time.Now()

func main() {
	addr := env("DDUI_BIND", ":443")

	if err := InitAuthFromEnv(); err != nil {
		fatalLog("OIDC setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := InitDBFromEnv(ctx); err != nil {
		fatalLog("DB init failed: %v", err)
	}
	if err := InitInventory(); err != nil {
		fatalLog("inventory init failed: %v", err)
	}

	// kick off background auto-scanner (Portainer-ish cadence)
	startAutoScanner(ctx)
	startIacAutoScanner(ctx)

	r := makeRouter()

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	enableTLS := isTrueish(env("DDUI_TLS_ENABLE", "true"))
	if !enableTLS {
		infoLog("http: listening on %s (TLS disabled)", addr)
		fatalLog("HTTP server error: %v", srv.ListenAndServe())
		return
	}

	certFile := strings.TrimSpace(env("DDUI_TLS_CERT_FILE", ""))
	keyFile := strings.TrimSpace(env("DDUI_TLS_KEY_FILE", ""))

	if certFile != "" && keyFile != "" {
		infoLog("https: listening on %s (cert=%s)", addr, certFile)
		fatalLog("HTTPS server error: %v", srv.ListenAndServeTLS(certFile, keyFile))
		return
	}

	if !isTrueish(env("DDUI_TLS_SELF_SIGNED", "true")) {
		fatalLog("https: TLS enabled but no cert files and self-signed disabled")
	}

	// Ephemeral self-signed (in-memory)
	certPEM, keyPEM, err := generateSelfSigned("ddui.local")
	if err != nil {
		fatalLog("Failed to generate self-signed certificate: %v", err)
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		fatalLog("Failed to load certificate key pair: %v", err)
	}
	srv.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}
	infoLog("https: listening on %s (self-signed)", addr)
	fatalLog("HTTPS server error: %v", srv.ListenAndServeTLS("", ""))
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

// logLevel returns the current log level, defaulting to "info"
func getLogLevel() string {
	return strings.ToLower(env("DDUI_LOG_LEVEL", "info"))
}

// shouldLog checks if the given level should be logged based on current log level
func shouldLog(level string) bool {
	currentLevel := getLogLevel()
	levelOrder := map[string]int{
		"debug": 0,
		"info":  1,
		"warn":  2,
		"error": 3,
		"fatal": 4,
	}
	
	currentLevelNum, exists := levelOrder[currentLevel]
	if !exists {
		currentLevelNum = 1 // default to info
	}
	
	targetLevelNum, exists := levelOrder[strings.ToLower(level)]
	if !exists {
		return false
	}
	
	return targetLevelNum >= currentLevelNum
}

// debugLog logs debug messages only if log level allows it
func debugLog(format string, args ...interface{}) {
	if shouldLog("debug") {
		log.Printf("DEBUG: "+format, args...)
	}
}

// infoLog logs info messages only if log level allows it
func infoLog(format string, args ...interface{}) {
	if shouldLog("info") {
		log.Printf("INFO: "+format, args...)
	}
}

// warnLog logs warning messages only if log level allows it
func warnLog(format string, args ...interface{}) {
	if shouldLog("warn") {
		log.Printf("WARN: "+format, args...)
	}
}

// errorLog logs error messages only if log level allows it
func errorLog(format string, args ...interface{}) {
	if shouldLog("error") {
		log.Printf("ERROR: "+format, args...)
	}
}

// fatalLog logs fatal messages and exits (always shown)
func fatalLog(format string, args ...interface{}) {
	log.Fatalf("FATAL: "+format, args...)
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
	infoLog("scan: complete hosts=%d scanned=%d skipped=%d total_saved=%d errors=%d",
		len(hostRows), scanned, skipped, total, failed)
}

func startAutoScanner(ctx context.Context) {
	if !envBool("DDUI_SCAN_DOCKER_AUTO", "true") {
		log.Printf("scan: auto disabled (DDUI_SCAN_DOCKER_AUTO=false)")
		return
	}
	interval := envDur("DDUI_SCAN_DOCKER_INTERVAL", "1m")       // Portainer-like
	perHostTO := envDur("DDUI_SCAN_DOCKER_HOST_TIMEOUT", "45s") // per host protection
	conc := envInt("DDUI_SCAN_DOCKER_CONCURRENCY", 3)

	log.Printf("scan: auto enabled interval=%s host_timeout=%s conc=%d", interval, perHostTO, conc)

	// optional boot scan
	if envBool("DDUI_SCAN_DOCKER_ON_START", "true") {
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
	if !envBool("DDUI_SCAN_IAC_AUTO", "true") {
		log.Printf("iac: auto disabled (DDUI_SCAN_IAC_AUTO=false)")
		return
	}
	interval := envDur("DDUI_SCAN_IAC_INTERVAL", "90s") // default 1m30s
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

/* -------- TLS self-signed helper -------- */

func generateSelfSigned(cn string) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{cn, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, nil
}
