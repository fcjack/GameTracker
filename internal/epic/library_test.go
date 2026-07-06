package epic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetLibraryParsesRecords(t *testing.T) {
	t.Parallel()

	const accessToken = "epic-access-token"

	libraryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			t.Errorf("Authorization = %q, want bearer access token", got)
		}
		if got := r.URL.Query().Get("includeMetadata"); got != "true" {
			t.Errorf("includeMetadata = %q, want true", got)
		}
		if got := r.URL.Query().Get("platform"); got != "Windows" {
			t.Errorf("platform = %q, want Windows", got)
		}
		if r.URL.Query().Get("cursor") != "" {
			t.Fatalf("unexpected cursor on first page")
		}

		json.NewEncoder(w).Encode(map[string]any{
			"records": []map[string]any{
				{
					"appName":       "Hades",
					"namespace":     "a1234567890abcdef1234567890abcdef",
					"catalogItemId": "11111111-1111-1111-1111-111111111111",
					"sandboxType":   "PUBLICGAME",
					"platform":      []string{"Windows"},
					"metadata": map[string]any{
						"title": "Hades",
						"keyImages": []map[string]string{
							{"type": "DieselGameBoxTall", "url": "https://cdn.example/hades-tall.jpg"},
						},
					},
				},
				{
					"appName":       "Fortnite",
					"namespace":     "fn",
					"catalogItemId": "22222222-2222-2222-2222-222222222222",
					"sandboxType":   "PUBLICGAME",
					"platform":      []string{"Win32"},
				},
				{
					"appName":       "ue-sample",
					"namespace":     "ue",
					"catalogItemId": "33333333-3333-3333-3333-333333333333",
					"sandboxType":   "PUBLICGAME",
				},
				{
					"appName":       "android-only",
					"namespace":     "a1234567890abcdef1234567890abcdef",
					"catalogItemId": "44444444-4444-4444-4444-444444444444",
					"sandboxType":   "PUBLICGAME",
					"platform":      []string{"Android"},
				},
			},
			"responseMetadata": map[string]any{
				"nextCursor": nil,
			},
		})
	}))
	defer libraryServer.Close()

	client := NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	client.SetLibraryURL(libraryServer.URL)

	games, err := client.GetLibrary(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("GetLibrary() error = %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("len(games) = %d, want 2", len(games))
	}
	if games[0].CatalogItemID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("CatalogItemID[0] = %q, want Hades id", games[0].CatalogItemID)
	}
	if games[0].Name != "Hades" {
		t.Errorf("Name[0] = %q, want Hades", games[0].Name)
	}
	if games[0].ImageURL != "https://cdn.example/hades-tall.jpg" {
		t.Errorf("ImageURL[0] = %q, want tall cover", games[0].ImageURL)
	}
	if games[1].AppName != "Fortnite" {
		t.Errorf("AppName[1] = %q, want Fortnite", games[1].AppName)
	}
}

func TestGetLibraryPaginates(t *testing.T) {
	t.Parallel()

	const accessToken = "epic-access-token"
	page := 0

	libraryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		switch r.URL.Query().Get("cursor") {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []map[string]any{
					{
						"appName":       "page-one",
						"namespace":     "ns-one",
						"catalogItemId": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						"sandboxType":   "PUBLICGAME",
					},
				},
				"responseMetadata": map[string]any{"nextCursor": "page-2"},
			})
		case "page-2":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []map[string]any{
					{
						"appName":       "page-two",
						"namespace":     "ns-two",
						"catalogItemId": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
						"sandboxType":   "PUBLICGAME",
					},
				},
				"responseMetadata": map[string]any{"nextCursor": nil},
			})
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	defer libraryServer.Close()

	client := NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	client.SetLibraryURL(libraryServer.URL)

	games, err := client.GetLibrary(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("GetLibrary() error = %v", err)
	}
	if page != 2 {
		t.Errorf("pages fetched = %d, want 2", page)
	}
	if len(games) != 2 {
		t.Fatalf("len(games) = %d, want 2", len(games))
	}
}

func TestGetLibraryEmpty(t *testing.T) {
	t.Parallel()

	libraryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"records":          []any{},
			"responseMetadata": map[string]any{"nextCursor": nil},
		})
	}))
	defer libraryServer.Close()

	client := NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	client.SetLibraryURL(libraryServer.URL)

	games, err := client.GetLibrary(context.Background(), "token")
	if err != nil {
		t.Fatalf("GetLibrary() error = %v", err)
	}
	if games == nil {
		t.Fatal("games = nil, want empty slice")
	}
	if len(games) != 0 {
		t.Errorf("len(games) = %d, want 0", len(games))
	}
}

func TestGetLibraryAuthError(t *testing.T) {
	t.Parallel()

	libraryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorCode":"errors.com.epicgames.common.authentication.authentication_failed"}`))
	}))
	defer libraryServer.Close()

	client := NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	client.SetLibraryURL(libraryServer.URL)

	_, err := client.GetLibrary(context.Background(), "expired-token")
	if err == nil {
		t.Fatal("GetLibrary() error = nil, want auth failure")
	}
	if !strings.Contains(err.Error(), "library access denied") {
		t.Errorf("GetLibrary() error = %v, want access denied message", err)
	}
	if !strings.Contains(err.Error(), "re-link") {
		t.Errorf("GetLibrary() error = %v, want re-link guidance", err)
	}
}

func TestGetLibraryMissingAccessToken(t *testing.T) {
	t.Parallel()

	client := NewClient("client-id", "secret")
	_, err := client.GetLibrary(context.Background(), "")
	if err == nil {
		t.Fatal("GetLibrary() error = nil, want missing token error")
	}
	if !strings.Contains(err.Error(), "access token is required") {
		t.Errorf("GetLibrary() error = %v, want required token message", err)
	}
}

func TestShouldImportRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record libraryRecord
		want   bool
	}{
		{
			name: "valid game",
			record: libraryRecord{
				AppName:       "game",
				Namespace:     "ns",
				CatalogItemID: "id",
				SandboxType:   "PUBLICGAME",
				Platform:      []string{"Windows"},
			},
			want: true,
		},
		{
			name: "missing catalog id",
			record: libraryRecord{
				AppName:   "game",
				Namespace: "ns",
			},
			want: false,
		},
		{
			name: "private sandbox",
			record: libraryRecord{
				AppName:       "game",
				Namespace:     "ns",
				CatalogItemID: "id",
				SandboxType:   "PRIVATE",
			},
			want: false,
		},
		{
			name: "unreal engine namespace",
			record: libraryRecord{
				AppName:       "asset",
				Namespace:     namespaceUnrealEngine,
				CatalogItemID: "id",
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldImportRecord(tc.record); got != tc.want {
				t.Errorf("shouldImportRecord() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPickCoverURL(t *testing.T) {
	t.Parallel()

	images := []libraryKeyImage{
		{Type: "Thumbnail", URL: "https://cdn.example/thumb.jpg"},
		{Type: "DieselGameBoxTall", URL: "https://cdn.example/tall.jpg"},
	}
	if got := pickCoverURL(images); got != "https://cdn.example/tall.jpg" {
		t.Errorf("pickCoverURL() = %q, want tall cover first", got)
	}
}
