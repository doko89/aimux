package middleware

import "strings"

// ValidateAPIKey checks the provided key against the allowed set. An empty
// allowed set permits all keys (useful for local development).
func ValidateAPIKey(key string, validKeys []string) bool {
	if len(validKeys) == 0 {
		return true
	}
	for _, k := range validKeys {
		if strings.TrimSpace(k) == key {
			return true
		}
	}
	return false
}
