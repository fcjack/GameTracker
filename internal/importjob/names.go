package importjob

import "github.com/jacksoncoelho/game-tracker/internal/gamename"

// namesMatch reports whether a Steam title and IGDB title likely refer to the same game.
func namesMatch(steamName, igdbName string) bool {
	return gamename.Match(steamName, igdbName)
}
