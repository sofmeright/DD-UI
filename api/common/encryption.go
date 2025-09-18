package common

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// DecryptIfNeeded attempts to decrypt a value if it looks encrypted
// Returns the original value if not encrypted or if decryption fails
func DecryptIfNeeded(value string) (string, error) {
	// Check if value looks like it's SOPS encrypted
	if !strings.Contains(value, "ENC[") {
		return value, nil
	}

	// Check if SOPS is available
	if _, err := exec.LookPath("sops"); err != nil {
		return value, nil // SOPS not available, return as-is
	}

	// Create temp file with encrypted content
	tmpFile, err := os.CreateTemp("", "ddui-decrypt-*.txt")
	if err != nil {
		return value, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(value); err != nil {
		return value, err
	}
	tmpFile.Close()

	// Decrypt with SOPS
	cmd := exec.Command("sops", "-d", tmpFile.Name())
	output, err := cmd.Output()
	if err != nil {
		return value, nil // Decryption failed, return original
	}

	return strings.TrimSpace(string(output)), nil
}

// EncryptIfAvailable attempts to encrypt a value with SOPS if available
// Returns the original value if SOPS is not available
func EncryptIfAvailable(value string) (string, error) {
	// Check if SOPS is available
	if _, err := exec.LookPath("sops"); err != nil {
		return value, nil // SOPS not available, return as-is
	}

	// Check if SOPS age key is configured
	ageKey := Env("SOPS_AGE_KEY", "")
	ageKeyFile := Env("SOPS_AGE_KEY_FILE", "")
	if ageKey == "" && ageKeyFile == "" {
		return value, nil // No encryption key configured
	}

	// Create temp file with plain content
	tmpFile, err := os.CreateTemp("", "ddui-encrypt-*.txt")
	if err != nil {
		return value, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(value); err != nil {
		return value, err
	}
	tmpFile.Close()

	// Get age recipients
	recipients := Env("SOPS_AGE_RECIPIENTS", "")
	if recipients == "" {
		// Try to derive from key if available
		if ageKey != "" {
			// Extract public key from age key (simplified - in production would parse properly)
			parts := strings.Fields(ageKey)
			if len(parts) > 0 && strings.HasPrefix(parts[0], "AGE-SECRET-KEY-") {
				// For now, return unencrypted as we'd need the public key
				return value, nil
			}
		}
		return value, nil
	}

	// Encrypt with SOPS
	cmd := exec.Command("sops", "-e", 
		"--age", recipients,
		"--encrypted-regex", "^.*$",
		tmpFile.Name())
	
	output, err := cmd.Output()
	if err != nil {
		return value, nil // Encryption failed, return original
	}

	return strings.TrimSpace(string(output)), nil
}


// EnvInt returns an environment variable as an integer
func EnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}