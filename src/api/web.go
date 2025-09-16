// src/api/web.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
)

// CommentInfo stores information about comments and their positions in dotenv files
type CommentInfo struct {
	LineNumber int    `json:"lineNumber"`
	Content    string `json:"content"`
}

// DotenvComments stores all comment metadata for a dotenv file
type DotenvComments struct {
	Comments []CommentInfo `json:"comments"`
}

// parseDotenvWithComments extracts comments and their positions, returns cleaned content for SOPS
func parseDotenvWithComments(content string) (cleanedContent string, comments DotenvComments) {
	debugLog("SOPS: parseDotenvWithComments called with %d bytes", len(content))
	
	// Normalize line endings: convert \r\n to \n and remove standalone \r
	normalizedContent := strings.ReplaceAll(content, "\r\n", "\n")
	normalizedContent = strings.ReplaceAll(normalizedContent, "\r", "\n")
	
	lines := strings.Split(normalizedContent, "\n")
	var cleanedLines []string
	var commentInfos []CommentInfo
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Track comments and their line numbers (but skip purely empty lines)
		if strings.HasPrefix(trimmed, "#") {
			commentInfos = append(commentInfos, CommentInfo{
				LineNumber: i,
				Content:    line, // preserve original spacing
			})
			continue
		}
		
		// Skip empty lines entirely - don't preserve them as comments
		if trimmed == "" {
			continue
		}
		
		// Keep lines that look like KEY=VALUE
		if strings.Contains(trimmed, "=") {
			cleanedLines = append(cleanedLines, line)
		} else {
			// Treat malformed lines as comments too
			commentInfos = append(commentInfos, CommentInfo{
				LineNumber: i,
				Content:    line,
			})
		}
	}
	
	// Check if original content ended with a newline to preserve that behavior
	cleanedContent = strings.Join(cleanedLines, "\n")
	if strings.HasSuffix(normalizedContent, "\n") && !strings.HasSuffix(cleanedContent, "\n") {
		cleanedContent += "\n"
	}
	
	debugLog("SOPS: parseDotenvWithComments returning - original had trailing newline: %v, output has trailing newline: %v", 
		strings.HasSuffix(normalizedContent, "\n"), strings.HasSuffix(cleanedContent, "\n"))
		
	return cleanedContent, DotenvComments{Comments: commentInfos}
}

// reconstructDotenvWithComments merges decrypted content with preserved comments
func reconstructDotenvWithComments(cleanContent string, comments DotenvComments) string {
	if len(comments.Comments) == 0 {
		return cleanContent
	}
	
	// Normalize line endings in clean content too
	normalizedClean := strings.ReplaceAll(cleanContent, "\r\n", "\n")
	normalizedClean = strings.ReplaceAll(normalizedClean, "\r", "\n")
	
	// Check if original clean content ended with a newline
	endsWithNewline := strings.HasSuffix(normalizedClean, "\n")
	
	cleanLines := strings.Split(strings.TrimSuffix(normalizedClean, "\n"), "\n")
	var result []string
	
	// Create a map of line numbers to comments for quick lookup
	commentMap := make(map[int]string)
	for _, comment := range comments.Comments {
		commentMap[comment.LineNumber] = comment.Content
	}
	
	// Find the maximum line number to determine final size
	maxLine := 0
	for lineNum := range commentMap {
		if lineNum > maxLine {
			maxLine = lineNum
		}
	}
	
	// Reconstruct the file line by line
	cleanIndex := 0
	for i := 0; i <= maxLine; i++ {
		if commentContent, isComment := commentMap[i]; isComment {
			result = append(result, commentContent)
		} else if cleanIndex < len(cleanLines) && cleanLines[cleanIndex] != "" {
			// Only add non-empty clean lines to avoid duplicating empty lines
			result = append(result, cleanLines[cleanIndex])
			cleanIndex++
		} else if cleanIndex < len(cleanLines) {
			// Skip empty clean lines since they should be represented as comments
			cleanIndex++
		}
	}
	
	// Add any remaining clean lines
	for cleanIndex < len(cleanLines) {
		if cleanLines[cleanIndex] != "" || cleanIndex == len(cleanLines)-1 {
			result = append(result, cleanLines[cleanIndex])
		}
		cleanIndex++
	}
	
	// Join and preserve original trailing newline behavior
	reconstructed := strings.Join(result, "\n")
	if endsWithNewline && !strings.HasSuffix(reconstructed, "\n") {
		reconstructed += "\n"
	}
	
	return reconstructed
}

