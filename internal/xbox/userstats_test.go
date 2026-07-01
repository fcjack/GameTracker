package xbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseMinutesPlayedBatchResponse(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"groups": [
			{
				"titleid": "1144039928",
				"statlistscollection": [
					{
						"stats": [
							{"name": "MinutesPlayed", "value": "150"}
						]
					}
				]
			},
			{
				"titleid": "999888777",
				"statlistscollection": [
					{
						"stats": [
							{"statname": "MinutesPlayed", "value": "45"}
						]
					}
				]
			}
		]
	}`)

	got, err := parseMinutesPlayedBatchBody(body)
	if err != nil {
		t.Fatalf("parseMinutesPlayedBatchResponse() error = %v", err)
	}
	if got[1144039928] != 150 {
		t.Errorf("1144039928 minutes = %d, want 150", got[1144039928])
	}
	if got[999888777] != 45 {
		t.Errorf("999888777 minutes = %d, want 45", got[999888777])
	}
}

func TestParseMinutesPlayedBatchResponseCamelCaseTitleID(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"groups": [
			{
				"titleId": "1144039928",
				"statlistscollection": [
					{
						"stats": [
							{"name": "MinutesPlayed", "value": 8520}
						]
					}
				]
			}
		]
	}`)

	got, err := parseMinutesPlayedBatchBody(body)
	if err != nil {
		t.Fatalf("parseMinutesPlayedBatchBody() error = %v", err)
	}
	if got[1144039928] != 8520 {
		t.Errorf("minutes = %v, want 8520", got[1144039928])
	}
}

func TestParseMinutesPlayedBatchResponseTopLevelCollection(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"statlistscollection": [
			{
				"arrangebyfield": "xuid",
				"arrangebyfieldid": "2535465432123456",
				"stats": [
					{"statname": "MinutesPlayed", "value": "240"}
				]
			}
		]
	}`)

	var parsed userStatsBatchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	minutes, ok := minutesFromBatch(parsed, 0)
	if !ok || minutes != 240 {
		t.Fatalf("minutesFromBatch() = (%d, %v), want (240, true)", minutes, ok)
	}
}

func TestGetMinutesPlayedForTitles(t *testing.T) {
	t.Parallel()

	const xuid = "2535465432123456"

	userStatsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/batch" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s, want POST /batch", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-xbl-contract-version"); got != "2" {
			t.Errorf("contract version = %q, want 2", got)
		}

		body, _ := io.ReadAll(r.Body)
		var req userStatsBatchRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode batch request: %v", err)
		}
		if len(req.XUIDs) != 1 || req.XUIDs[0] != xuid {
			t.Errorf("xuids = %v, want [%s]", req.XUIDs, xuid)
		}
		if len(req.Stats) != 1 {
			t.Errorf("stats count = %d, want 1 per title", len(req.Stats))
		}

		groups := make([]map[string]any, 0, len(req.Stats))
		for _, stat := range req.Stats {
			var minutes int
			switch stat.TitleID {
			case "1144039928":
				minutes = 150
			case "999888777":
				minutes = 45
			default:
				continue
			}
			groups = append(groups, map[string]any{
				"titleid": stat.TitleID,
				"statlistscollection": []map[string]any{
					{
						"stats": []map[string]any{
							{"name": "MinutesPlayed", "value": minutes},
						},
					},
				},
			})
		}

		json.NewEncoder(w).Encode(map[string]any{"groups": groups})
	}))
	defer userStatsServer.Close()

	client := NewClientWithHTTP("id", "secret", userStatsServer.Client())
	client.SetUserStatsURL(userStatsServer.URL)

	session := &XSTSSession{XUID: xuid, UserHash: "hash", Token: "token"}
	got, err := client.GetMinutesPlayedForTitles(context.Background(), session, []int{1144039928, 999888777})
	if err != nil {
		t.Fatalf("GetMinutesPlayedForTitles() error = %v", err)
	}
	if got[1144039928] != 150 {
		t.Errorf("1144039928 minutes = %d, want 150", got[1144039928])
	}
	if got[999888777] != 45 {
		t.Errorf("999888777 minutes = %d, want 45", got[999888777])
	}
}

func TestEnrichGamePlaytime(t *testing.T) {
	t.Parallel()

	const (
		accessToken = "microsoft-access-token"
		xuid        = "2535465432123456"
	)

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
					"xid": xuid,
				}},
			},
		})
	}))
	defer xstsServer.Close()

	titleHubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"titles": []map[string]any{
				{
					"titleId":      "1144039928",
					"name":         "Halo Infinite",
					"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=halo.jpg",
				},
			},
		})
	}))
	defer titleHubServer.Close()

	userStatsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"groups": []map[string]any{
				{
					"titleid": "1144039928",
					"statlistscollection": []map[string]any{
						{
							"stats": []map[string]any{
								{"name": "MinutesPlayed", "value": "8520"},
							},
						},
					},
				},
			},
		})
	}))
	defer userStatsServer.Close()

	client := NewClientWithHTTP("id", "secret", userServer.Client())
	client.SetEndpoints("", userServer.URL, xstsServer.URL)
	client.SetTitleHubURL(titleHubServer.URL)
	client.SetUserStatsURL(userStatsServer.URL)

	snapshot, err := client.GetLibrarySnapshot(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("GetLibrarySnapshot() error = %v", err)
	}
	if len(snapshot.Games) != 1 {
		t.Fatalf("GetLibrarySnapshot() returned %d games, want 1", len(snapshot.Games))
	}

	client.EnrichGamePlaytime(context.Background(), snapshot.Session, &snapshot.Games[0])
	if snapshot.Games[0].PlaytimeMinutes == nil || *snapshot.Games[0].PlaytimeMinutes != 8520 {
		t.Errorf("PlaytimeMinutes = %v, want 8520", snapshot.Games[0].PlaytimeMinutes)
	}
}
