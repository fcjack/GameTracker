package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/cover"
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type LibraryHandler struct {
	db     *pgxpool.Pool
	igdb   *igdb.Client
	covers *cover.Resolver
}

type libraryGameCard struct {
	*models.UserGameWithGame
	ShowPlatform        bool
	NeedsCompletionYear bool
	PlaytimeLabel       string
	CurrentYear         int
	Lang                string
	T                   func(string, ...any) string
}

type libraryGameGroup struct {
	Label string
	Games []libraryGameCard
}

func toLibraryCardWithLocale(locale string, g *models.UserGameWithGame, showPlatform bool) libraryGameCard {
	currentYear := time.Now().Year()
	card := libraryGameCard{
		UserGameWithGame:    g,
		ShowPlatform:        showPlatform,
		CurrentYear:         currentYear,
		NeedsCompletionYear: g.ReleaseYear > 0 && g.ReleaseYear < currentYear,
		Lang:                locale,
		T:                   i18n.NewTranslator(locale),
	}
	if g.PlaytimeMinutes != nil {
		card.PlaytimeLabel = i18n.FormatPlaytime(locale, *g.PlaytimeMinutes)
	}
	return card
}

func toLibraryCard(c *gin.Context, g *models.UserGameWithGame, showPlatform bool) libraryGameCard {
	return toLibraryCardWithLocale(LocaleFromContext(c), g, showPlatform)
}

func toLibraryCardsWithLocale(locale string, games []*models.UserGameWithGame, showPlatform bool) []libraryGameCard {
	cards := make([]libraryGameCard, len(games))
	for i, g := range games {
		cards[i] = toLibraryCardWithLocale(locale, g, showPlatform)
	}
	return cards
}

func toLibraryCards(c *gin.Context, games []*models.UserGameWithGame, showPlatform bool) []libraryGameCard {
	return toLibraryCardsWithLocale(LocaleFromContext(c), games, showPlatform)
}

func toLibraryPlatformGroups(c *gin.Context, games []*models.UserGameWithGame) []libraryGameGroup {
	grouped := models.GroupUserGamesByPlatform(games)
	groups := make([]libraryGameGroup, len(grouped))
	for i, g := range grouped {
		groups[i] = libraryGameGroup{
			Label: g.Platform,
			Games: toLibraryCardsWithLocale(LocaleFromContext(c), g.Games, false),
		}
	}
	return groups
}

func toLibraryYearGroups(c *gin.Context, games []*models.UserGameWithGame) []libraryGameGroup {
	grouped := models.GroupUserGamesByCompletionYear(games)
	groups := make([]libraryGameGroup, len(grouped))
	for i, g := range grouped {
		groups[i] = libraryGameGroup{
			Label: i18n.GroupYearLabel(LocaleFromContext(c), g.Label),
			Games: toLibraryCardsWithLocale(LocaleFromContext(c), g.Games, true),
		}
	}
	return groups
}

func completionYearOptions(releaseYear, currentYear int) []int {
	start := releaseYear
	if start <= 0 {
		start = currentYear
	}
	if start > currentYear {
		start = currentYear
	}
	years := make([]int, 0, currentYear-start+1)
	for y := currentYear; y >= start; y-- {
		years = append(years, y)
	}
	return years
}

func NewLibraryHandler(db *pgxpool.Pool, igdbClient *igdb.Client) *LibraryHandler {
	return &LibraryHandler{
		db:     db,
		igdb:   igdbClient,
		covers: cover.NewResolver(db, igdbClient),
	}
}

func (h *LibraryHandler) LibraryPage(c *gin.Context) {
	session := sessions.Default(c)
	username := session.Get("username").(string)

	// The grid loads its own data via HTMX (GET /library/games), so the
	// page shell renders without touching the database.
	c.HTML(http.StatusOK, "library/index", ViewData(c, gin.H{
		"username":  username,
		"activeNav": "library",
	}))
}

const libraryPageSize = 30

func nextLibraryPageURL(page int, filter string) string {
	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	if filter != "" {
		q.Set("filter", filter)
	}
	return "/library/games?" + q.Encode()
}

