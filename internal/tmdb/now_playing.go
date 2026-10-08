package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// NowPlayingMovies returns one page of movies currently in theatres in Thailand.
// Thai translations are requested where available. This does not write to MySQL.
func (c *Client) NowPlayingMovies(page int) (MovieList, error) {
	if page < 1 || page > 500 {
		return MovieList{}, fmt.Errorf("TMDB page must be between 1 and 500")
	}
	if c.apiKey == "" {
		return MovieList{}, fmt.Errorf("TMDB API key is missing")
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.themoviedb.org/3/movie/now_playing", nil)
	if err != nil {
		return MovieList{}, fmt.Errorf("could not create TMDB now-playing request")
	}
	params := req.URL.Query()
	params.Set("api_key", c.apiKey)
	params.Set("page", strconv.Itoa(page))
	params.Set("language", "th-TH")
	params.Set("region", "TH")
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Avoid exposing the request URL, which contains the API key.
		return MovieList{}, fmt.Errorf("could not contact TMDB")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MovieList{}, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}
	var list MovieList
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return MovieList{}, fmt.Errorf("could not decode TMDB now-playing list: %w", err)
	}
	return list, nil
}
