package login

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ChatGPTAuth holds the OAuth tokens for ChatGPT/Codex authentication.
type ChatGPTAuth struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	AuthMode     string    `json:"auth_mode"`
}

// TokenDir returns ~/.aimux, creating it if necessary.
func TokenDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot get home dir: %w", err)
	}
	dir := filepath.Join(home, ".aimux")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("cannot create config dir: %w", err)
	}
	return dir, nil
}

// authPath returns the full path to chatgpt-auth.json.
func authPath() (string, error) {
	dir, err := TokenDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "chatgpt-auth.json"), nil
}

// SaveChatGPTAuth persists the ChatGPT auth tokens to disk.
func SaveChatGPTAuth(auth *ChatGPTAuth) error {
	path, err := authPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadChatGPTAuth reads the ChatGPT auth tokens from disk.
// Returns nil, nil if no file exists.
func LoadChatGPTAuth() (*ChatGPTAuth, error) {
	path, err := authPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var auth ChatGPTAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("corrupt auth file: %w", err)
	}
	return &auth, nil
}

// IsExpired reports whether the access token is expired (or will expire
// within a 5-minute safety window).
func (a *ChatGPTAuth) IsExpired() bool {
	return time.Now().After(a.ExpiresAt.Add(-5 * time.Minute))
}

// AuthFilePath returns the path to the auth file (for display purposes).
func AuthFilePath() string {
	path, _ := authPath()
	return path
}

// ExtractAccountIDFromToken is a public wrapper around extractAccountID.
func ExtractAccountIDFromToken(idToken string) string {
	return extractAccountID(idToken)
}
