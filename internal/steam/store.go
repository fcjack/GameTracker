package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// errAppUnavailable is returned when the Steam Store has no details for an app (e.g. removed/delisted).
var errAppUnavailable = errors.New("steam store: app unavailable")

const (
	storeAPIBase         = "https://store.steampowered.com"
	defaultFilterWorkers = 2
	defaultRateBurst     = 2
	// Steam store appdetails is limited to ~200 requests per 5 minutes per IP.
	defaultMinInterval = 1500 * time.Millisecond
	maxStoreRetries    = 5
	storeRetryBase     = 5 * time.Second
)

// StoreClient resolves Steam store metadata for owned apps.
type StoreClient struct {
	baseURL     string
	httpClient  *http.Client
	minInterval time.Duration
	rateBurst   int

	rateOnce sync.Once
	rateCh   chan struct{}

	typeCache sync.Map // int -> cachedAppType
}

type cachedAppType struct {
	typ string
	ok  bool
}

func NewStoreClient() *StoreClient {
	return NewStoreClientWithHTTP(storeAPIBase, nil)
}

func NewStoreClientWithHTTP(baseURL string, httpClient *http.Client) *StoreClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &StoreClient{
		baseURL:     baseURL,
		httpClient:  httpClient,
		minInterval: defaultMinInterval,
		rateBurst:   defaultRateBurst,
	}
}

func (c *StoreClient) SetMinInterval(d time.Duration) {
	c.minInterval = d
}

func (c *StoreClient) initRateGate() {
	c.rateOnce.Do(func() {
		if c.minInterval <= 0 {
			return
		}
		burst := c.rateBurst
		if burst < 1 {
			burst = 1
		}
		c.rateCh = make(chan struct{}, burst)
		for i := 0; i < burst; i++ {
			c.rateCh <- struct{}{}
		}
		go func() {
			ticker := time.NewTicker(c.minInterval)
			defer ticker.Stop()
			for range ticker.C {
				select {
				case c.rateCh <- struct{}{}:
				default:
				}
			}
		}()
	})
}

func (c *StoreClient) acquireRate(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}
	c.initRateGate()
	select {
	case <-c.rateCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsImportableAppType reports whether a Steam store app should be tracked as a game.
func IsImportableAppType(typ string) bool {
	return typ == "game"
}

// GetAppType returns the Steam store type for an app ID.
// The second value is false when the store has no details for the app.
func (c *StoreClient) GetAppType(ctx context.Context, appID int) (string, bool, error) {
	return c.getAppType(ctx, appID)
}

func (c *StoreClient) getAppType(ctx context.Context, appID int) (string, bool, error) {
	if cached, hit := c.typeCache.Load(appID); hit {
		entry := cached.(cachedAppType)
		return entry.typ, entry.ok, nil
	}

	body, err := c.fetchAppDetails(ctx, appID)
	if err != nil {
		if errors.Is(err, errAppUnavailable) {
			c.typeCache.Store(appID, cachedAppType{})
			return "", false, nil
		}
		return "", false, err
	}

	var result map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", false, fmt.Errorf("steam store: app %d decode response: %w", appID, err)
	}

	entry, ok := result[strconv.Itoa(appID)]
	if !ok || !entry.Success {
		c.typeCache.Store(appID, cachedAppType{})
		return "", false, nil
	}

	c.typeCache.Store(appID, cachedAppType{typ: entry.Data.Type, ok: true})
	return entry.Data.Type, true, nil
}

func (c *StoreClient) fetchAppDetails(ctx context.Context, appID int) ([]byte, error) {
	url := fmt.Sprintf("%s/api/appdetails?appids=%d&filters=basic", c.baseURL, appID)

	var lastErr error
	for attempt := 0; attempt <= maxStoreRetries; attempt++ {
		if err := c.acquireRate(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "HeliosGameTracker/1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("steam store: app %d request: %w", appID, err)
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("steam store: app %d read response: %w", appID, readErr)
		}

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}

		if resp.StatusCode == http.StatusInternalServerError {
			// Steam returns 500 for removed/delisted apps; treat as unavailable, not a fatal error.
			return nil, fmt.Errorf("%w: app %d returned 500", errAppUnavailable, appID)
		}
		lastErr = fmt.Errorf("steam store: app %d returned %d: %s", appID, resp.StatusCode, string(body))
		if !isRetryableStoreStatus(resp.StatusCode) || attempt == maxStoreRetries {
			return nil, lastErr
		}

		wait := storeRetryWait(resp, attempt)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

func isRetryableStoreStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusBadGateway
}

func storeRetryWait(resp *http.Response, attempt int) time.Duration {
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return storeRetryBase * time.Duration(1<<attempt)
}

// FilterImportableGames keeps only Steam store entries classified as games.
// Lookups run in parallel with a bounded rate limiter so multiple requests can be in flight.
func (c *StoreClient) FilterImportableGames(ctx context.Context, games []OwnedGame) ([]OwnedGame, error) {
	if len(games) == 0 {
		return games, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan OwnedGame)
	type outcome struct {
		game OwnedGame
		keep bool
	}
	results := make(chan outcome, len(games))
	errCh := make(chan error, 1)

	var wg sync.WaitGroup
	workers := defaultFilterWorkers
	if len(games) < workers {
		workers = len(games)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for g := range jobs {
				if ctx.Err() != nil {
					return
				}
				typ, ok, err := c.getAppType(ctx, g.AppID)
				if err != nil {
					select {
					case errCh <- err:
						cancel()
					default:
					}
					return
				}
				results <- outcome{game: g, keep: ok && IsImportableAppType(typ)}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, g := range games {
			select {
			case jobs <- g:
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	close(results)

	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	default:
	}

	filtered := make([]OwnedGame, 0, len(games))
	for r := range results {
		if r.keep {
			filtered = append(filtered, r.game)
		}
	}
	return filtered, nil
}
