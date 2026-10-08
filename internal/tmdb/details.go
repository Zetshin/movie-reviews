package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// MovieDetails extends the list fields with details and the appended credits.
// All IDs here are TMDB IDs, not IDs from our MySQL database.
type MovieDetails struct {
	Movie
	Runtime *int    `json:"runtime"`
	Genres  []Genre `json:"genres"`
	Credits Credits `json:"credits"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Credits struct {
	Cast []CastMember `json:"cast"`
}

type CastMember struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Character    string  `json:"character"`
	ProfilePath  *string `json:"profile_path"`
	Order        int     `json:"order"`
	CreditID     string  `json:"credit_id"`
}

// GetMovie fetches one movie and its cast in one request.
// It does not read .env or write to MySQL.
func (c *Client) GetMovie(tmdbID int) (MovieDetails, error) {
	if tmdbID < 1 {
		return MovieDetails{}, fmt.Errorf("TMDB movie ID must be positive")
	}
	if c.apiKey == "" {
		return MovieDetails{}, fmt.Errorf("TMDB API key is missing")
	}

	endpoint := fmt.Sprintf("https://api.themoviedb.org/3/movie/%d", tmdbID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return MovieDetails{}, fmt.Errorf("could not create TMDB details request")
	}
	params := req.URL.Query()
	params.Set("api_key", c.apiKey)
	params.Set("language", "th-TH")
	params.Set("append_to_response", "credits")
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Transport errors may contain the URL and its API key.
		return MovieDetails{}, fmt.Errorf("could not contact TMDB")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MovieDetails{}, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}

	var movie MovieDetails
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&movie); err != nil {
		return MovieDetails{}, fmt.Errorf("could not decode TMDB movie details: %w", err)
	}
	return movie, nil
}
