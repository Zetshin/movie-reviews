package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type PeopleList struct {
	Page       int              `json:"page"`
	Results    []TrendingPerson `json:"results"`
	TotalPages int              `json:"total_pages"`
}

func (c *Client) SearchPeople(ctx context.Context, query string, page int) (PeopleList, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return PeopleList{Results: []TrendingPerson{}}, nil
	}
	return c.peopleList(ctx, "/search/person", query, page)
}

func (c *Client) PopularPeople(ctx context.Context, page int) (PeopleList, error) {
	return c.peopleList(ctx, "/person/popular", "", page)
}

func (c *Client) peopleList(ctx context.Context, path, query string, page int) (PeopleList, error) {
	if page < 1 || page > 500 || c.apiKey == "" {
		return PeopleList{}, fmt.Errorf("TMDB people require a valid page and API key")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.themoviedb.org/3"+path, nil)
	if err != nil {
		return PeopleList{}, fmt.Errorf("could not create TMDB people request")
	}
	params := req.URL.Query()
	params.Set("api_key", c.apiKey)
	params.Set("page", strconv.Itoa(page))
	params.Set("language", "th-TH")
	if query != "" {
		params.Set("query", query)
		params.Set("include_adult", "false")
	}
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PeopleList{}, fmt.Errorf("could not contact TMDB")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PeopleList{}, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}
	var list PeopleList
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return PeopleList{}, fmt.Errorf("could not decode TMDB people")
	}
	return list, nil
}
