package models

import (
	"database/sql"
	"time"
)

type Movie struct {
	ID          int
	Title       string
	Description string
	ReleaseDate time.Time
	PosterImage string
}

// Define a SnippetModel type which wraps a sql.DB connection pool.
type MovieModel struct {
	DB *sql.DB
}

// This will insert a new snippet into the database.
func (m *MovieModel) Insert(title string, description string, releaseDate time.Time, posterImage string) (int, error) {
	return 0, nil
}

// This will return a specific snippet based on its id.
func (m *MovieModel) Get(id int) (Movie, error) {
	return Movie{}, nil
}

// This will return the 10 most recently created snippets.
func (m *MovieModel) Latest() ([]Movie, error) {
	return nil, nil
}