func (h *LibraryHandler) LibraryGrid(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	groupBy := c.Query("group_by")
	filter := c.Query("filter")
	statuses := []string{"playing", "completed"}

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}

	var games []*models.UserGameWithGame
	var err error
	hasMore := false

	if groupBy == "platform" || groupBy == "year" {
		// Grouped views need the full library; pagination applies to the flat grid only.
		if filter == "active" {
			games, err = models.ListUserGamesByStatuses(c.Request.Context(), h.db, userID, statuses)
		} else {
			games, err = models.ListUserGames(c.Request.Context(), h.db, userID)
		}
	} else {
		// Fetch one extra row to know whether another page exists.
		offset := (page - 1) * libraryPageSize
		if filter == "active" {
			games, err = models.ListUserGamesByStatusesPage(c.Request.Context(), h.db, userID, statuses, libraryPageSize+1, offset)
		} else {
			games, err = models.ListUserGamesPage(c.Request.Context(), h.db, userID, libraryPageSize+1, offset)
		}
		if len(games) > libraryPageSize {
			games = games[:libraryPageSize]
			hasMore = true
		}
	}
	if err != nil {
		games = nil
		hasMore = false
	}

	data := ViewData(c, gin.H{
		"hasGames": len(games) > 0,
	})
	if filter == "active" {
		data["emptyTitle"] = "library.no_active_games"
		data["emptyHint"] = "library.no_active_hint"
	}
	if hasMore {
		data["nextPageURL"] = nextLibraryPageURL(page+1, filter)
	}
	switch groupBy {
	case "platform":
		data["groups"] = toLibraryPlatformGroups(c, games)
	case "year":
		data["groups"] = toLibraryYearGroups(c, games)
	default:
		data["games"] = toLibraryCards(c, games, true)
	}

	if page > 1 {
		c.HTML(http.StatusOK, "library/game_grid_page", data)
		return
	}
	c.HTML(http.StatusOK, "library/game_grid", data)
}

func (h *LibraryHandler) SearchLibrary(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.HTML(http.StatusOK, "library/library_search_results", ViewData(c, gin.H{}))
		return
	}

	games, err := models.SearchUserGames(c.Request.Context(), h.db, userID, query, 20)
	if err != nil {
		c.HTML(http.StatusOK, "library/library_search_results", ViewData(c, gin.H{
			"error": "error.search_failed",
		}))
		return
	}

	c.HTML(http.StatusOK, "library/library_search_results", ViewData(c, gin.H{
		"searched": true,
		"games":    toLibraryCards(c, games, true),
	}))
}

func (h *LibraryHandler) SearchIGDB(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.HTML(http.StatusOK, "library/search_results", ViewData(c, gin.H{"results": nil}))
		return
	}

	results, err := h.igdb.Search(query, 10)
	if err != nil {
		c.HTML(http.StatusOK, "library/search_results", ViewData(c, gin.H{
			"error": "error.search_failed",
		}))
		return
	}

	type resultWithStatus struct {
		igdb.SearchResult
		InLibrary   bool
		ReleaseYear int
		CoverURL    string
	}

	igdbIDs := make([]int64, len(results))
	for i, r := range results {
		igdbIDs[i] = r.ID
	}
	inLibrary, err := models.LibraryIGDBIDs(c.Request.Context(), h.db, userID, igdbIDs)
	if err != nil {
		inLibrary = nil
	}

	enriched := make([]resultWithStatus, 0, len(results))
	for _, r := range results {
		rws := resultWithStatus{
			SearchResult: r,
			ReleaseYear:  igdb.ReleaseYear(r.FirstReleaseDate),
		}
		if r.Cover != nil {
			rws.CoverURL = r.Cover.URL
		}
		_, rws.InLibrary = inLibrary[r.ID]

		enriched = append(enriched, rws)
	}

	c.HTML(http.StatusOK, "library/search_results", ViewData(c, gin.H{
		"results": enriched,
		"userID":  userID,
	}))
}

func (h *LibraryHandler) AddGame(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	igdbIDStr := c.PostForm("igdb_id")
	igdbID, err := strconv.ParseInt(igdbIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid igdb_id"})
		return
	}

	name := c.PostForm("name")
	coverURL := c.PostForm("cover_url")

	releaseYearStr := c.PostForm("release_year")
	releaseYear, _ := strconv.Atoi(releaseYearStr)

	allPlatforms := c.PostFormArray("platforms")
	if allPlatforms == nil {
		allPlatforms = []string{}
	}

	platform := strings.TrimSpace(c.PostForm("platform"))
	switch len(allPlatforms) {
	case 0:
		// no platform metadata from IGDB
	case 1:
		platform = allPlatforms[0]
	default:
		if platform == "" {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		valid := false
		for _, p := range allPlatforms {
			if p == platform {
				valid = true
				break
			}
		}
		if !valid {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
	}

	categoryIGDBValueStr := c.PostForm("category_igdb_value")
	categoryIGDBValue, _ := strconv.Atoi(categoryIGDBValueStr)

	cat, err := models.GetCategoryByIGDBValue(c.Request.Context(), h.db, categoryIGDBValue)
	if err != nil {
		// Fallback to "Main Game" (igdb_value=0)
		cat, err = models.GetCategoryByIGDBValue(c.Request.Context(), h.db, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "category lookup failed"})
			return
		}
	}

	game, err := models.FindOrCreateGame(
		c.Request.Context(), h.db,
		igdbID, name, coverURL, releaseYear, allPlatforms, cat.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save game"})
		return
	}

	if err := models.AddToLibrary(c.Request.Context(), h.db, userID, game.ID, platform, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add to library"})
		return
	}

	_ = h.covers.FetchAndStore(c.Request.Context(), game.ID)

	c.Header("HX-Trigger-After-Swap", "libraryUpdated")
	c.HTML(http.StatusOK, "library/in_library_button", ViewData(c, gin.H{
		"gameID": game.ID,
	}))
}

