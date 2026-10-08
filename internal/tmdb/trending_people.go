package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type TrendingPerson struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	ProfilePath *string `json:"profile_path"`
}

type TrendingPeopleList struct {
	Results []TrendingPerson `json:"results"`
}

// TrendingPeople requests TMDB's weekly ordering, not people ordered by local ID.
func (c *Client) TrendingPeople() (TrendingPeopleList, error) {
	if c.apiKey == "" {
		return TrendingPeopleList{}, fmt.Errorf("TMDB API key is missing")
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.themoviedb.org/3/trending/person/week", nil)
	if err != nil {
		return TrendingPeopleList{}, fmt.Errorf("could not create TMDB trending people request")
	}
	params := req.URL.Query()
	params.Set("api_key", c.apiKey)
	params.Set("language", "th-TH")
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TrendingPeopleList{}, fmt.Errorf("could not contact TMDB")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TrendingPeopleList{}, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}
	var list TrendingPeopleList
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return TrendingPeopleList{}, fmt.Errorf("could not decode TMDB trending people: %w", err)
	}
	return list, nil
}
