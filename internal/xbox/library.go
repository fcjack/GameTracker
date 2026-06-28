package xbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// OwnedGame is a title from the user's Xbox title history.
type OwnedGame struct {
	TitleID  int
	Name     string
	ImageURL string
}

const titleHistoryDecorations = "TitleHistory,GamePass,image,detail,achievement"

type titleHistoryResponse struct {
	Titles []titleHistoryEntry `json:"titles"`
}

type titleHistoryEntry struct {
	TitleID      json.Number `json:"titleId"`
	Name         string      `json:"name"`
	DisplayImage string      `json:"displayImage"`
	TitleHistory struct {
		LastTimePlayed string `json:"lastTimePlayed"`
	} `json:"titleHistory"`
	GamePass struct {
		IsGamePass bool `json:"isGamePass"`
	} `json:"gamePass"`
	Detail struct {
		Name              string   `json:"name"`
		Programs          []string `json:"programs"`
		UserPrograms      []string `json:"userPrograms"`
		UserSubscriptions []string `json:"userSubscriptions"`
	} `json:"detail"`
	Achievement struct {
		CurrentAchievements int `json:"currentAchievements"`
		CurrentGamerscore   int `json:"currentGamerscore"`
	} `json:"achievement"`
}

// GetOwnedGames fetches the user's library via Title Hub title history, including
// purchased titles and subscription games the user has actually played.
func (c *Client) GetOwnedGames(ctx context.Context, accessToken string) ([]OwnedGame, error) {
	session, err := c.Authenticate(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf(
		"%s/users/xuid(%s)/titles/titlehistory/decoration/%s",
		c.titleHubURL,
		session.XUID,
		titleHistoryDecorations,
	)

	var parsed titleHistoryResponse
	if err := c.getXBLJSON(ctx, endpoint, session, "2", &parsed); err != nil {
		return nil, err
	}

	return parseOwnedGames(parsed.Titles)
}

func parseOwnedGames(entries []titleHistoryEntry) ([]OwnedGame, error) {
	if entries == nil {
		return []OwnedGame{}, nil
	}

	games := make([]OwnedGame, 0, len(entries))
	for _, entry := range entries {
		if !shouldImportTitle(entry) {
			continue
		}

		titleID, err := entry.TitleID.Int64()
		if err != nil || titleID <= 0 {
			continue
		}

		name := entry.Name
		if name == "" {
			name = entry.Detail.Name
		}

		games = append(games, OwnedGame{
			TitleID:  int(titleID),
			Name:     name,
			ImageURL: entry.DisplayImage,
		})
	}
	return games, nil
}

// shouldImportTitle keeps purchased titles and subscription titles only when the
// user has play activity (last played time or earned achievements).
func shouldImportTitle(entry titleHistoryEntry) bool {
	if !entry.isSubscriptionTitle() {
		return true
	}
	return entry.hasPlayActivity()
}

func (entry titleHistoryEntry) isSubscriptionTitle() bool {
	if entry.GamePass.IsGamePass {
		return true
	}
	for _, value := range entry.Detail.Programs {
		if isGamePassProgram(value) {
			return true
		}
	}
	for _, value := range entry.Detail.UserPrograms {
		if isGamePassProgram(value) {
			return true
		}
	}
	for _, value := range entry.Detail.UserSubscriptions {
		if isGamePassSubscription(value) {
			return true
		}
	}
	return false
}

func isGamePassProgram(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(upper, "GP") || strings.Contains(upper, "GAMEPASS")
}

func isGamePassSubscription(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(upper, "XGP") || strings.Contains(upper, "GAMEPASS")
}

func (entry titleHistoryEntry) hasPlayActivity() bool {
	if entry.TitleHistory.LastTimePlayed != "" {
		return true
	}
	if entry.Achievement.CurrentAchievements > 0 {
		return true
	}
	return entry.Achievement.CurrentGamerscore > 0
}
