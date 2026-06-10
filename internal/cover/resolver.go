package cover

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/steam"
	"golang.org/x/sync/singleflight"
)

const maxCoverSize = 5 * 1024 * 1024

type Resolver struct {
	db         *pgxpool.Pool
	igdb       *igdb.Client
	httpClient *http.Client
	sf         singleflight.Group
}

func NewResolver(db *pgxpool.Pool, igdbClient *igdb.Client) *Resolver {
	return NewResolverWithHTTP(db, igdbClient, nil)
}

func NewResolverWithHTTP(db *pgxpool.Pool, igdbClient *igdb.Client, httpClient *http.Client) *Resolver {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Resolver{
		db:         db,
		igdb:       igdbClient,
		httpClient: httpClient,
	}
}

// GetOrFetch returns stored cover bytes or resolves, stores, and returns them.
func (r *Resolver) GetOrFetch(ctx context.Context, gameID int64) ([]byte, string, error) {
	data, mime, err := models.GetGameCoverData(ctx, r.db, gameID)
	if err != nil {
		return nil, "", err
	}
	if len(data) > 0 {
		return data, mime, nil
	}

	result, err, _ := r.sf.Do(strconv.FormatInt(gameID, 10), func() (any, error) {
		data, mime, err := models.GetGameCoverData(ctx, r.db, gameID)
		if err != nil {
			return nil, err
		}
		if len(data) > 0 {
			return coverResult{data: data, mime: mime}, nil
		}
		if err := r.fetchAndStore(ctx, gameID); err != nil {
			return nil, err
		}
		data, mime, err = models.GetGameCoverData(ctx, r.db, gameID)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return coverResult{data: Placeholder(), mime: PlaceholderMIME}, nil
		}
		return coverResult{data: data, mime: mime}, nil
	})
	if err != nil {
		return nil, "", err
	}
	cr := result.(coverResult)
	return cr.data, cr.mime, nil
}

// FetchAndStore resolves cover URLs and persists the image bytes.
func (r *Resolver) FetchAndStore(ctx context.Context, gameID int64) error {
	_, err, _ := r.sf.Do("fetch:"+strconv.FormatInt(gameID, 10), func() (any, error) {
		return nil, r.fetchAndStore(ctx, gameID)
	})
	return err
}

type coverResult struct {
	data []byte
	mime string
}

func (r *Resolver) fetchAndStore(ctx context.Context, gameID int64) error {
	src, err := models.GetGameCoverSources(ctx, r.db, gameID)
	if err != nil {
		return err
	}

	game := &models.Game{
		SteamAppID: src.SteamAppID,
		IGDBId:     src.IGDBId,
		Name:       src.Name,
		CoverURL:   src.CoverURL,
	}

	igdbURL, err := r.resolveIGDBCoverURL(game)
	if err != nil {
		igdbURL = ""
	}

	urls := candidateURLs(game, igdbURL)
	data, mime, sourceURL, err := tryURLs(r.httpClient, urls...)
	if err != nil {
		return models.SaveGameCover(ctx, r.db, gameID, Placeholder(), PlaceholderMIME, "")
	}

	return models.SaveGameCover(ctx, r.db, gameID, data, mime, sourceURL)
}

func candidateURLs(game *models.Game, igdbURL string) []string {
	var urls []string
	if game.SteamAppID != nil {
		urls = append(urls,
			steam.CoverImageURL(*game.SteamAppID, ""),
			steam.HeaderImageURL(*game.SteamAppID),
		)
	}
	if igdbURL != "" {
		urls = append(urls, igdbURL)
	}
	if game.CoverURL != "" && !containsURL(urls, game.CoverURL) {
		urls = append(urls, game.CoverURL)
	}
	return urls
}

func containsURL(urls []string, target string) bool {
	for _, u := range urls {
		if u == target {
			return true
		}
	}
	return false
}

func (r *Resolver) resolveIGDBCoverURL(game *models.Game) (string, error) {
	if game.IGDBId != nil {
		return r.igdbCoverFromID(*game.IGDBId)
	}
	if game.SteamAppID != nil && r.igdb != nil {
		igdbID, err := r.igdb.LookupIGDBIDBySteamAppID(*game.SteamAppID, game.Name)
		if err != nil {
			return "", err
		}
		if igdbID != 0 {
			return r.igdbCoverFromID(igdbID)
		}
	}
	if strings.Contains(game.CoverURL, "igdb.com") {
		return game.CoverURL, nil
	}
	return "", nil
}

func (r *Resolver) igdbCoverFromID(igdbID int64) (string, error) {
	if r.igdb == nil {
		return "", nil
	}
	gameData, err := r.igdb.GetGameByID(igdbID)
	if err != nil {
		return "", err
	}
	if gameData == nil || gameData.Cover == nil {
		return "", nil
	}
	return gameData.Cover.URL, nil
}

func tryURLs(client *http.Client, urls ...string) ([]byte, string, string, error) {
	var lastErr error
	for _, u := range urls {
		if u == "" {
			continue
		}
		data, mime, err := fetchImage(client, u)
		if err == nil {
			return data, mime, u, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no cover URLs to fetch")
	}
	return nil, "", "", lastErr
}

func fetchImage(client *http.Client, url string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxCoverSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxCoverSize {
		return nil, "", fmt.Errorf("cover exceeds max size")
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty cover response")
	}

	mime := resp.Header.Get("Content-Type")
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	if mime == "" || mime == "application/octet-stream" {
		mime = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("unexpected content type %q", mime)
	}
	return data, mime, nil
}