// normalizeFileContent normalizes line endings and handles trailing newlines consistently
func normalizeFileContent(content string) string {
	// Normalize line endings: convert \r\n to \n and remove standalone \r
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	
	// Don't add extra trailing newlines - preserve original behavior
	return normalized
}

type Health struct {
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	Edition   string    `json:"edition"`
}

func makeRouter() http.Handler {
	r := chi.NewRouter()

	// CORS – locked down for credentials
	uiOrigin := strings.TrimSpace(env("DDUI_UI_ORIGIN", ""))
	allowedOrigins := []string{}
	if uiOrigin != "" {
		allowedOrigins = append(allowedOrigins, uiOrigin)
	}
	// dev helpers
	allowedOrigins = append(allowedOrigins,
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins, // no "*"
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "X-Confirm-Reveal"},
		AllowCredentials: true,
		MaxAge:           600,
	}))

	// -------- API
	r.Route("/api", func(api chi.Router) {
		api.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, Health{Status: "ok", StartedAt: startedAt, Edition: "Community"})
		})

		// Session probe MUST be public
		api.Get("/session", SessionHandler)

		// Everything below requires auth
		api.Group(func(priv chi.Router) {
			priv.Use(RequireAuth)





			// Host-scoped Stack CRUD operations



			// Create a new stack for a host

			// Group-scoped Stack CRUD operations



			// Host-scoped File operations




			// Group-scoped File operations




			// Host-scoped Deployment operations




			// Group-scoped Deployment operations




			// Force IaC scan (local)




			// Direct SSH command execution on hosts


			// Create a new stack in the local IaC repo

			// Get a single IaC stack (returns effective auto devops)

			// Patch IaC stack (no implicit override writes)

			// Delete a stack (optionally delete files too)

			// ===== IaC Editor APIs =====

			// List files tracked for a stack

			// Read file content for a stack file (with optional decrypt)

			// Create/update file content

			// Delete a file from a stack

			// Deploy endpoint
			// - Manual deploys: **default** (for UI). Pass ?auto=1 for background/auto callers.

			// Streaming deploy endpoint (compatible with existing frontend)

			// Alternative streaming deploy endpoint using scope/stack name

			// Check if configuration has changed endpoint

			// Confirmation endpoint for deploying unchanged configuration

			setupIacRoutes(priv)

			// Docker operations routes (organized in routes_docker.go) 
			setupDockerRoutes(priv)

			// Docker cleanup endpoints (organized in routes/cleanup.go)
			setupCleanupRoutes(priv)

			// System management routes (organized in routes_system.go)
			setupSystemRoutes(priv)

			// SSH operation routes (organized in routes_ssh.go)
			setupSshRoutes(priv)

			// GitOps configuration routes (organized in routes_gitops.go)
			setupGitopsRoutes(priv)
		})
	})

	// Legacy alias
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, Health{Status: "ok", StartedAt: startedAt, Edition: "Community"})
	})

	// -------- Auth endpoints (must come BEFORE SPA fallback)
	r.Get("/login", LoginHandler)
	r.Get("/auth/login", LoginHandler) // alias
	r.Get("/auth/callback", CallbackHandler)
	r.Post("/logout", LogoutHandler)
	r.Post("/auth/logout", LogoutHandler) // alias

	// -------- Static SPA (Vite)
	uiRoot := env("DDUI_UI_DIR", "/app/ui/dist")
	fs := http.FileServer(http.Dir(uiRoot))

	// Serve built assets directly
	r.Get("/assets/*", func(w http.ResponseWriter, req *http.Request) {
		fs.ServeHTTP(w, req)
	})

	// SPA fallback (last)
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api") || strings.HasPrefix(req.URL.Path, "/auth") {
			http.NotFound(w, req)
			return
		}
		path := filepath.Join(uiRoot, filepath.Clean(strings.TrimPrefix(req.URL.Path, "/")))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, req, path)
			return
		}
		http.ServeFile(w, req, filepath.Join(uiRoot, "index.html"))
	})

	return r
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func parseDurationDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

