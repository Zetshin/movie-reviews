package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client calls TMDB using an API key supplied by the caller.
// It does not read .env or write to the database.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			// Keep credentials on the original endpoint rather than following redirects.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// DiscoverMovies fetches one page. It does not import movies into MySQL.
func (c *Client) DiscoverMovies(page int) (MovieList, error) {
	if page < 1 || page > 500 {
		return MovieList{}, fmt.Errorf("TMDB page must be between 1 and 500")
	}
	if c.apiKey == "" {
		return MovieList{}, fmt.Errorf("TMDB API key is missing")
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.themoviedb.org/3/discover/movie", nil)
	if err != nil {
		return MovieList{}, fmt.Errorf("could not create TMDB request")
	}
	params := req.URL.Query()
	params.Set("api_key", c.apiKey)
	params.Set("page", strconv.Itoa(page))
	params.Set("language", "th-TH")
	params.Set("sort_by", "popularity.desc")
	params.Set("include_adult", "false")
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Transport errors can contain the request URL and its API key.
		return MovieList{}, fmt.Errorf("could not contact TMDB")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MovieList{}, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}

	var list MovieList
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return MovieList{}, fmt.Errorf("could not decode TMDB movie list: %w", err)
	}
	return list, nil
}
