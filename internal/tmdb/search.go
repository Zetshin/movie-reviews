package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SearchMovies returns the first page of title matches for the search dropdown.
// th-TH requests Thai translations where available; it does not filter by country.
func (c *Client) SearchMovies(query string) (MovieList, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return MovieList{Results: []Movie{}}, nil
	}
	if c.apiKey == "" {
		return MovieList{}, fmt.Errorf("TMDB API key is missing")
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.themoviedb.org/3/search/movie", nil)
	if err != nil {
		return MovieList{}, fmt.Errorf("could not create TMDB search request")
	}
	params := req.URL.Query()
	params.Set("api_key", c.apiKey)
	params.Set("query", query)
	params.Set("page", "1")
	params.Set("language", "th-TH")
	params.Set("include_adult", "false")
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Do not expose the request URL, which contains the API key.
		return MovieList{}, fmt.Errorf("could not contact TMDB")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MovieList{}, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}

	var list MovieList
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return MovieList{}, fmt.Errorf("could not decode TMDB search results: %w", err)
	}
	return list, nil
}
