package xbox

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
	got := client.AuthorizeURL("http://localhost:8080/auth/xbox/callback", "state123")

	for _, want := range []string{
		"client_id=client-id",
		"response_type=code",
		"redirect_uri=http",
		"scope=xboxlive.signin",
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
		accessToken  = "microsoft-access-token"
		refreshToken = "microsoft-refresh-token"
		userToken    = "xbox-user-token"
		xuid         = "2535465432123456"
		gamertag     = "TestGamer"
	)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("code"); got != "auth-code" {
			t.Errorf("code = %q, want auth-code", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"expires_in":    3600,
			"token_type":    "bearer",
		})
	}))
	defer tokenServer.Close()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode user auth body: %v", err)
		}
		props, _ := payload["Properties"].(map[string]any)
		if props["RpsTicket"] != "d="+accessToken {
			t.Errorf("RpsTicket = %v, want d=%s", props["RpsTicket"], accessToken)
		}
		json.NewEncoder(w).Encode(map[string]any{"Token": userToken})
	}))
	defer userServer.Close()

	xstsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Token": "xsts-token",
			"DisplayClaims": map[string]any{
				"xui": []map[string]string{{
					"uhs": "user-hash",
					"xid": xuid,
					"gtg": gamertag,
				}},
			},
		})
	}))
	defer xstsServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, userServer.URL, xstsServer.URL)

	tokens, err := client.ExchangeCode(context.Background(), "auth-code", "http://localhost:8080/auth/xbox/callback")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
	if tokens.AccessToken != accessToken {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, accessToken)
	}
	if tokens.RefreshToken != refreshToken {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, refreshToken)
	}

	identity, err := client.ResolveIdentity(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if identity.XUID != xuid {
		t.Errorf("XUID = %q, want %q", identity.XUID, xuid)
	}
	if identity.Gamertag != gamertag {
		t.Errorf("Gamertag = %q, want %q", identity.Gamertag, gamertag)
	}
}

func TestResolveIdentityFallbackGamertag(t *testing.T) {
	t.Parallel()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"Token": "user-token"})
	}))
	defer userServer.Close()

	xstsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Token": "xsts-token",
			"DisplayClaims": map[string]any{
				"xui": []map[string]string{{
					"uhs": "user-hash",
					"xid": "2535465432123456",
				}},
			},
		})
	}))
	defer xstsServer.Close()

	client := NewClientWithHTTP("id", "secret", userServer.Client())
	client.SetEndpoints("", userServer.URL, xstsServer.URL)

	identity, err := client.ResolveIdentity(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if identity.Gamertag != identity.XUID {
		t.Errorf("Gamertag = %q, want fallback to XUID %q", identity.Gamertag, identity.XUID)
	}
}

func TestRefreshToken(t *testing.T) {
	t.Parallel()

	const (
		oldRefresh = "old-refresh-token"
		newAccess  = "new-access-token"
		newRefresh = "new-refresh-token"
	)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("refresh_token"); got != oldRefresh {
			t.Errorf("refresh_token = %q, want %q", got, oldRefresh)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  newAccess,
			"refresh_token": newRefresh,
			"expires_in":    3600,
			"token_type":    "bearer",
		})
	}))
	defer tokenServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, "", "")

	tokens, err := client.RefreshToken(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if tokens.AccessToken != newAccess {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, newAccess)
	}
	if tokens.RefreshToken != newRefresh {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, newRefresh)
	}
}

func TestRefreshTokenKeepsExistingRefreshWhenOmitted(t *testing.T) {
	t.Parallel()

	const oldRefresh = "rolling-refresh-token"

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access-token",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, "", "")

	tokens, err := client.RefreshToken(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if tokens.RefreshToken != oldRefresh {
		t.Errorf("RefreshToken = %q, want preserved %q", tokens.RefreshToken, oldRefresh)
	}
}

func TestRefreshTokenFailure(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "Refresh token expired",
		})
	}))
	defer tokenServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, "", "")

	_, err := client.RefreshToken(context.Background(), "expired-refresh")
	if err == nil {
		t.Fatal("RefreshToken() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "Refresh token expired") {
		t.Errorf("RefreshToken() error = %v, want refresh failure details", err)
	}
}
