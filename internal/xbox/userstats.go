package xbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

var ErrPlaytimeNotFound = errors.New("xbox: playtime not found")

const statMinutesPlayed = "MinutesPlayed"

type userStatsBatchRequest struct {
	ArrangeByField string             `json:"arrangebyfield"`
	Groups         []userStatsGroup   `json:"groups"`
	Stats          []userStatsStatReq `json:"stats"`
	XUIDs          []string           `json:"xuids"`
}

type userStatsGroup struct {
	Name    string `json:"name"`
	TitleID string `json:"titleid,omitempty"`
	SCID    string `json:"scid,omitempty"`
}

type userStatsStatReq struct {
	Name    string `json:"name"`
	TitleID string `json:"titleid,omitempty"`
	SCID    string `json:"scid,omitempty"`
}

type userStatsBatchResponse struct {
	Groups              []batchGroup          `json:"groups"`
	StatListsCollection []statListsCollection `json:"statlistscollection"`
}

type batchGroup struct {
	TitleID             string                `json:"titleid"`
	TitleIDCamel        string                `json:"titleId"`
	StatListsCollection []statListsCollection `json:"statlistscollection"`
}

type statListsCollection struct {
	ArrangeByField   string          `json:"arrangebyfield"`
	ArrangeByFieldID string          `json:"arrangebyfieldid"`
	Stats            []userStatsStat `json:"stats"`
}

type userStatsGetResponse struct {
	User struct {
		Stats []userStatsStat `json:"stats"`
	} `json:"user"`
}

type userStatsStat struct {
	Name     string          `json:"name"`
	StatName string          `json:"statname"`
	Value    json.RawMessage `json:"value"`
}

func (s userStatsStat) isMinutesPlayed() bool {
	return strings.EqualFold(s.Name, statMinutesPlayed) || strings.EqualFold(s.StatName, statMinutesPlayed)
}

func statValueMinutes(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		minutes, err := strconv.Atoi(asString)
		return minutes, err == nil && minutes >= 0
	}

	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return asInt, asInt >= 0
	}

	var asFloat float64
	if err := json.Unmarshal(raw, &asFloat); err == nil && asFloat >= 0 {
		return int(asFloat), true
	}

	return 0, false
}

// titleIDToSCID derives the GDK-style SCID from a decimal Xbox title ID.
func titleIDToSCID(titleID int) string {
	if titleID <= 0 {
		return ""
	}
	return fmt.Sprintf("00000000-0000-0000-0000-%012x", titleID)
}

// SetUserStatsURL overrides the User Stats base URL. Used by tests.
func (c *Client) SetUserStatsURL(userStatsURL string) {
	if userStatsURL != "" {
		c.userStatsURL = userStatsURL
	}
}

// GetMinutesPlayedForTitles fetches MinutesPlayed stats for the given title IDs.
func (c *Client) GetMinutesPlayedForTitles(ctx context.Context, session *XSTSSession, titleIDs []int) (map[int]int, error) {
	result := make(map[int]int, len(titleIDs))
	for _, titleID := range titleIDs {
		minutes, ok, err := c.getMinutesPlayedForTitle(ctx, session, titleID, "")
		if err != nil {
			continue
		}
		if ok {
			result[titleID] = minutes
		}
	}
	return result, nil
}

// FetchMinutesPlayed returns total minutes played for a title, or ErrPlaytimeNotFound.
func (c *Client) FetchMinutesPlayed(ctx context.Context, session *XSTSSession, titleID int, scid string) (int, error) {
	minutes, ok, err := c.getMinutesPlayedForTitle(ctx, session, titleID, scid)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrPlaytimeNotFound
	}
	return minutes, nil
}

// EnrichGamePlaytime fetches playtime for a single title and stores it on the game.
func (c *Client) EnrichGamePlaytime(ctx context.Context, session *XSTSSession, game *OwnedGame) {
	if game == nil || session == nil {
		return
	}

	minutes, ok, err := c.getMinutesPlayedForTitle(ctx, session, game.TitleID, game.SCID)
	if err != nil {
		slog.Warn("xbox playtime fetch failed",
			"title_id", game.TitleID,
			"name", game.Name,
			"error", err,
		)
		return
	}
	if ok {
		game.PlaytimeMinutes = &minutes
	}
}