// safe path join under repo root
func joinUnder(root, rel string) (string, error) {
	clean := filepath.Clean("/" + rel) // force absolute-clean then strip
	clean = strings.TrimPrefix(clean, "/")
	full := filepath.Join(root, clean)
	r, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(r, "..") {
		return "", errors.New("outside root")
	}
	return full, nil
}

// --- Image repo/tag persistence helpers ---

func upsertImageTag(ctx context.Context, host, id, repo, tag string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO image_tags (host_name, image_id, repo, tag)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (host_name, image_id)
		DO UPDATE SET repo=EXCLUDED.repo, tag=EXCLUDED.tag, last_seen=now();
	`, host, id, repo, tag)
	return err
}

func getImageTagMap(ctx context.Context, host string) (map[string][2]string, error) {
	rows, err := db.Query(ctx, `SELECT image_id, repo, tag FROM image_tags WHERE host_name=$1`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][2]string)
	for rows.Next() {
		var id, repo, tag string
		if err := rows.Scan(&id, &repo, &tag); err != nil {
			return nil, err
		}
		out[id] = [2]string{repo, tag}
	}
	return out, nil
}

func cleanupImageTags(ctx context.Context, host string, keepIDs map[string]struct{}) error {
	ids := make([]string, 0, len(keepIDs))
	for id := range keepIDs {
		ids = append(ids, id)
	}
	_, err := db.Exec(ctx, `
		DELETE FROM image_tags t
		WHERE t.host_name = $1
		  AND NOT EXISTS (
		    SELECT 1
		    FROM UNNEST($2::text[]) AS u(id)
		    WHERE u.id = t.image_id
		  );
	`, host, ids)
	return err
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strings.TrimSuffix(strings.TrimSpace(
		strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64)), ".0") + " " + string("KMGTPE"[exp]) + "B"
}

/* ---------------- DevOps Apply helpers ---------------- */

func isTrueish(s string) bool {
	if s == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	}
	return false
}

// app_settings: simple KV store
func getAppSetting(ctx context.Context, key string) (string, bool) {
	var v string
	err := db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}
func setAppSetting(ctx context.Context, key, value string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()
	`, key, value)
	return err
}
func delAppSetting(ctx context.Context, key string) error {
	_, err := db.Exec(ctx, `DELETE FROM app_settings WHERE key=$1`, key)
	return err
}
func getAppSettingBool(ctx context.Context, key string) (*bool, bool) {
	if s, ok := getAppSetting(ctx, key); ok {
		b := isTrueish(s)
		return &b, true
	}
	return nil, false
}

// Global DevOps Apply (Auto DevOps) – DB override with ENV fallback
func getGlobalDevopsApply(ctx context.Context) (bool, string) {
	if b, ok := getAppSettingBool(ctx, "devops_apply"); ok && b != nil {
		return *b, "db"
	}
	return envBool("DDUI_DEVOPS_APPLY", "false"), "env"
}
func setGlobalDevopsApply(ctx context.Context, v *bool) error {
	if v == nil {
		return delAppSetting(ctx, "devops_apply")
	}
	if *v {
		return setAppSetting(ctx, "devops_apply", "true")
	}
	return setAppSetting(ctx, "devops_apply", "false")
}

// host_settings: per-host overrides
func getHostDevopsOverride(ctx context.Context, host string) (*bool, error) {
	var val *bool
	err := db.QueryRow(ctx, `SELECT auto_apply_override FROM host_settings WHERE host_name=$1`, host).Scan(&val)
	if err != nil {
		return nil, nil // treat as absent
	}
	return val, nil
}
func setHostDevopsOverride(ctx context.Context, host string, v *bool) error {
	if v == nil {
		_, err := db.Exec(ctx, `DELETE FROM host_settings WHERE host_name=$1`, host)
		return err
	}
	_, err := db.Exec(ctx, `
		INSERT INTO host_settings (host_name, auto_apply_override)
		VALUES ($1,$2)
		ON CONFLICT (host_name) DO UPDATE SET auto_apply_override=EXCLUDED.auto_apply_override, updated_at=now()
	`, host, *v)
	return err
}

