package handlers

import (
	"net/http"
	"strconv"
	"strings"

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
			"error":    "Failed to load library",
			"username": username,
			"games":    nil,
		})
		return
	}

	c.HTML(http.StatusOK, "library/index", gin.H{
		"username": username,
		"games":    games,
	})
}

func (h *LibraryHandler) LibraryGrid(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	games, err := models.ListUserGames(c.Request.Context(), h.db, userID)
	if err != nil {
		games = nil
	}

	c.HTML(http.StatusOK, "library/game_grid", gin.H{
		"games": games,
	})
}

func (h *LibraryHandler) Search(c *gin.Context) {
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

	platforms := c.PostFormArray("platforms")
	if platforms == nil {
		platforms = []string{}
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
		igdbID, name, coverURL, releaseYear, platforms, cat.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save game"})
		return
	}

	if err := models.AddToLibrary(c.Request.Context(), h.db, userID, game.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add to library"})
		return
	}

	// Return the replacement button fragment
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

	c.Status(http.StatusOK)
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

	// Return updated status badge fragment
	c.HTML(http.StatusOK, "library/status_badge", gin.H{
		"status": status,
		"gameID": gameID,
	})
}
