package epic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	libraryItemsPath      = "/library/api/public/items"
	namespaceUnrealEngine = "ue"
	namespaceUnrealTools  = "89efe5924d3d467c839449ab6ab52e7f"
)

// OwnedGame is a title from the user's Epic Games Store library.
type OwnedGame struct {
	CatalogItemID string
	Namespace     string
	AppName       string
	Name          string
	ImageURL      string
}

type libraryPageResponse struct {
	Records          []libraryRecord `json:"records"`
	ResponseMetadata struct {
		NextCursor *string `json:"nextCursor"`
	} `json:"responseMetadata"`
}

type libraryRecord struct {
	AppName       string   `json:"appName"`
	Namespace     string   `json:"namespace"`
	CatalogItemID string   `json:"catalogItemId"`
	SandboxType   string   `json:"sandboxType"`
	Platform      []string `json:"platform"`
	Metadata      *struct {
		Title     string            `json:"title"`
		KeyImages []libraryKeyImage `json:"keyImages"`
	} `json:"metadata"`
}

type libraryKeyImage struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// GetLibrary fetches the user's EGS library using the community-documented library
// service endpoint (same approach as Playnite and Legendary). Requires the user's
// OAuth access token from account linking (basic_profile scope). Pagination is handled
// internally; callers receive the full merged list.
//
// Endpoint: GET {libraryURL}/library/api/public/items?includeMetadata=true&platform=Windows
func (c *Client) GetLibrary(ctx context.Context, accessToken string) ([]OwnedGame, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("epic: access token is required")
	}

	records, err := c.listLibraryRecords(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	return parseLibraryRecords(records), nil
}

func (c *Client) listLibraryRecords(ctx context.Context, accessToken string) ([]libraryRecord, error) {
	var (
		allRecords []libraryRecord
		cursor     string
		prevCursor string
	)

	for {
		page, err := c.fetchLibraryPage(ctx, accessToken, cursor)
		if err != nil {
			return nil, err
		}

		allRecords = append(allRecords, page.Records...)

		next := ""
		if page.ResponseMetadata.NextCursor != nil {
			next = strings.TrimSpace(*page.ResponseMetadata.NextCursor)
		}
		if next == "" || next == prevCursor {
			break
		}
		prevCursor = cursor
		cursor = next
	}

	return allRecords, nil
}

func (c *Client) fetchLibraryPage(ctx context.Context, accessToken, cursor string) (*libraryPageResponse, error) {
	params := url.Values{}
	params.Set("includeMetadata", "true")
	params.Set("platform", "Windows")
	if cursor != "" {
		params.Set("cursor", cursor)
	}

	endpoint := strings.TrimRight(c.libraryURL, "/") + libraryItemsPath + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("epic: create library request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("epic: library request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("epic: read library response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("epic: library access denied (%d): re-link your Epic account", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("epic: library request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var page libraryPageResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("epic: decode library response: %w", err)
	}
	if page.Records == nil {
		page.Records = []libraryRecord{}
	}
	return &page, nil
}

func parseLibraryRecords(records []libraryRecord) []OwnedGame {
	if len(records) == 0 {
		return []OwnedGame{}
	}

	games := make([]OwnedGame, 0, len(records))
	for _, record := range records {
		if !shouldImportRecord(record) {
			continue
		}

		name := record.CatalogItemID
		var imageURL string
		if record.Metadata != nil {
			if record.Metadata.Title != "" {
				name = record.Metadata.Title
			}
			imageURL = pickCoverURL(record.Metadata.KeyImages)
		}

		namespace := record.Namespace
		if namespace == "" {
			namespace = "egs"
		}

		games = append(games, OwnedGame{
			CatalogItemID: record.CatalogItemID,
			Namespace:     namespace,
			AppName:       record.AppName,
			Name:          name,
			ImageURL:      imageURL,
		})
	}
	return games
}

func shouldImportRecord(record libraryRecord) bool {
	if record.CatalogItemID == "" || record.AppName == "" {
		return false
	}
	if record.AppName == "1" {
		return false
	}
	switch record.Namespace {
	case namespaceUnrealEngine, namespaceUnrealTools:
		return false
	}
	if strings.EqualFold(record.SandboxType, "PRIVATE") {
		return false
	}
	return supportsWindowsPlatform(record.Platform)
}

func supportsWindowsPlatform(platforms []string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, platform := range platforms {
		switch strings.ToLower(strings.TrimSpace(platform)) {
		case "windows", "win32":
			return true
		}
	}
	return false
}

func pickCoverURL(images []libraryKeyImage) string {
	preferred := []string{
		"DieselGameBoxTall",
		"DieselGameBox",
		"Thumbnail",
		"DieselStoreFrontWide",
	}
	for _, want := range preferred {
		for _, image := range images {
			if image.Type == want && image.URL != "" {
				return image.URL
			}
		}
	}
	return ""
}
