package epic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	t.Parallel()

	client := NewClient("client-id", "secret")
	got := client.AuthorizeURL("http://localhost:8080/auth/epic/callback", "state123")

	for _, want := range []string{
		"www.epicgames.com/id/authorize",
		"client_id=client-id",
		"response_type=code",
		"redirect_uri=http",
		"scope=basic_profile",
		"state=state123",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AuthorizeURL() = %q, want substring %q", got, want)
		}
	}
}

func TestExchangeCodeAndResolveIdentity(t *testing.T) {
	t.Parallel()

	const (
		accessToken  = "epic-access-token"
		refreshToken = "epic-refresh-token"
		accountID    = "9626f441055349ce8cb7d7d5a483eaa2"
		displayName  = "EpicGamer"
	)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
			t.Errorf("Authorization = %q, want Basic auth", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := r.Form.Get("code"); got != "auth-code" {
			t.Errorf("code = %q, want auth-code", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"expires_in":    7200,
			"account_id":    accountID,
			"token_type":    "bearer",
		})
	}))
	defer tokenServer.Close()

	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			t.Errorf("Authorization = %q, want bearer access token", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"sub":                accountID,
			"preferred_username": displayName,
		})
	}))
	defer userInfoServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, userInfoServer.URL)

	tokens, err := client.ExchangeCode(context.Background(), "auth-code", "http://localhost:8080/auth/epic/callback")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
	if tokens.AccessToken != accessToken {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, accessToken)
	}
	if tokens.RefreshToken != refreshToken {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, refreshToken)
	}
	if tokens.AccountID != accountID {
		t.Errorf("AccountID = %q, want %q", tokens.AccountID, accountID)
	}

	identity, err := client.ResolveIdentity(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if identity.AccountID != accountID {
		t.Errorf("AccountID = %q, want %q", identity.AccountID, accountID)
	}
	if identity.DisplayName != displayName {
		t.Errorf("DisplayName = %q, want %q", identity.DisplayName, displayName)
	}
}

func TestResolveIdentityFallbackDisplayName(t *testing.T) {
	t.Parallel()

	const accountID = "9626f441055349ce8cb7d7d5a483eaa2"

	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"sub": accountID})
	}))
	defer userInfoServer.Close()

	client := NewClientWithHTTP("id", "secret", userInfoServer.Client())
	client.SetEndpoints("", userInfoServer.URL)

	identity, err := client.ResolveIdentity(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if identity.DisplayName != accountID {
		t.Errorf("DisplayName = %q, want fallback to account ID %q", identity.DisplayName, accountID)
	}
}

func TestExchangeCodeFailure(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "Authorization code expired",
		})
	}))
	defer tokenServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, "")

	_, err := client.ExchangeCode(context.Background(), "expired-code", "http://localhost:8080/auth/epic/callback")
	if err == nil {
		t.Fatal("ExchangeCode() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "Authorization code expired") {
		t.Errorf("ExchangeCode() error = %v, want expiry details", err)
	}
}

func TestConfigured(t *testing.T) {
	t.Parallel()

	if NewClient("", "").Configured() {
		t.Error("empty credentials should not be configured")
	}
	if !NewClient("id", "secret").Configured() {
		t.Error("expected configured client")
	}
}
