package models

import (
	"database/sql"
	"errors"
	"time"
)

type Movie struct {
	ID           int
	Title        string
	Description  string
	Release_date time.Time
	Poster_image string
}

// Define a SnippetModel type which wraps a sql.DB connection pool.
type MovieModel struct {
	DB *sql.DB
}

// This will insert a new snippet into the database.
func (m *MovieModel) Insert(title string, description string, release_date time.Time, poster_image string) (int, error) {
	stmt := `INSERT INTO movies (title, description, release_date, poster_image)
	VALUES(?,?,?,?)`
	result, err := m.DB.Exec(stmt, title, description, release_date, poster_image)
	if err != nil {
		return 0, err
	}
	// Use the LastInsertId() method on the result to get the ID of our
	// newly inserted record in the snippets table.
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	// The ID returned has the type int64, so we convert it to an int type
	// before returning.
	return int(id), nil

}

// This will return a specific snippet based on its id.
func (m *MovieModel) Get(id int) (Movie, error) {
	stmt := `SELECT id, title, description, release_date, poster_image FROM movies
	WHERE id = ?`

	row := m.DB.QueryRow(stmt, id)

	var s Movie

	err := row.Scan(&s.ID, &s.Title, &s.Description, &s.Release_date, &s.Poster_image)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Movie{}, ErrNoRecord
		} else {
			return Movie{}, err
		}
	}
	return s, nil

}

// This will return the 10 most recently created snippets.
func (m *MovieModel) Latest() ([]Movie, error) {
	stmt := `SELECT id, title, description, release_date, poster_image FROM movies
	ORDER BY id DESC LIMIT 10`
	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var movies []Movie
	for rows.Next() {
		var s Movie
		err = rows.Scan(&s.ID, &s.Title, &s.Description, &s.Release_date, &s.Poster_image)
		if err != nil {
			return nil, err
		}
		movies = append(movies, s)

	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return movies, nil
}
