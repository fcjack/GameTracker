package playtime

// Kind identifies which playtime handler should process an event.
type Kind string

const (
	KindXbox  Kind = "xbox"
	KindSteam Kind = "steam"
)

// Event is a unit of playtime work published after library import/sync.
type Event struct {
	Kind   Kind
	UserID int64

	// Xbox User Stats lookup
	TitleID int
	SCID    string
	Name    string

	// Steam playtime already known from library response
	AppID   int
	Minutes int
}
