package playtime

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/config"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

const (
	steamPlatform = "Steam"
	xboxPlatform  = "Xbox"
)

// Publisher enqueues playtime work for background processing.
type Publisher interface {
	Publish(Event)
}

// Handler processes playtime events with retries and rate limiting.
type Handler struct {
	db        *pgxpool.Pool
	xbox      *xbox.Client
	encrypter *crypto.Encrypter

	sessions sessionCache

	executor failsafe.Executor[struct{}]
}

type sessionCache struct {
	mu      sync.Mutex
	entries map[int64]cachedSession
}

type cachedSession struct {
	session *xbox.XSTSSession
	expires time.Time
}

func NewHandler(db *pgxpool.Pool, xboxClient *xbox.Client, encrypter *crypto.Encrypter) *Handler {
	retryMax := config.PlaytimeRetryMax()
	builder := retrypolicy.NewBuilder[struct{}]().
		HandleIf(isRetryableError).
		WithBackoff(time.Second, config.PlaytimeRetryBackoffMax()).
		AbortIf(func(_ struct{}, err error) bool {
			return errors.Is(err, xbox.ErrPlaytimeNotFound)
		})
	if retryMax > 0 {
		builder = builder.WithMaxRetries(retryMax)
	}

	rate := uint(config.PlaytimeRatePerSecond())
	limiter := ratelimiter.NewSmoothBuilder[struct{}](rate, time.Second).
		WithMaxWaitTime(10 * time.Second).
		Build()

	return &Handler{
		db:        db,
		xbox:      xboxClient,
		encrypter: encrypter,
		sessions:  sessionCache{entries: make(map[int64]cachedSession)},
		executor:  failsafe.With(limiter, builder.Build()),
	}
}

func isRetryableError(_ struct{}, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, xbox.ErrPlaytimeNotFound) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, " 500") ||
		strings.Contains(msg, " 502") ||
		strings.Contains(msg, " 503") ||
		strings.Contains(msg, " 504")
}

// Process executes a single playtime event.
func (h *Handler) Process(ctx context.Context, event Event) error {
	_, err := h.executor.WithContext(ctx).Get(func() (struct{}, error) {
		return struct{}{}, h.processOnce(ctx, event)
	})
	return err
}

func (h *Handler) processOnce(ctx context.Context, event Event) error {
	switch event.Kind {
	case KindXbox:
		return h.processXbox(ctx, event)
	case KindSteam:
		return h.processSteam(ctx, event)
	default:
		return nil
	}
}

func (h *Handler) processSteam(ctx context.Context, event Event) error {
	if event.AppID <= 0 || event.Minutes < 0 {
		return nil
	}
	return models.UpdatePlaytimeBySteamAppID(ctx, h.db, event.UserID, steamPlatform, event.AppID, event.Minutes)
}

func (h *Handler) processXbox(ctx context.Context, event Event) error {
	if h.xbox == nil || h.encrypter == nil || event.TitleID <= 0 {
		return nil
	}

	session, err := h.sessionForUser(ctx, event.UserID)
	if err != nil {
		return err
	}

	minutes, err := h.xbox.FetchMinutesPlayed(ctx, session, event.TitleID, event.SCID)
	if errors.Is(err, xbox.ErrPlaytimeNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	return persistXboxPlaytime(ctx, h.db, event.UserID, event.TitleID, event.Name, minutes)
}

func (h *Handler) sessionForUser(ctx context.Context, userID int64) (*xbox.XSTSSession, error) {
	h.sessions.mu.Lock()
	if cached, ok := h.sessions.entries[userID]; ok && time.Now().Before(cached.expires) {
		session := cached.session
		h.sessions.mu.Unlock()
		return session, nil
	}
	h.sessions.mu.Unlock()

	tokens, err := xbox.EnsureFreshTokens(ctx, h.xbox, h.encrypter, h.db, userID)
	if err != nil {
		return nil, err
	}
	session, err := h.xbox.Authenticate(ctx, tokens.AccessToken)
	if err != nil {
		return nil, err
	}

	h.sessions.mu.Lock()
	h.sessions.entries[userID] = cachedSession{
		session: session,
		expires: time.Now().Add(10 * time.Minute),
	}
	h.sessions.mu.Unlock()
	return session, nil
}

func persistXboxPlaytime(ctx context.Context, db *pgxpool.Pool, userID int64, titleID int, name string, minutes int) error {
	rows, err := models.UpdatePlaytimeByXboxTitleID(ctx, db, userID, xboxPlatform, titleID, minutes)
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}

	linked, err := models.SetGameXboxTitleIDForUserXboxGame(ctx, db, userID, xboxPlatform, titleID, name)
	if err != nil {
		return err
	}
	if linked == 0 {
		slog.Warn("playtime update matched no library entry",
			"user_id", userID,
			"title_id", titleID,
			"name", name,
		)
		return nil
	}

	_, err = models.UpdatePlaytimeByXboxTitleID(ctx, db, userID, xboxPlatform, titleID, minutes)
	return err
}

// SyncPublisher runs playtime events inline. Useful in tests.
type SyncPublisher struct {
	handler *Handler
}

func NewSyncPublisher(handler *Handler) *SyncPublisher {
	return &SyncPublisher{handler: handler}
}

func (p *SyncPublisher) Publish(event Event) {
	if p == nil || p.handler == nil {
		return
	}
	if err := p.handler.Process(context.Background(), event); err != nil {
		slog.Warn("sync playtime event failed",
			"kind", event.Kind,
			"user_id", event.UserID,
			"error", err,
		)
	}
}
