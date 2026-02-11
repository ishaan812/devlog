// Package auth implements the OAuth PKCE "Sign in with ChatGPT" flow
// used by the ChatGPT provider. The flow mirrors the one used by the
// OpenAI Codex CLI:
//
//  1. Generate PKCE codes (code_verifier / code_challenge).
//  2. Start a local HTTP server on localhost:1455.
//  3. Open the user's browser to https://auth.openai.com/oauth/authorize.
//  4. Receive the authorization code via the /auth/callback redirect.
//  5. Exchange the code for access_token + refresh_token.
//  6. Return the tokens so they can be persisted in config.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ── Constants ──────────────────────────────────────────────────────────────

const (
	// ClientID is the public OAuth client ID used by the Codex CLI.
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

	// Issuer is the OpenAI OAuth issuer base URL.
	Issuer = "https://auth.openai.com"

	// DefaultPort is the default local redirect port.
	DefaultPort = 1455

	// Scopes requested during the OAuth flow.
	oauthScope = "openid profile email offline_access"
)

// ── Public types ───────────────────────────────────────────────────────────

// ChatGPTTokens holds the tokens returned by the OAuth flow.
type ChatGPTTokens struct {
	AccessToken  string `json:"access_token"`  // ChatGPT session token (OAuth)
	RefreshToken string `json:"refresh_token"` // Used to refresh the session
	IDToken      string `json:"id_token"`      // JWT with user claims
	APIKey       string `json:"-"`             // Actual OpenAI API key (from token exchange)
}

// BearerToken returns the API key for OpenAI API calls.
func (t *ChatGPTTokens) BearerToken() string {
	return t.APIKey
}

// ── PKCE helpers ───────────────────────────────────────────────────────────

func generatePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 64)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])
	return verifier, challenge, nil
}

func generateState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ── Browser helper ─────────────────────────────────────────────────────────

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// ── Authorization URL ──────────────────────────────────────────────────────

func buildAuthorizeURL(redirectURI, challenge, state string) string {
	params := url.Values{
		"response_type":              {"code"},
		"client_id":                  {ClientID},
		"redirect_uri":               {redirectURI},
		"scope":                      {oauthScope},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"state":                      {state},
		"id_token_add_organizations": {"true"}, // Embeds organization info in id_token
		"codex_cli_simplified_flow":  {"true"},
	}
	return fmt.Sprintf("%s/oauth/authorize?%s", Issuer, params.Encode())
}

// ── Token exchange ─────────────────────────────────────────────────────────

func exchangeCodeForTokens(ctx context.Context, code, redirectURI, verifier string) (*ChatGPTTokens, error) {
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {ClientID},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", Issuer+"/oauth/token", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	var tokens ChatGPTTokens
	if err := json.Unmarshal(respBody, &tokens); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tokens, nil
}

// ── API key exchange (id_token → OpenAI API key) ──────────────────────────

// obtainAPIKey exchanges the id_token for an actual OpenAI API key via
// RFC 8693 token exchange. The resulting key has the model.request scope
// needed to call /v1/chat/completions.
func obtainAPIKey(ctx context.Context, idToken string) (string, error) {
	body := url.Values{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"client_id":          {ClientID},
		"requested_token":    {"openai-api-key"},
		"subject_token":      {idToken},
		"subject_token_type": {"urn:ietf:params:oauth:token-type:id_token"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", Issuer+"/oauth/token", strings.NewReader(body.Encode()))
	if err != nil {
		return "", fmt.Errorf("create api-key exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("api-key exchange request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read api-key exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api-key exchange returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse api-key exchange response: %w", err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("api-key exchange returned empty key")
	}

	return result.AccessToken, nil
}

// ── Token refresh ──────────────────────────────────────────────────────────

// RefreshAccessToken uses a refresh_token to obtain a new access_token.
func RefreshAccessToken(ctx context.Context, refreshToken string) (*ChatGPTTokens, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ClientID},
		"refresh_token": {refreshToken},
		"scope":         {oauthScope},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", Issuer+"/oauth/token", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh token returned %d: %s", resp.StatusCode, string(respBody))
	}

	var tokens ChatGPTTokens
	if err := json.Unmarshal(respBody, &tokens); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}
	return &tokens, nil
}

// ── Success HTML ───────────────────────────────────────────────────────────

