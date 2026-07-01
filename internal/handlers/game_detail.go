package handlers

import (
	"context"
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type gameDetailMeta struct {
	Summary       string
	Storyline     string
	Genres        []string
	Themes        []string
	Keywords      []string
	Developers    []string
	Publishers    []string
	Platforms     []string
	ReleaseDate   string
	GameStatus    string
	CriticScore   *int
	UserScore     *int
	CriticVotes   int
	UserVotes     int
	BackdropURL   string
	ExternalLinks map[string]string
	HasMetadata   bool
}

func roundScore(v float64) *int {
	if v <= 0 {
		return nil
	}
	s := int(math.Round(v))
	return &s
}

func gameStatusKey(status int) string {
	switch status {
	case 2:
		return "game.status_alpha"
	case 3:
		return "game.status_beta"
	case 4:
		return "game.status_early_access"
	case 5:
		return "game.status_offline"
	case 6:
		return "game.status_cancelled"
	case 7:
		return "game.status_rumored"
	case 8:
		return "game.status_delisted"
	default:
		return "game.status_released"
	}
}

func formatReleaseDate(locale string, unix int64) string {
	if unix <= 0 {
		return ""
	}
	t := time.Unix(unix, 0).UTC()
	switch locale {
	case "pt-BR", "pt_BR", "pt-br":
		return t.Format("02/01/2006")
	default:
		return t.Format("January 2, 2006")
	}
}

func detailMetaFromStored(locale string, meta *models.GameIGDBMetadata, T func(string, ...any) string) *gameDetailMeta {
	if meta == nil {
		return nil
	}
	dm := &gameDetailMeta{
		Summary:       meta.Summary,
		Storyline:     meta.Storyline,
		Genres:        meta.Genres,
		Themes:        meta.Themes,
		Keywords:      meta.Keywords,
		Developers:    meta.Developers,
		Publishers:    meta.Publishers,
		Platforms:     meta.Platforms,
		ReleaseDate:   formatReleaseDate(locale, meta.ReleaseDate),
		GameStatus:    T(gameStatusKey(meta.GameStatus)),
		CriticScore:   roundScore(ptrFloat(meta.AggregatedRating)),
		UserScore:     roundScore(ptrFloat(meta.TotalRating)),
		CriticVotes:   meta.RatingCount,
		UserVotes:     meta.TotalRatingCount,
		BackdropURL:   meta.BackdropURL,
		ExternalLinks: meta.ExternalLinks,
		HasMetadata:   meta.Summary != "" || len(meta.Genres) > 0 || meta.AggregatedRating != nil || meta.TotalRating != nil,
	}
	if dm.ExternalLinks == nil {
		dm.ExternalLinks = map[string]string{}
	}
	return dm
}

func ptrFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func (h *LibraryHandler) loadGameIGDBMetadata(ctx context.Context, gameID int64, igdbID *int64) (*models.GameIGDBMetadata, error) {
	meta, err := models.GetGameIGDBMetadata(ctx, h.db, gameID)
	if err != nil {
		return nil, err
	}
	if meta != nil || igdbID == nil || h.igdb == nil {
		return meta, nil
	}

	details, err := h.igdb.GetGameDetails(*igdbID)
	if err != nil || details == nil {
		return meta, err
	}
	meta = models.GameIGDBMetadataFromDetails(details)
	if saveErr := models.SaveGameIGDBMetadata(ctx, h.db, gameID, meta); saveErr != nil {
		return meta, saveErr
	}
	return meta, nil
}

func (h *LibraryHandler) enrichDetailCard(c *gin.Context, card libraryGameCard) libraryGameCard {
	if card.IGDBId == nil {
		return card
	}
	meta, err := h.loadGameIGDBMetadata(c.Request.Context(), card.GameID, card.IGDBId)
	if err != nil || meta == nil {
		return card
	}
	card.Meta = detailMetaFromStored(LocaleFromContext(c), meta, card.T)
	return card
}

func (h *LibraryHandler) storeIGDBMetadata(ctx context.Context, gameID int64, igdbID int64) {
	if h.igdb == nil {
		return
	}
	details, err := h.igdb.GetGameDetails(igdbID)
	if err != nil || details == nil {
		return
	}
	_ = models.SaveGameIGDBMetadata(ctx, h.db, gameID, models.GameIGDBMetadataFromDetails(details))
}
