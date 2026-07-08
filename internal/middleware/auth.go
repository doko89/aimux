package middleware

import (
	"crypto/subtle"
	"strings"
)

// ValidateAPIKey checks the provided key against the allowed set. An empty
// allowed set permits all keys (useful for local development).
// Comparison is constant-time to avoid timing side-channels.
func ValidateAPIKey(key string, validKeys []string) bool {
	if len(validKeys) == 0 {
		return true
	}
	trimmed := strings.TrimSpace(key)
	for _, k := range validKeys {
		candidate := strings.TrimSpace(k)
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(trimmed)) == 1 {
			return true
		}
	}
	return false
}
