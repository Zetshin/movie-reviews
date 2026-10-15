package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type PersonDetails struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Biography   string  `json:"biography"`
	Birthday    *string `json:"birthday"`
	ProfilePath *string `json:"profile_path"`
}

// GetPerson prefers Thai biography text, falling back to English if absent.
func (c *Client) GetPerson(tmdbID int) (PersonDetails, error) {
	return c.GetPersonContext(context.Background(), tmdbID)
}

// One deadline covers both Thai and English requests during interactive imports.
func (c *Client) GetPersonContext(ctx context.Context, tmdbID int) (PersonDetails, error) {
	person, err := c.getPerson(ctx, tmdbID, "th-TH")
	if err != nil {
		return PersonDetails{}, err
	}
	if strings.TrimSpace(person.Biography) == "" {
		english, err := c.getPerson(ctx, tmdbID, "en-US")
		if err != nil {
			return PersonDetails{}, err
		}
		person.Biography = english.Biography
	}
	return person, nil
}

func (c *Client) getPerson(ctx context.Context, tmdbID int, language string) (PersonDetails, error) {
	if tmdbID < 1 || c.apiKey == "" {
		return PersonDetails{}, fmt.Errorf("TMDB person requires a positive ID and an API key")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.themoviedb.org/3/person/%d", tmdbID), nil)
	if err != nil {
		return PersonDetails{}, fmt.Errorf("could not create TMDB person request")
	}
	params := req.URL.Query()
	params.Set("api_key", c.apiKey)
	params.Set("language", language)
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PersonDetails{}, fmt.Errorf("could not contact TMDB")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PersonDetails{}, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}
	var person PersonDetails
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&person); err != nil {
		return PersonDetails{}, fmt.Errorf("could not decode TMDB person details: %w", err)
	}
	if person.ID != tmdbID {
		return PersonDetails{}, fmt.Errorf("TMDB returned an unexpected person ID")
	}
	return person, nil
}
