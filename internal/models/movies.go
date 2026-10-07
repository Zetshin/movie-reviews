package models

import (
	"database/sql"
	"errors"
	"time"
)

type Movie struct {
	ID          int
	Title       string
	Description string
	ReleaseDate time.Time
	PosterImage string
	ReviewCount string
	AvgRating   string
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
	stmt := `SELECT id, title, description, release_date, COALESCE(poster_image, ''), review_count, avg_rating
	FROM movies
	WHERE id = ?`

	row := m.DB.QueryRow(stmt, id)

	var s Movie

	err := row.Scan(&s.ID, &s.Title, &s.Description, &s.ReleaseDate, &s.PosterImage, &s.ReviewCount, &s.AvgRating)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Movie{}, ErrNoRecord
		} else {
			return Movie{}, err
		}
	}
	return s, nil

}

// Latest returns up to limit movies ordered by descending ID.
func (m *MovieModel) Latest(limit int) ([]Movie, error) {
	stmt := `SELECT id, title, description, release_date, COALESCE(poster_image, ''), review_count, avg_rating FROM movies
	ORDER BY id DESC LIMIT ?`
	rows, err := m.DB.Query(stmt, limit)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var movies []Movie
	for rows.Next() {
		var s Movie
		err = rows.Scan(&s.ID, &s.Title, &s.Description, &s.ReleaseDate, &s.PosterImage, &s.ReviewCount, &s.AvgRating)
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

// UpdateReviewStats recalculates a movie's stored statistics from all its reviews.
// ReviewModel also refreshes these statistics inside each write transaction.
func (m *MovieModel) UpdateReviewStats(movieID int) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockMovieForReview(tx, movieID); err != nil {
		return err
	}
	if err := updateReviewStats(tx, movieID); err != nil {
		return err
	}
	return tx.Commit()
}

// Serialize review changes for one movie before recalculating its statistics.
func lockMovieForReview(tx *sql.Tx, movieID int) error {
	var id int
	err := tx.QueryRow(`SELECT id FROM movies WHERE id = ? FOR UPDATE`, movieID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoRecord
	}
	return err
}

func updateReviewStats(tx *sql.Tx, movieID int) error {
	stmt := `UPDATE movies
	SET review_count = (SELECT COUNT(*) FROM reviews WHERE movie_id = ?),
		avg_rating = (SELECT COALESCE(AVG(rating), 0) FROM reviews WHERE movie_id = ?)
	WHERE id = ?`

	_, err := tx.Exec(stmt, movieID, movieID, movieID)
	return err
}

func (m *MovieModel) TopPicks() ([]Movie, error) {
	stmt := `SELECT id, title, COALESCE(poster_image, ''), review_count, avg_rating FROM movies
	WHERE avg_rating > 4.00 AND review_count >= 5
	ORDER BY avg_rating DESC, review_count DESC, id DESC LIMIT 3
	`
	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var movies []Movie
	for rows.Next() {
		var s Movie
		err = rows.Scan(&s.ID, &s.Title, &s.PosterImage, &s.ReviewCount, &s.AvgRating)
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

func (m *MovieModel) Search(query string, limit int) ([]Movie, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("search limit must be between 1 and 100")
	}
	stmt := `SELECT id, title, release_date, COALESCE(poster_image, ''), COALESCE(avg_rating, 0)
 FROM movies WHERE title LIKE ? ORDER BY title ASC, id DESC LIMIT ?`
	rows, err := m.DB.Query(stmt, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	movies := make([]Movie, 0)
	for rows.Next() {
		var movie Movie
		if err := rows.Scan(&movie.ID, &movie.Title, &movie.ReleaseDate, &movie.PosterImage, &movie.AvgRating); err != nil {
			return nil, err
		}
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}
