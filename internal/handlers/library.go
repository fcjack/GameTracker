package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type LibraryHandler struct {
	db   *pgxpool.Pool
	igdb *igdb.Client
}

type libraryGameCard struct {
	*models.UserGameWithGame
	ShowPlatform        bool
	NeedsCompletionYear bool
	CurrentYear         int
}

type libraryGameGroup struct {
	Label string
	Games []libraryGameCard
}

func toLibraryCard(g *models.UserGameWithGame, showPlatform bool) libraryGameCard {
	currentYear := time.Now().Year()
	return libraryGameCard{
		UserGameWithGame:    g,
		ShowPlatform:        showPlatform,
		CurrentYear:         currentYear,
		NeedsCompletionYear: g.ReleaseYear > 0 && g.ReleaseYear < currentYear,
	}
}

func toLibraryCards(games []*models.UserGameWithGame, showPlatform bool) []libraryGameCard {
	cards := make([]libraryGameCard, len(games))
	for i, g := range games {
		cards[i] = toLibraryCard(g, showPlatform)
	}
	return cards
}

func toLibraryPlatformGroups(games []*models.UserGameWithGame) []libraryGameGroup {
	grouped := models.GroupUserGamesByPlatform(games)
	groups := make([]libraryGameGroup, len(grouped))
	for i, g := range grouped {
		groups[i] = libraryGameGroup{
			Label: g.Platform,
			Games: toLibraryCards(g.Games, false),
		}
	}
	return groups
}

func toLibraryYearGroups(games []*models.UserGameWithGame) []libraryGameGroup {
	grouped := models.GroupUserGamesByCompletionYear(games)
	groups := make([]libraryGameGroup, len(grouped))
	for i, g := range grouped {
		groups[i] = libraryGameGroup{
			Label: g.Label,
			Games: toLibraryCards(g.Games, true),
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
	return &LibraryHandler{db: db, igdb: igdbClient}
}

func (h *LibraryHandler) LibraryPage(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	username := session.Get("username").(string)

	games, err := models.ListUserGames(c.Request.Context(), h.db, userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "library/index", gin.H{
			"error":     "Failed to load library",
			"username":  username,
			"activeNav": "library",
			"games":     nil,
		})
		return
	}

	c.HTML(http.StatusOK, "library/index", gin.H{
		"username":  username,
		"activeNav": "library",
		"games":     games,
	})
}

func (h *LibraryHandler) LibraryGrid(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	var games []*models.UserGameWithGame
	var err error
	if c.Query("filter") == "active" {
		games, err = models.ListUserGamesByStatuses(c.Request.Context(), h.db, userID, []string{"playing", "completed"})
	} else {
		games, err = models.ListUserGames(c.Request.Context(), h.db, userID)
	}
	if err != nil {
		games = nil
	}

	data := gin.H{
		"hasGames": len(games) > 0,
	}
	if c.Query("filter") == "active" {
		data["emptyTitle"] = "No active games"
		data["emptyHint"] = "Games you're playing or have completed appear here. Backlog and dropped games are on the full library page."
	}
	switch c.Query("group_by") {
	case "platform":
		data["groups"] = toLibraryPlatformGroups(games)
	case "year":
		data["groups"] = toLibraryYearGroups(games)
	default:
		data["games"] = toLibraryCards(games, true)
	}

	c.HTML(http.StatusOK, "library/game_grid", data)
}

func (h *LibraryHandler) SearchLibrary(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.HTML(http.StatusOK, "library/library_search_results", gin.H{})
		return
	}

	games, err := models.SearchUserGames(c.Request.Context(), h.db, userID, query, 20)
	if err != nil {
		c.HTML(http.StatusOK, "library/library_search_results", gin.H{
			"error": "Search failed. Please try again.",
		})
		return
	}

	c.HTML(http.StatusOK, "library/library_search_results", gin.H{
		"searched": true,
		"games":    toLibraryCards(games, true),
	})
}

func (h *LibraryHandler) SearchIGDB(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.HTML(http.StatusOK, "library/search_results", gin.H{"results": nil})
		return
	}

	results, err := h.igdb.Search(query, 10)
	if err != nil {
		c.HTML(http.StatusOK, "library/search_results", gin.H{
			"error": "Search failed. Please try again.",
		})
		return
	}

	type resultWithStatus struct {
		igdb.SearchResult
		InLibrary   bool
		ReleaseYear int
		CoverURL    string
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

		// Check if this IGDB game is already in the DB and in the user's library
		var internalID int64
		err := h.db.QueryRow(c.Request.Context(),
			`SELECT id FROM games WHERE igdb_id = $1`, r.ID,
		).Scan(&internalID)
		if err == nil {
			// Game exists in DB; check user_games
			in, _ := models.IsInLibrary(c.Request.Context(), h.db, userID, internalID)
			rws.InLibrary = in
		}

		enriched = append(enriched, rws)
	}

	c.HTML(http.StatusOK, "library/search_results", gin.H{
		"results": enriched,
		"userID":  userID,
	})
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

	if err := models.AddToLibrary(c.Request.Context(), h.db, userID, game.ID, platform); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add to library"})
		return
	}

	c.Header("HX-Trigger-After-Swap", "libraryUpdated")
	c.HTML(http.StatusOK, "library/in_library_button", gin.H{
		"gameID": game.ID,
	})
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
	c.HTML(http.StatusOK, "library/complete_form", gin.H{
		"gameID":       gameID,
		"name":         game.Name,
		"releaseYear":  game.ReleaseYear,
		"years":        completionYearOptions(game.ReleaseYear, currentYear),
		"showPlatform": showPlatformFromQuery(c),
	})
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
	if game.Status == "completed" {
		h.renderGameCard(c, game, showPlatformFromQuery(c))
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

	if err := models.UpdateGameStatus(c.Request.Context(), h.db, userID, gameID, status); err != nil {
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
	c.HTML(http.StatusOK, "library/game_card", toLibraryCard(game, showPlatform))
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
	c.HTML(http.StatusOK, "library/status_badge", gin.H{
		"status": status,
		"gameID": gameID,
	})
}
