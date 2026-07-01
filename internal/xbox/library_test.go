package xbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetOwnedGames(t *testing.T) {
	t.Parallel()

	const (
		accessToken = "microsoft-access-token"
		userToken   = "xbox-user-token"
		xstsToken   = "xsts-token"
		userHash    = "user-hash"
		xuid        = "2535465432123456"
	)

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"Token": userToken})
	}))
	defer userServer.Close()

	xstsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Token": xstsToken,
			"DisplayClaims": map[string]any{
				"xui": []map[string]string{{
					"uhs": userHash,
					"xid": xuid,
					"gtg": "TestGamer",
				}},
			},
		})
	}))
	defer xstsServer.Close()

	titleHubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/users/xuid(" + xuid + ")/titles/titlehistory/decoration/" + titleHistoryDecorations
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.Header.Get("x-xbl-contract-version"); got != "2" {
			t.Errorf("contract version = %q, want 2", got)
		}
		wantAuth := "XBL3.0 x=" + userHash + ";" + xstsToken
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("Authorization = %q, want %q", got, wantAuth)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"titles": []map[string]any{
				{
					"titleId":      "1144039928",
					"name":         "Halo Infinite",
					"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=halo.jpg",
				},
				{
					"titleId": "987654321",
					"detail": map[string]string{
						"name": "Detail Name Game",
					},
					"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=detail.jpg",
				},
				{
					"titleId": "2071061510",
					"name":    "Lies of P",
					"gamePass": map[string]any{
						"isGamePass": true,
					},
					"titleHistory": map[string]string{
						"lastTimePlayed": "2024-05-31T12:02:41.6829304Z",
					},
					"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=lies.jpg",
				},
				{
					"titleId": "555555555",
					"name":    "Unplayed Game Pass Catalog Entry",
					"detail": map[string]any{
						"programs": []string{"GPULTIMATE"},
					},
				},
			},
		})
	}))
	defer titleHubServer.Close()

	userStatsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"groups": []any{}})
	}))
	defer userStatsServer.Close()

	client := NewClientWithHTTP("client-id", "secret", userServer.Client())
	client.SetEndpoints("", userServer.URL, xstsServer.URL)
	client.SetTitleHubURL(titleHubServer.URL)
	client.SetUserStatsURL(userStatsServer.URL)

	games, err := client.GetOwnedGames(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("GetOwnedGames() error = %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("GetOwnedGames() returned %d games, want 3", len(games))
	}
	if games[0].TitleID != 1144039928 || games[0].Name != "Halo Infinite" || games[0].ImageURL == "" {
		t.Errorf("first game = %+v, want Halo Infinite with image", games[0])
	}
	if games[1].TitleID != 987654321 || games[1].Name != "Detail Name Game" {
		t.Errorf("second game = %+v, want detail name fallback", games[1])
	}
	if games[2].TitleID != 2071061510 || games[2].Name != "Lies of P" {
		t.Errorf("third game = %+v, want played Game Pass title", games[2])
	}
}

func TestGetOwnedGamesEmpty(t *testing.T) {
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
					"uhs": "hash",
					"xid": "2535465432123456",
				}},
			},
		})
	}))
	defer xstsServer.Close()

	titleHubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"titles": []any{}})
	}))
	defer titleHubServer.Close()

	userStatsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"groups": []any{}})
	}))
	defer userStatsServer.Close()

	client := NewClientWithHTTP("id", "secret", userServer.Client())
	client.SetEndpoints("", userServer.URL, xstsServer.URL)
	client.SetTitleHubURL(titleHubServer.URL)
	client.SetUserStatsURL(userStatsServer.URL)

	games, err := client.GetOwnedGames(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("GetOwnedGames() error = %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("GetOwnedGames() returned %d games, want 0", len(games))
	}
}

func TestGetOwnedGamesTitleHubError(t *testing.T) {
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
					"uhs": "hash",
					"xid": "2535465432123456",
				}},
			},
		})
	}))
	defer xstsServer.Close()

	titleHubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"message":"Forbidden"}`))
	}))
	defer titleHubServer.Close()

	client := NewClientWithHTTP("id", "secret", userServer.Client())
	client.SetEndpoints("", userServer.URL, xstsServer.URL)
	client.SetTitleHubURL(titleHubServer.URL)

	_, err := client.GetOwnedGames(context.Background(), "access-token")
	if err == nil {
		t.Fatal("GetOwnedGames() error = nil, want title hub failure")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("GetOwnedGames() error = %v, want status in message", err)
	}
}

func TestGetOwnedGamesAuthError(t *testing.T) {
	t.Parallel()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"XErr":2148916233}`))
	}))
	defer userServer.Close()

	client := NewClientWithHTTP("id", "secret", userServer.Client())
	client.SetEndpoints("", userServer.URL, "")

	_, err := client.GetOwnedGames(context.Background(), "bad-access-token")
	if err == nil {
		t.Fatal("GetOwnedGames() error = nil, want auth failure")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("GetOwnedGames() error = %v, want auth failure details", err)
	}
}

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	const (
		accessToken = "microsoft-access-token"
		userToken   = "xbox-user-token"
		xstsToken   = "xsts-token"
		userHash    = "user-hash"
		xuid        = "2535465432123456"
		gamertag    = "TestGamer"
	)

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"Token": userToken})
	}))
	defer userServer.Close()

	xstsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Token": xstsToken,
			"DisplayClaims": map[string]any{
				"xui": []map[string]string{{
					"uhs": userHash,
					"xid": xuid,
					"gtg": gamertag,
				}},
			},
		})
	}))
	defer xstsServer.Close()

	client := NewClientWithHTTP("id", "secret", userServer.Client())
	client.SetEndpoints("", userServer.URL, xstsServer.URL)

	session, err := client.Authenticate(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if session.Token != xstsToken {
		t.Errorf("Token = %q, want %q", session.Token, xstsToken)
	}
	if session.UserHash != userHash {
		t.Errorf("UserHash = %q, want %q", session.UserHash, userHash)
	}
	if session.XUID != xuid {
		t.Errorf("XUID = %q, want %q", session.XUID, xuid)
	}
	if session.Gamertag != gamertag {
		t.Errorf("Gamertag = %q, want %q", session.Gamertag, gamertag)
	}
}