const successHTML = `<!DOCTYPE html>
<html>
<head><title>DevLog — Signed in</title>
<style>
body{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#0d1117;color:#e6edf3}
.card{text-align:center;padding:2rem 3rem;border-radius:12px;background:#161b22;border:1px solid #30363d}
h1{color:#58a6ff;margin-bottom:.5rem}
p{color:#8b949e}
</style>
</head>
<body>
<div class="card">
<h1>Signed in to DevLog</h1>
<p>You can close this tab and return to your terminal.</p>
</div>
</body>
</html>`

// ── Main login flow ────────────────────────────────────────────────────────

// LoginWithChatGPT runs the full OAuth PKCE flow synchronously.
// It blocks until the user completes login in the browser (or ctx is canceled).
// Returns the auth tokens on success.
func LoginWithChatGPT(ctx context.Context) (*ChatGPTTokens, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	state, err := generateState()
	if err != nil {
		return nil, err
	}

	// Start local server
	addr := fmt.Sprintf("127.0.0.1:%d", DefaultPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("start login server on %s: %w", addr, err)
	}
	defer listener.Close()

	actualPort := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", actualPort)
	authURL := buildAuthorizeURL(redirectURI, challenge, state)

	// Channel for result
	type result struct {
		tokens *ChatGPTTokens
		err    error
	}
	resultCh := make(chan result, 1)

	var once sync.Once
	sendResult := func(r result) {
		once.Do(func() { resultCh <- r })
	}

	// HTTP handler
	mux := http.NewServeMux()

	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Validate state
		if q.Get("state") != state {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			sendResult(result{err: fmt.Errorf("OAuth state mismatch")})
			return
		}

		code := q.Get("code")
		if code == "" {
			errMsg := q.Get("error_description")
			if errMsg == "" {
				errMsg = q.Get("error")
			}
			if errMsg == "" {
				errMsg = "missing authorization code"
			}
			http.Error(w, errMsg, http.StatusBadRequest)
			sendResult(result{err: fmt.Errorf("OAuth error: %s", errMsg)})
			return
		}

		// Exchange code for OAuth tokens
		tokens, err := exchangeCodeForTokens(r.Context(), code, redirectURI, verifier)
		if err != nil {
			http.Error(w, "Token exchange failed", http.StatusInternalServerError)
			sendResult(result{err: fmt.Errorf("token exchange: %w", err)})
			return
		}

		// Exchange id_token for an actual OpenAI API key.
		// This requires organization_id in the id_token, which is only
		// available on Plus, Pro, Team, and Enterprise plans.
		apiKey, apiKeyErr := obtainAPIKey(r.Context(), tokens.IDToken)
		if apiKeyErr != nil {
			http.Error(w, "Your plan does not support API access", http.StatusForbidden)
			sendResult(result{err: fmt.Errorf(
				"your ChatGPT plan (Free/Go) does not support API access.\n" +
					"  Please upgrade to Plus, Pro, Team, or Enterprise,\n" +
					"  or use the OpenAI provider with an API key from https://platform.openai.com/api-keys")})
			return
		}
		tokens.APIKey = apiKey

		// Redirect to success page
		http.Redirect(w, r, fmt.Sprintf("http://localhost:%d/success", actualPort), http.StatusFound)
		sendResult(result{tokens: tokens})
	})

	mux.HandleFunc("/success", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, successHTML)
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Serve in background
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			sendResult(result{err: fmt.Errorf("login server: %w", err)})
		}
	}()

	// Open browser
	if err := openBrowser(authURL); err != nil {
		// Non-fatal: user can open the URL manually
		fmt.Printf("Could not open browser: %v\nOpen this URL manually:\n%s\n", err, authURL)
	}

	// Wait for result or context cancellation
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return nil, ctx.Err()
	case res := <-resultCh:
		// Give a moment for the success page redirect to complete
		time.Sleep(500 * time.Millisecond)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return res.tokens, res.err
	}
}

// GetAuthURL returns the OAuth authorization URL without starting a server.
// Useful for displaying the URL to the user in the TUI.
func GetAuthURL(port int) (authURL string, err error) {
	_, challenge, err := generatePKCE()
	if err != nil {
		return "", err
	}
	state, err := generateState()
	if err != nil {
		return "", err
	}
	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", port)
	return buildAuthorizeURL(redirectURI, challenge, state), nil
}
