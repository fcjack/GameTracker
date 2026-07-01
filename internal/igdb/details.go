package igdb

import (
	"fmt"
	"strings"
)

const (
	externalGameCategoryGOG        = 5
	externalGameCategoryEpic       = 13
	externalGameCategoryMetacritic = 36
)

type NamedEntity struct {
	Name string `json:"name"`
}

type InvolvedCompany struct {
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	Developer bool `json:"developer"`
	Publisher bool `json:"publisher"`
}

type ImageAsset struct {
	URL string `json:"url"`
}

type ExternalGame struct {
	Category int    `json:"category"`
	UID      string `json:"uid"`
	URL      string `json:"url"`
}

// GameDetails is the rich IGDB payload used on the game detail page.
type GameDetails struct {
	ID                int64             `json:"id"`
	Name              string            `json:"name"`
	Summary           string            `json:"summary"`
	Storyline         string            `json:"storyline"`
	AggregatedRating  float64           `json:"aggregated_rating"`
	Rating            float64           `json:"rating"`
	TotalRating       float64           `json:"total_rating"`
	RatingCount       int               `json:"rating_count"`
	TotalRatingCount  int               `json:"total_rating_count"`
	FirstReleaseDate  int64             `json:"first_release_date"`
	GameStatus        int               `json:"game_status"`
	Genres            []NamedEntity     `json:"genres"`
	Themes            []NamedEntity     `json:"themes"`
	Keywords          []NamedEntity     `json:"keywords"`
	Platforms         []Platform        `json:"platforms"`
	InvolvedCompanies []InvolvedCompany `json:"involved_companies"`
	Cover             *Cover            `json:"cover"`
	Artworks          []ImageAsset      `json:"artworks"`
	Screenshots       []ImageAsset      `json:"screenshots"`
	ExternalGames     []ExternalGame    `json:"external_games"`
}

func (c *Client) GetGameDetails(id int64) (*GameDetails, error) {
	body := fmt.Sprintf(
		`fields name,summary,storyline,aggregated_rating,rating,total_rating,rating_count,total_rating_count,
first_release_date,game_status,genres.name,themes.name,keywords.name,platforms.name,
involved_companies.company.name,involved_companies.developer,involved_companies.publisher,
cover.url,artworks.url,screenshots.url,external_games.category,external_games.uid,external_games.url;
where id = %d; limit 1;`,
		id,
	)

	var results []GameDetails
	if err := c.post("/games", body, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	d := &results[0]
	if d.Cover != nil {
		d.Cover.URL = normalizeImageURL(d.Cover.URL, "t_cover_big")
	}
	for i := range d.Artworks {
		d.Artworks[i].URL = normalizeImageURL(d.Artworks[i].URL, "t_1080p")
	}
	for i := range d.Screenshots {
		d.Screenshots[i].URL = normalizeImageURL(d.Screenshots[i].URL, "t_1080p")
	}
	return d, nil
}

func normalizeImageURL(raw, size string) string {
	if raw == "" {
		return ""
	}
	url := strings.ReplaceAll(raw, "t_thumb", size)
	if strings.HasPrefix(url, "//") {
		url = "https:" + url
	}
	return url
}

// BackdropURL returns the best wide image for a hero backdrop.
func BackdropURL(d *GameDetails) string {
	if d == nil {
		return ""
	}
	for _, a := range d.Artworks {
		if a.URL != "" {
			return a.URL
		}
	}
	for _, s := range d.Screenshots {
		if s.URL != "" {
			return s.URL
		}
	}
	if d.Cover != nil {
		return d.Cover.URL
	}
	return ""
}

func namedEntities(entities []NamedEntity) []string {
	out := make([]string, 0, len(entities))
	for _, e := range entities {
		if e.Name != "" {
			out = append(out, e.Name)
		}
	}
	return out
}

func (d *GameDetails) GenreNames() []string   { return namedEntities(d.Genres) }
func (d *GameDetails) ThemeNames() []string   { return namedEntities(d.Themes) }
func (d *GameDetails) KeywordNames() []string { return namedEntities(d.Keywords) }

func (d *GameDetails) PlatformNames() []string {
	out := make([]string, 0, len(d.Platforms))
	for _, p := range d.Platforms {
		if p.Name != "" {
			out = append(out, p.Name)
		}
	}
	return out
}

func (d *GameDetails) DeveloperNames() []string {
	var out []string
	for _, ic := range d.InvolvedCompanies {
		if ic.Developer && ic.Company.Name != "" {
			out = append(out, ic.Company.Name)
		}
	}
	return out
}

func (d *GameDetails) PublisherNames() []string {
	var out []string
	for _, ic := range d.InvolvedCompanies {
		if ic.Publisher && ic.Company.Name != "" {
			out = append(out, ic.Company.Name)
		}
	}
	return out
}

// ExternalLinks maps known storefront keys to URLs.
func (d *GameDetails) ExternalLinks() map[string]string {
	links := map[string]string{
		"igdb": fmt.Sprintf("https://www.igdb.com/games/%s", slugify(d.Name)),
	}
	for _, eg := range d.ExternalGames {
		if eg.URL != "" {
			switch eg.Category {
			case externalGameCategorySteam:
				links["steam"] = eg.URL
			case externalGameCategoryGOG:
				links["gog"] = eg.URL
			case externalGameCategoryEpic:
				links["epic"] = eg.URL
			case externalGameCategoryMetacritic:
				links["metacritic"] = eg.URL
			}
			continue
		}
		if eg.UID == "" {
			continue
		}
		switch eg.Category {
		case externalGameCategorySteam:
			links["steam"] = fmt.Sprintf("https://store.steampowered.com/app/%s", eg.UID)
		case externalGameCategoryMetacritic:
			links["metacritic"] = fmt.Sprintf("https://www.metacritic.com/game/%s", eg.UID)
		}
	}
	return links
}

func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "—", "-")
	return strings.ReplaceAll(s, " ", "-")
}