func (c *Client) getMinutesPlayedForTitle(ctx context.Context, session *XSTSSession, titleID int, scid string) (minutes int, ok bool, err error) {
	if minutes, ok, err = c.getMinutesPlayedByTitleBatch(ctx, session, titleID); ok {
		return minutes, true, nil
	}
	if err != nil && !isUserStatsNotFound(err) {
		// Best-effort: still try SCID-based lookups below.
		slog.Debug("xbox title batch playtime failed",
			"title_id", titleID,
			"error", err,
		)
	}

	var lastErr error
	if err != nil {
		lastErr = err
	}

	candidates, _ := c.scidCandidates(ctx, session, titleID, scid)
	if len(candidates) == 0 {
		return 0, false, lastErr
	}
	for _, candidate := range candidates {
		if minutes, ok, batchErr := c.getMinutesPlayedBySCIDBatch(ctx, session, candidate); ok {
			c.cacheSCID(titleID, candidate)
			return minutes, true, nil
		} else if batchErr != nil && !isUserStatsNotFound(batchErr) {
			lastErr = batchErr
		}

		minutes, ok, getErr := c.getMinutesPlayedBySCID(ctx, session, candidate)
		if getErr != nil {
			if isUserStatsNotFound(getErr) {
				continue
			}
			lastErr = getErr
			continue
		}
		if ok {
			c.cacheSCID(titleID, candidate)
			return minutes, true, nil
		}
	}
	return 0, false, lastErr
}

func (c *Client) scidCandidates(ctx context.Context, session *XSTSSession, titleID int, scid string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}

	add(scid)
	if cached, ok := c.cachedSCID(titleID); ok {
		add(cached)
	}
	if resolved, resolveErr := c.resolveSCID(ctx, session, titleID); resolveErr != nil {
		slog.Debug("xbox scid resolve failed",
			"title_id", titleID,
			"error", resolveErr,
		)
	} else {
		add(resolved)
	}
	add(titleIDToSCID(titleID))
	return out, nil
}

func (c *Client) cacheSCID(titleID int, scid string) {
	if c == nil || titleID <= 0 || strings.TrimSpace(scid) == "" {
		return
	}
	c.scidCache.Store(titleID, strings.TrimSpace(scid))
}

func (c *Client) cachedSCID(titleID int) (string, bool) {
	if c == nil || titleID <= 0 {
		return "", false
	}
	if value, ok := c.scidCache.Load(titleID); ok {
		if scid, ok := value.(string); ok && scid != "" {
			return scid, true
		}
	}
	return "", false
}

func (c *Client) resolveSCID(ctx context.Context, session *XSTSSession, titleID int) (string, error) {
	if cached, ok := c.cachedSCID(titleID); ok {
		return cached, nil
	}

	endpoint := fmt.Sprintf(
		"%s/users/xuid(%s)/titles/titleid(%d)/decoration/scid",
		c.titleHubURL,
		session.XUID,
		titleID,
	)

	var parsed titleHistoryResponse
	if err := c.getXBLJSON(ctx, endpoint, session, "2", &parsed); err != nil {
		if isUserStatsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	for _, entry := range parsed.Titles {
		id, err := entry.TitleID.Int64()
		if err != nil || int(id) != titleID {
			continue
		}
		if scid := entry.titleSCID(); scid != "" {
			c.cacheSCID(titleID, scid)
			return scid, nil
		}
	}
	if len(parsed.Titles) == 1 {
		if scid := parsed.Titles[0].titleSCID(); scid != "" {
			c.cacheSCID(titleID, scid)
			return scid, nil
		}
	}
	return "", nil
}

func isUserStatsNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, " 404") ||
		strings.Contains(msg, "404:") ||
		strings.Contains(msg, "not found")
}

func (c *Client) getMinutesPlayedByTitleBatch(ctx context.Context, session *XSTSSession, titleID int) (int, bool, error) {
	id := strconv.Itoa(titleID)
	payload := userStatsBatchRequest{
		ArrangeByField: "xuid",
		Groups:         []userStatsGroup{{Name: "Hero", TitleID: id}},
		Stats:          []userStatsStatReq{{Name: statMinutesPlayed, TitleID: id}},
		XUIDs:          []string{session.XUID},
	}

	endpoint := c.userStatsURL + "/batch"
	var parsed userStatsBatchResponse
	if err := c.postXBLJSON(ctx, endpoint, session, payload, &parsed, "2"); err != nil {
		if isUserStatsNotFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}

	if minutes, ok := minutesFromBatch(parsed, titleID); ok {
		return minutes, true, nil
	}
	return 0, false, nil
}

