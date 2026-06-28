package xbox

import (
	"encoding/json"
	"testing"
)

func TestParseOwnedGamesIncludesPlayedGamePassTitle(t *testing.T) {
	t.Parallel()

	entry := titleHistoryEntry{
		TitleID: json.Number("2071061510"),
		Name:    "Lies of P",
	}
	entry.GamePass.IsGamePass = true
	entry.TitleHistory.LastTimePlayed = "2024-05-31T12:02:41.6829304Z"

	games, err := parseOwnedGames([]titleHistoryEntry{entry})
	if err != nil {
		t.Fatalf("parseOwnedGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("parseOwnedGames() returned %d games, want 1", len(games))
	}
	if games[0].TitleID != 2071061510 {
		t.Errorf("TitleID = %d, want 2071061510", games[0].TitleID)
	}
}

func TestParseOwnedGamesSkipsInactiveGamePassTitle(t *testing.T) {
	t.Parallel()

	entry := titleHistoryEntry{
		TitleID: json.Number("2071061510"),
		Name:    "Unplayed Game Pass Game",
	}
	entry.Detail.Programs = []string{"GPULTIMATE"}

	games, err := parseOwnedGames([]titleHistoryEntry{entry})
	if err != nil {
		t.Fatalf("parseOwnedGames() error = %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("parseOwnedGames() returned %d games, want 0", len(games))
	}
}

func TestParseOwnedGamesIncludesPurchasedTitleWithoutActivity(t *testing.T) {
	t.Parallel()

	games, err := parseOwnedGames([]titleHistoryEntry{{
		TitleID: json.Number("1144039928"),
		Name:    "Owned Game",
	}})
	if err != nil {
		t.Fatalf("parseOwnedGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("parseOwnedGames() returned %d games, want 1", len(games))
	}
}

func TestParseOwnedGamesIncludesGamePassTitleWithAchievements(t *testing.T) {
	t.Parallel()

	entry := titleHistoryEntry{
		TitleID: json.Number("374923716"),
		Name:    "Gears 5",
	}
	entry.Detail.UserSubscriptions = []string{"XGPULTIMATE"}
	entry.Achievement.CurrentAchievements = 3

	games, err := parseOwnedGames([]titleHistoryEntry{entry})
	if err != nil {
		t.Fatalf("parseOwnedGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("parseOwnedGames() returned %d games, want 1", len(games))
	}
}
