package common

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

// Log levels for hierarchical logging
const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var logLevels = map[string]int{
	"debug": LevelDebug,
	"info":  LevelInfo,
	"warn":  LevelWarn,
	"error": LevelError,
	"fatal": LevelFatal,
}

// shouldLog determines if a message at the given level should be logged
func shouldLog(level string) bool {
	currentLevel := Env("DD_UI_LOG_LEVEL", "info")
	
	currentLevelNum, ok1 := logLevels[strings.ToLower(currentLevel)]
	targetLevelNum, ok2 := logLevels[strings.ToLower(level)]
	
	if !ok1 || !ok2 {
		return true // Default to logging if unknown level
	}
	
	return targetLevelNum >= currentLevelNum
}

// logOutput handles both text and JSON output based on DD_UI_LOG_FORMAT
func logOutput(level string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	
	// Ensure no secrets are accidentally logged
	message = sanitizeForLogging(message)
	
	if Env("DD_UI_LOG_FORMAT", "text") == "json" {
		// JSON format for Loki/Grafana
		entry := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"level":     strings.ToLower(level), // Lowercase for Loki auto-detection
			"message":   message,
		}
		if jsonBytes, err := json.Marshal(entry); err == nil {
			fmt.Println(string(jsonBytes))
		} else {
			// Fallback to text if JSON fails
			fmt.Printf("%s: %s\n", level, message)
		}
	} else {
		// Standard text format with timestamp for consistency
		if level == "FATAL" {
			log.Printf("%s: %s", level, message)
		} else {
			fmt.Printf("%s/%s %s: %s\n", 
				time.Now().Format("2006/01/02"),
				time.Now().Format("15:04:05"),
				level, message)
		}
	}
}

// DebugLog logs debug messages only if log level allows it
func DebugLog(format string, args ...interface{}) {
	if shouldLog("debug") {
		logOutput("DEBUG", format, args...)
	}
}

// InfoLog logs info messages only if log level allows it
func InfoLog(format string, args ...interface{}) {
	if shouldLog("info") {
		logOutput("INFO", format, args...)
	}
}

// WarnLog logs warning messages only if log level allows it
func WarnLog(format string, args ...interface{}) {
	if shouldLog("warn") {
		logOutput("WARN", format, args...)
	}
}

// ErrorLog logs error messages only if log level allows it
func ErrorLog(format string, args ...interface{}) {
	if shouldLog("error") {
		logOutput("ERROR", format, args...)
	}
}

// FatalLog logs fatal messages and exits (always shown)
func FatalLog(format string, args ...interface{}) {
	if Env("DD_UI_LOG_FORMAT", "text") == "json" {
		message := fmt.Sprintf(format, args...)
		entry := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"level":     "fatal", // Lowercase for Loki auto-detection
			"message":   message,
		}
		if jsonBytes, err := json.Marshal(entry); err == nil {
			fmt.Println(string(jsonBytes))
		}
	} else {
		log.Fatalf("FATAL: "+format, args...)
	}
	os.Exit(1)
}

// LogCommandOutput logs command output in a structured way
// It only logs in debug mode to prevent sensitive data exposure
func LogCommandOutput(level, prefix string, output []byte) {
	// Only log command output at debug level to prevent sensitive data exposure
	if !shouldLog("debug") {
		return
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	// Limit output to prevent log flooding
	maxLines := 20
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], fmt.Sprintf("... %d more lines truncated ...", len(lines)-maxLines))
	}
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			DebugLog("%s: %s", prefix, line)
		}
	}
}

// sanitizeForLogging removes potential secrets from any string before logging
func sanitizeForLogging(line string) string {
	// Check if any protected environment variable values are in the string
	protectedEnvVars := []string{
		"SOPS_AGE_KEY",
		"SOPS_AGE_RECIPIENTS",
		"SSH_KEY",
		"OIDC_CLIENT_SECRET",
		"OIDC_CLIENT_ID",
		"DD_UI_SESSION_SECRET",  // Correct spelling
		"DD_UI_DB_PASS",
		"DD_UI_DB_DSN",
		"DDUI_SESSION_SECRET",   // Keep for backwards compatibility
		"JWT_SECRET",
		"AUTH_SECRET",
		"DB_PASSWORD",
		"POSTGRES_PASSWORD",
		"MYSQL_PASSWORD",
		"REDIS_PASSWORD",
	}
	
	for _, envVar := range protectedEnvVars {
		if value := os.Getenv(envVar); value != "" && value != "true" && value != "false" {
			// Replace the actual value with REDACTED
			line = strings.ReplaceAll(line, value, "***REDACTED***")
		}
		// Also check _FILE variants
		fileVar := envVar + "_FILE"
		if fileContent := os.Getenv(fileVar); fileContent != "" {
			line = strings.ReplaceAll(line, fileContent, "***REDACTED***")
		}
	}
	
	// Patterns that might contain secrets
	patterns := []string{
		`(?i)(password|passwd|pwd|secret|key|token|api[-_]?key|auth|credential|bearer)[-_=:\s]*[^\s]+`,
		`(?i)(mysql|postgres|postgresql|mongodb|redis|amqp|mongodb\+srv)://[^@]+@[^\s]+`,
		`[a-zA-Z0-9]{40,}`, // Long strings that might be keys/tokens (increased from 20 to 40 to reduce false positives)
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		line = re.ReplaceAllStringFunc(line, func(match string) string {
			// Keep the label but redact the value
			parts := strings.SplitN(match, "=", 2)
			if len(parts) == 2 {
				return parts[0] + "=***REDACTED***"
			}
			parts = strings.SplitN(match, ":", 2)
			if len(parts) == 2 {
				return parts[0] + ":***REDACTED***"
			}
			return "***REDACTED***"
		})
	}
	return line
}

// LogCommandError logs a command error WITHOUT exposing sensitive data
func LogCommandError(prefix string, err error, output []byte) {
	// Log the error without the output to prevent sensitive data exposure
	ErrorLog("%s: command failed: %v", prefix, err)
	
	// In debug mode, log sanitized output only
	if shouldLog("debug") && len(output) > 0 {
		DebugLog("%s: Output available in debug mode (sanitized)", prefix)
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		
		// Only show first 3 lines even in debug, and sanitize them
		maxLines := 3
		for i := 0; i < len(lines) && i < maxLines; i++ {
			sanitized := sanitizeForLogging(strings.TrimSpace(lines[i]))
			DebugLog("%s [line %d]: %s", prefix, i+1, sanitized)
		}
		
		if len(lines) > maxLines {
			DebugLog("%s: ... %d more lines omitted for security ...", prefix, len(lines)-maxLines)
		}
	}
}