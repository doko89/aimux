package login

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenAI Codex OAuth constants (from CommonsProxy / opencode / codex-openai-proxy).
const (
	codexClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexIssuer      = "https://auth.openai.com"
	codexUsercodeURL = codexIssuer + "/api/accounts/deviceauth/usercode"
	codexPollURL     = codexIssuer + "/api/accounts/deviceauth/token"
	codexTokenURL    = codexIssuer + "/oauth/token"
	codexDeviceURL   = codexIssuer + "/codex/device"
	codexRedirectURI = codexIssuer + "/deviceauth/callback"
	codexUserAgent   = "aimux/0.1.0"
)

// DeviceAuthResponse is the response from the device authorization endpoint.
type DeviceAuthResponse struct {
	DeviceAuthID    string `json:"device_auth_id"`
	UserCode        string `json:"user_code"`
	Interval        string `json:"interval"`
	VerificationURI string `json:"verification_uri"`
}

// DeviceCodePollResponse is the response when polling succeeds (user authorized).
type DeviceCodePollResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeChallenge     string `json:"code_challenge"`
	CodeVerifier      string `json:"code_verifier"`
}

// CodexTokenResponse is the final OAuth token response.
type CodexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// InitiateDeviceAuth starts the OAuth device code flow and returns the
// device auth info needed to complete login.
func InitiateDeviceAuth() (*DeviceAuthResponse, error) {
	payload := map[string]string{
		"client_id": codexClientID,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, codexUsercodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("device auth request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", codexUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("device auth returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result DeviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode device auth response: %w", err)
	}
	return &result, nil
}

// PollForToken polls the Codex device auth endpoint until the user completes
// login, then exchanges the authorization code for OAuth tokens.
func PollForToken(deviceAuthID, userCode string, intervalSec int) (*CodexTokenResponse, error) {
	if intervalSec < 1 {
		intervalSec = 5
	}

	maxAttempts := 120 // ~10 minutes max
	for i := 0; i < maxAttempts; i++ {
		// Step 2: Poll for authorization code.
		payload := map[string]string{
			"device_auth_id": deviceAuthID,
			"user_code":      userCode,
		}
		body, _ := json.Marshal(payload)

		pollReq, err := http.NewRequest(http.MethodPost, codexPollURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("token poll request failed: %w", err)
		}
		pollReq.Header.Set("Content-Type", "application/json")
		pollReq.Header.Set("User-Agent", codexUserAgent)

		resp, err := http.DefaultClient.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("token poll request failed: %w", err)
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()

		// 403/404 means still pending.
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("poll returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}

		// Got 200 OK — try parsing as authorization code response.
		var pollResp DeviceCodePollResponse
		if err := json.Unmarshal(respBody, &pollResp); err == nil && pollResp.AuthorizationCode != "" {
			// Step 3: Exchange authorization code for tokens.
			return exchangeCodeForTokens(pollResp.AuthorizationCode, pollResp.CodeVerifier)
		}

		// Try parsing as direct tokens (some endpoints may return tokens directly).
		var directTokens CodexTokenResponse
		if err := json.Unmarshal(respBody, &directTokens); err == nil && directTokens.AccessToken != "" {
			return &directTokens, nil
		}

		// Unknown response format — retry.
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}

	return nil, fmt.Errorf("timed out waiting for login (no response after %d attempts)", maxAttempts)
}

// exchangeCodeForTokens performs the OAuth token exchange using the
// authorization code and code verifier from the device auth flow.
func exchangeCodeForTokens(authCode, codeVerifier string) (*CodexTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", codexClientID)
	form.Set("code", authCode)
	form.Set("redirect_uri", codexRedirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest(http.MethodPost, codexTokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", codexUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if len(respBody) == 0 {
		return nil, fmt.Errorf("token exchange returned empty body (status %d)", resp.StatusCode)
	}

	var tokenResp CodexTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token exchange response: %w (body: %s)", err, string(respBody))
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token after exchange: %s", string(respBody))
	}
	return &tokenResp, nil
}

// extractAccountID extracts the ChatGPT account ID from JWT claims.
func extractAccountID(idToken string) string {
	claims := parseJWTClaims(idToken)
	if claims == nil {
		return ""
	}
	if v, ok := claims["chatgpt_account_id"].(string); ok && v != "" {
		return v
	}
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]interface{}); ok {
		if v, ok := auth["chatgpt_account_id"].(string); ok && v != "" {
			return v
		}
	}
	if orgs, ok := claims["organizations"].([]interface{}); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]interface{}); ok {
			if v, ok := org["id"].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// parseJWTClaims decodes the JWT payload and returns the claims map.
func parseJWTClaims(token string) map[string]interface{} {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil
	}
	return claims
}
