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
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/services"
)

var startedAt = time.Now()

// View-based polling boost system
type ViewBoostTracker struct {
	mu           sync.RWMutex
	activeViews  map[string]int        // host -> count of active viewers
	boostTimers  map[string]*time.Timer // host -> timer for boost cleanup
}

var viewBoostTracker = &ViewBoostTracker{
	activeViews: make(map[string]int),
	boostTimers: make(map[string]*time.Timer),
}

// AddView registers a new viewer for a host
func (vbt *ViewBoostTracker) AddView(hostName string) {
	vbt.mu.Lock()
	defer vbt.mu.Unlock()
	
	vbt.activeViews[hostName]++
	debugLog("View boost: Added viewer for host %s (total: %d)", hostName, vbt.activeViews[hostName])
	
	// Clear any existing cleanup timer since we have active viewers
	if timer, exists := vbt.boostTimers[hostName]; exists {
		timer.Stop()
		delete(vbt.boostTimers, hostName)
	}
}

// RemoveView unregisters a viewer for a host
func (vbt *ViewBoostTracker) RemoveView(hostName string) {
	vbt.mu.Lock()
	defer vbt.mu.Unlock()
	
	if vbt.activeViews[hostName] > 0 {
		vbt.activeViews[hostName]--
		debugLog("View boost: Removed viewer for host %s (remaining: %d)", hostName, vbt.activeViews[hostName])
		
		// If no more active viewers, start cleanup timer
		if vbt.activeViews[hostName] == 0 {
			// Clean up immediately
			delete(vbt.activeViews, hostName)
			debugLog("View boost: No more viewers for host %s, boost disabled", hostName)
		}
	}
}

// ShouldBoostHost returns true if this host should get boosted polling
func (vbt *ViewBoostTracker) ShouldBoostHost(hostName string) bool {
	vbt.mu.RLock()
	defer vbt.mu.RUnlock()
	return vbt.activeViews[hostName] > 0
}