func (c *Client) getMinutesPlayedBySCIDBatch(ctx context.Context, session *XSTSSession, scid string) (int, bool, error) {
	payload := userStatsBatchRequest{
		ArrangeByField: "xuid",
		Groups:         []userStatsGroup{{Name: "Hero", SCID: scid}},
		Stats:          []userStatsStatReq{{Name: statMinutesPlayed, SCID: scid}},
		XUIDs:          []string{session.XUID},
	}

	endpoint := c.userStatsURL + "/batch"
	var parsed userStatsBatchResponse
	if err := c.postXBLJSON(ctx, endpoint, session, payload, &parsed, "2"); err != nil {
		if isUserStatsNotFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}

	if minutes, ok := minutesFromBatch(parsed, 0); ok {
		return minutes, true, nil
	}
	return 0, false, nil
}

func (c *Client) getMinutesPlayedBySCID(ctx context.Context, session *XSTSSession, scid string) (int, bool, error) {
	endpoint := fmt.Sprintf("%s/users/xuid(%s)/scids/%s/stats/%s", c.userStatsURL, session.XUID, scid, statMinutesPlayed)
	var parsed userStatsGetResponse
	if err := c.getXBLJSON(ctx, endpoint, session, "2", &parsed); err != nil {
		return 0, false, err
	}

	for _, stat := range parsed.User.Stats {
		if !stat.isMinutesPlayed() {
			continue
		}
		minutes, ok := statValueMinutes(stat.Value)
		if ok {
			return minutes, true, nil
		}
	}
	return 0, false, nil
}

func minutesFromStats(stats []userStatsStat) (int, bool) {
	for _, stat := range stats {
		if !stat.isMinutesPlayed() {
			continue
		}
		if minutes, ok := statValueMinutes(stat.Value); ok {
			return minutes, true
		}
	}
	return 0, false
}

func minutesFromBatch(parsed userStatsBatchResponse, wantTitleID int) (int, bool) {
	byTitle := make(map[int]int)
	var unattached []int

	collect := func(titleIDStr string, stats []userStatsStat) {
		minutes, ok := minutesFromStats(stats)
		if !ok {
			return
		}
		if titleIDStr == "" {
			unattached = append(unattached, minutes)
			return
		}
		titleID, err := strconv.Atoi(titleIDStr)
		if err != nil || titleID <= 0 {
			unattached = append(unattached, minutes)
			return
		}
		byTitle[titleID] = minutes
	}

	for _, group := range parsed.Groups {
		titleIDStr := group.TitleID
		if titleIDStr == "" {
			titleIDStr = group.TitleIDCamel
		}
		for _, collection := range group.StatListsCollection {
			collect(titleIDStr, collection.Stats)
		}
	}
	for _, collection := range parsed.StatListsCollection {
		collect(collection.ArrangeByFieldID, collection.Stats)
		if len(collection.Stats) > 0 && len(unattached) == 0 {
			if minutes, ok := minutesFromStats(collection.Stats); ok {
				unattached = append(unattached, minutes)
			}
		}
	}

	if wantTitleID > 0 {
		if minutes, ok := byTitle[wantTitleID]; ok {
			return minutes, true
		}
	}
	if len(byTitle) == 1 {
		for _, minutes := range byTitle {
			return minutes, true
		}
	}
	if len(unattached) == 1 {
		return unattached[0], true
	}
	if wantTitleID == 0 && len(unattached) > 0 {
		return unattached[0], true
	}
	return 0, false
}

func parseMinutesPlayedBatchResponse(parsed userStatsBatchResponse) (map[int]int, error) {
	result := make(map[int]int)
	for _, group := range parsed.Groups {
		titleIDStr := group.TitleID
		if titleIDStr == "" {
			titleIDStr = group.TitleIDCamel
		}
		titleID, err := strconv.Atoi(titleIDStr)
		if err != nil || titleID <= 0 {
			continue
		}
		for _, collection := range group.StatListsCollection {
			if minutes, ok := minutesFromStats(collection.Stats); ok {
				result[titleID] = minutes
			}
		}
	}
	return result, nil
}

func parseMinutesPlayedBatchBody(body []byte) (map[int]int, error) {
	var parsed userStatsBatchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("xbox: decode user stats response: %w", err)
	}
	return parseMinutesPlayedBatchResponse(parsed)
}