func (h *LibraryHandler) RemoveGame(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	gameIDStr := c.Param("game_id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	if err := models.RemoveFromLibrary(c.Request.Context(), h.db, userID, gameID); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Header("HX-Trigger", "libraryUpdated")
	c.Status(http.StatusOK)
}

func (h *LibraryHandler) CompleteGameForm(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	gameID, err := strconv.ParseInt(c.Param("game_id"), 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	game, err := models.GetUserGame(c.Request.Context(), h.db, userID, gameID)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if game.Status == "completed" {
		c.Status(http.StatusOK)
		return
	}

	currentYear := time.Now().Year()
	c.HTML(http.StatusOK, "library/complete_form", ViewData(c, gin.H{
		"gameID":       gameID,
		"name":         game.Name,
		"releaseYear":  game.ReleaseYear,
		"years":        completionYearOptions(game.ReleaseYear, currentYear),
		"showPlatform": showPlatformFromQuery(c),
	}))
}

func (h *LibraryHandler) CompleteGame(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	gameID, err := strconv.ParseInt(c.Param("game_id"), 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	game, err := models.GetUserGame(c.Request.Context(), h.db, userID, gameID)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	// Toggle off: an already-completed game returns to the backlog.
	if game.Status == "completed" {
		if err := models.UpdateGameStatus(c.Request.Context(), h.db, userID, gameID, "owned"); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		updated, err := models.GetUserGame(c.Request.Context(), h.db, userID, gameID)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Header("HX-Trigger-After-Swap", "libraryUpdated")
		h.renderGameCard(c, updated, showPlatformFromQuery(c))
		return
	}

	currentYear := time.Now().Year()
	needsYear := game.ReleaseYear > 0 && game.ReleaseYear < currentYear

	var completedAt time.Time
	if needsYear {
		yearStr := c.PostForm("completion_year")
		year, err := strconv.Atoi(yearStr)
		if err != nil || year < game.ReleaseYear || year > currentYear {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		completedAt = time.Date(year, 12, 31, 12, 0, 0, 0, time.UTC)
	} else {
		completedAt = time.Now().UTC()
	}

	if err := models.MarkGameCompleted(c.Request.Context(), h.db, userID, gameID, completedAt); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	updated, err := models.GetUserGame(c.Request.Context(), h.db, userID, gameID)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Header("HX-Trigger-After-Swap", "libraryUpdated")
	h.renderGameCard(c, updated, showPlatformFromQuery(c))
}

func showPlatformFromQuery(c *gin.Context) bool {
	return c.Query("show_platform") != "0"
}

func (h *LibraryHandler) SetPlaying(c *gin.Context) {
	h.setGameStatus(c, "playing")
}

func (h *LibraryHandler) SetDropped(c *gin.Context) {
	h.setGameStatus(c, "dropped")
}

func (h *LibraryHandler) setGameStatus(c *gin.Context, status string) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	gameID, err := strconv.ParseInt(c.Param("game_id"), 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	current, err := models.GetUserGame(c.Request.Context(), h.db, userID, gameID)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	// Toggle off: clicking the active status returns the game to the backlog.
	target := status
	if current.Status == status {
		target = "owned"
	}

	if err := models.UpdateGameStatus(c.Request.Context(), h.db, userID, gameID, target); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	updated, err := models.GetUserGame(c.Request.Context(), h.db, userID, gameID)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Header("HX-Trigger-After-Swap", "libraryUpdated")
	h.renderGameCard(c, updated, showPlatformFromQuery(c))
}

func (h *LibraryHandler) renderGameCard(c *gin.Context, game *models.UserGameWithGame, showPlatform bool) {
	c.HTML(http.StatusOK, "library/game_card", toLibraryCard(c, game, showPlatform))
}

func ServeCoverPlaceholder(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=86400, immutable")
	c.Data(http.StatusOK, cover.PlaceholderMIME, cover.Placeholder())
}

func (h *LibraryHandler) ServeGameCover(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	gameID, err := strconv.ParseInt(c.Param("game_id"), 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	inLibrary, err := models.IsInLibrary(c.Request.Context(), h.db, userID, gameID)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if !inLibrary {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	data, mime, err := h.covers.GetOrFetch(c.Request.Context(), gameID)
	if err != nil || len(data) == 0 {
		ServeCoverPlaceholder(c)
		return
	}

	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, mime, data)
}

func (h *LibraryHandler) UpdateStatus(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	gameIDStr := c.Param("game_id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	status := c.PostForm("status")
	validStatuses := map[string]bool{
		"owned": true, "playing": true, "completed": true, "dropped": true,
	}
	if !validStatuses[status] {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	if err := models.UpdateGameStatus(c.Request.Context(), h.db, userID, gameID, status); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Header("HX-Trigger-After-Swap", "libraryUpdated")
	c.HTML(http.StatusOK, "library/status_badge", ViewData(c, gin.H{
		"status": status,
		"gameID": gameID,
	}))
}