func main() {
	addr := common.Env("DD_UI_BIND", ":443")
	
	// Show current log level on startup
	currentLevel := getLogLevel()
	infoLog("DDUI starting with log level: %s", currentLevel)
	debugLog("Debug logging is enabled")

	// Initialize auth from environment
	sessionManager, err := InitAuthFromEnv()
	if err != nil {
		fatalLog("OIDC setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := database.InitDBFromEnv(ctx); err != nil {
		fatalLog("DB init failed: %v", err)
	}
	if err := services.InitInventory(); err != nil {
		fatalLog("inventory init failed: %v", err)
	}

	// Start inventory file watcher for auto-reload
	services.StartInventoryWatcher(ctx)

	// kick off background auto-scanner (Portainer-ish cadence)
	startAutoScanner(ctx)
	startIacAutoScanner(ctx)

	r := makeRouter()
	
	// Wrap router with session middleware
	var handler http.Handler = r
	if sessionManager != nil {
		handler = sessionManager.LoadAndSave(r)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	enableTLS := isTrueish(common.Env("DD_UI_TLS_ENABLE", "true"))
	if !enableTLS {
		infoLog("http: listening on %s (TLS disabled)", addr)
		fatalLog("HTTP server error: %v", srv.ListenAndServe())
		return
	}

	certFile := strings.TrimSpace(common.Env("DD_UI_TLS_CERT_FILE", ""))
	keyFile := strings.TrimSpace(common.Env("DD_UI_TLS_KEY_FILE", ""))

	if certFile != "" && keyFile != "" {
		infoLog("https: listening on %s (cert=%s)", addr, certFile)
		fatalLog("HTTPS server error: %v", srv.ListenAndServeTLS(certFile, keyFile))
		return
	}

	if !isTrueish(common.Env("DD_UI_TLS_SELF_SIGNED", "true")) {
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
	v := strings.ToLower(common.Env(key, def))
	return v == "1" || v == "t" || v == "true" || v == "yes" || v == "on"
}
func envDur(key, def string) time.Duration {
	if d, err := time.ParseDuration(common.Env(key, def)); err == nil {
		return d
	}
	out, _ := time.ParseDuration(def)
	return out
}
func envInt(key string, def int) int {
	if s := common.Env(key, ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

// logLevel returns the current log level, defaulting to "info"
func getLogLevel() string {
	return strings.ToLower(common.Env("DD_UI_LOG_LEVEL", "info"))
}

// Use common logging functions (aliases for backward compatibility)
var (
	debugLog = common.DebugLog
	infoLog  = common.InfoLog
	warnLog  = common.WarnLog
	errorLog = common.ErrorLog
	fatalLog = common.FatalLog
)

// run one full pass across hosts with limited concurrency
func scanAllOnce(ctx context.Context, perHostTO time.Duration, conc int) {
	hostRows, err := database.ListHosts(ctx)
	if err != nil {
		errorLog("scan: list hosts failed: %v", err)
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
			n, err := services.ScanHostContainers(hctx, h.Name)
			cancel()

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				// treat intentional skips distinctly
				if errors.Is(err, services.ErrSkipScan) {
					skipped++
					return
				}
				failed++
				errorLog("scan: host=%s error=%v", h.Name, err)
				return
			}
			scanned++
			total += n
			infoLog("scan: host=%s saved=%d", h.Name, n)
		}()
	}
	wg.Wait()
	infoLog("scan: complete hosts=%d scanned=%d skipped=%d total_saved=%d errors=%d",
		len(hostRows), scanned, skipped, total, failed)
}

func startAutoScanner(ctx context.Context) {
	if !common.EnvBool("DD_UI_SCAN_DOCKER_AUTO", "true") {
		infoLog("scan: auto disabled (DD_UI_SCAN_DOCKER_AUTO=false)")
		return
	}
	baseInterval := envDur("DD_UI_SCAN_DOCKER_INTERVAL", "5s")       // Smart default: 5s
	boostInterval := 500 * time.Millisecond                         // View-based boost interval
	perHostTO := envDur("DD_UI_SCAN_DOCKER_HOST_TIMEOUT", "45s")     // per host protection
	conc := envInt("DD_UI_SCAN_DOCKER_CONCURRENCY", 3)

	infoLog("scan: smart scanner enabled base_interval=%s boost_interval=%s host_timeout=%s conc=%d", 
		baseInterval, boostInterval, perHostTO, conc)

	// optional boot scan
	if common.EnvBool("DD_UI_SCAN_DOCKER_ON_START", "true") {
		go scanAllOnce(ctx, perHostTO, conc)
	}

	// Start smart scanning loop with view-based boost
	go startSmartScanLoop(ctx, baseInterval, boostInterval, perHostTO, conc)
}

// startSmartScanLoop runs separate scan timers for each host with view-based boost
func startSmartScanLoop(ctx context.Context, baseInterval, boostInterval, perHostTO time.Duration, conc int) {
	// Track per-host timers
	hostTimers := make(map[string]*time.Timer)
	var timersMu sync.Mutex
	
	// Function to scan a specific host
	scanHost := func(hostName string) {
		// Scan this specific host
		if saved, err := services.ScanHostContainers(ctx, hostName); err != nil {
			if !errors.Is(err, services.ErrSkipScan) {
				debugLog("Smart scan failed for host %s: %v", hostName, err)
			}
		} else {
			debugLog("Smart scan completed for host %s: saved=%d containers", hostName, saved)
		}
	}
	
	// Function to schedule next scan for a host
	var scheduleHostScan func(string)
	scheduleHostScan = func(hostName string) {
		timersMu.Lock()
		defer timersMu.Unlock()
		
		// Stop existing timer if any
		if timer, exists := hostTimers[hostName]; exists {
			timer.Stop()
		}
		
		// Choose interval based on view boost
		interval := baseInterval
		if viewBoostTracker.ShouldBoostHost(hostName) {
			interval = boostInterval
		}
		
		// Schedule next scan
		hostTimers[hostName] = time.AfterFunc(interval, func() {
			select {
			case <-ctx.Done():
				return
			default:
				scanHost(hostName)
				scheduleHostScan(hostName) // Reschedule
			}
		})
	}
	
	// Initial scan and scheduling for all hosts
	hosts, err := database.ListHosts(ctx)
	if err != nil {
		debugLog("Smart scan failed to get initial hosts: %v", err)
		return
	}
	
	debugLog("Smart scan: Starting timers for %d hosts", len(hosts))
	for _, host := range hosts {
		scheduleHostScan(host.Name)
	}
	
	// Wait for context cancellation
	<-ctx.Done()
	debugLog("Smart scan: Stopping all host timers")
	
	// Clean up all timers
	timersMu.Lock()
	for hostName, timer := range hostTimers {
		timer.Stop()
		debugLog("Smart scan: Stopped timer for host %s", hostName)
	}
	timersMu.Unlock()
}

// ---- IaC auto-scan (local + apply) ----

func startIacAutoScanner(ctx context.Context) {
	if !common.EnvBool("DD_UI_SCAN_IAC_AUTO", "true") {
		infoLog("iac: auto disabled (DD_UI_SCAN_IAC_AUTO=false)")
		return
	}
	interval := envDur("DD_UI_SCAN_IAC_INTERVAL", "90s") // default 1m30s
	infoLog("iac: auto enabled interval=%s", interval)

	// initial scan on boot (non-fatal)
	go func() {
		if _, _, err := services.ScanIacLocal(ctx); err != nil {
			errorLog("iac: initial scan failed: %v", err)
		}
		if err := applyAutoDevOps(ctx); err != nil {
			errorLog("iac: initial apply failed: %v", err)
		}
	}()

	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if _, _, err := services.ScanIacLocal(ctx); err != nil {
					errorLog("iac: periodic scan failed: %v", err)
				}
				if err := applyAutoDevOps(ctx); err != nil {
					errorLog("iac: apply failed: %v", err)
				}
			case <-ctx.Done():
				infoLog("iac: auto scanner stopping: %v", ctx.Err())
				return
			}
		}
	}()
}

/* --- Auto DevOps evaluator:
     - Default disabled.
     - If .env DD_UI_DEVOPS_APPLY is present at stack > host > global, it overrides DB flag.
     - Else DB iac_enabled used.
     - When effective true: deploy stack (compose/script), which idempotently fixes drift & creates missing.
*/

func applyAutoDevOps(ctx context.Context) error {
	rows, err := common.DB.Query(ctx, `SELECT id FROM iac_stacks`)
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
		has, _ := services.StackHasContent(ctx, id)
		if !has {
			continue
		}

		// Respect the *effective* Auto DevOps policy (global env + DB overrides)
		allowed, err := services.ShouldAutoApply(ctx, id)
		if err != nil || !allowed {
			continue
		}

		// Kick the deploy (manual=false -> gated in deployStack, which is fine)
		dctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		_ = services.DeployStack(dctx, id) // best effort; idempotent for compose
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