// group_settings: per-group overrides
func getGroupDevopsOverride(ctx context.Context, group string) (*bool, error) {
	var val *bool
	err := db.QueryRow(ctx, `SELECT auto_apply_override FROM group_settings WHERE group_name=$1`, group).Scan(&val)
	if err != nil {
		return nil, nil // treat as absent
	}
	return val, nil
}
func setGroupDevopsOverride(ctx context.Context, group string, v *bool) error {
	if v == nil {
		_, err := db.Exec(ctx, `DELETE FROM group_settings WHERE group_name=$1`, group)
		return err
	}
	_, err := db.Exec(ctx, `
		INSERT INTO group_settings (group_name, auto_apply_override)
		VALUES ($1,$2)
		ON CONFLICT (group_name) DO UPDATE SET auto_apply_override=EXCLUDED.auto_apply_override, updated_at=now()
	`, group, *v)
	return err
}

/* ---------- SSE + WS helpers ---------- */

type sseLineWriter struct {
	mu     sync.Mutex
	w      http.ResponseWriter
	fl     http.Flusher
	stream string // "stdout" | "stderr"
	buf    []byte
}

func (s *sseLineWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	for {
		i := -1
		for j, b := range s.buf {
			if b == '\n' {
				i = j
				break
			}
		}
		if i == -1 {
			break
		}
		line := string(s.buf[:i])
		s.buf = s.buf[i+1:]
		_, _ = s.w.Write([]byte("event: " + s.stream + "\n"))
		_, _ = s.w.Write([]byte("data: " + line + "\n\n"))
		if s.fl != nil {
			s.fl.Flush()
		}
	}
	return len(p), nil
}

func writeSSEHeader(w http.ResponseWriter) (http.Flusher, bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Disable proxy buffering (nginx)
	w.Header().Set("X-Accel-Buffering", "no")
	fl, ok := w.(http.Flusher)
	return fl, ok
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		// allow same-origin and configured UI origin
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		ui := strings.TrimSpace(env("DDUI_UI_ORIGIN", ""))
		if origin == "" || origin == ui {
			return true
		}
		// dev helpers
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			return true
		}
		return false
	},
}


// cleanupEmptyDirs removes empty directories recursively up to but not including root
func cleanupEmptyDirs(path, root string) error {
	if path == root || path == "" || path == "/" {
		return nil
	}
	
	dir := filepath.Dir(path)
	if dir == path || dir == root {
		return nil
	}
	
	// Check if directory is empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // Directory might not exist, ignore
	}
	
	if len(entries) == 0 {
		debugLog("cleanupEmptyDirs: removing empty dir %s", dir)
		if err := os.Remove(dir); err != nil {
			return err
		}
		// Recursively check parent directories
		return cleanupEmptyDirs(dir, root)
	}
	
	return nil
}

// cleanupAssociatedCommentFile removes the .comments.json file if it exists for the given IaC file
func cleanupAssociatedCommentFile(filePath string) {
	if strings.HasSuffix(filePath, ".env") {
		commentFile := filePath + ".comments.json"
		if err := os.Remove(commentFile); err == nil {
			debugLog("Cleaned up comment file: %s", commentFile)
		}
	}
}

// cleanupEmptyStackAfterFileDeletion removes a stack from database if it has no content after file deletion
func cleanupEmptyStackAfterFileDeletion(ctx context.Context, stackID int64, root string) error {
	hasContent, err := stackHasContent(ctx, stackID)
	if err != nil {
		return err
	}
	
	if !hasContent {
		debugLog("cleanupEmptyStackAfterFileDeletion: removing empty stack id=%d", stackID)
		// Get stack path for directory cleanup
		var relPath string
		_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&relPath)
		
		// Delete the stack from database
		_, err := db.Exec(ctx, `DELETE FROM iac_stacks WHERE id=$1`, stackID)
		if err != nil {
			return err
		}
		
		// Clean up empty directories
		if relPath != "" {
			stackDir := filepath.Join(root, relPath)
			return cleanupEmptyDirs(stackDir, root)
		}
	}
	
	return nil
}
