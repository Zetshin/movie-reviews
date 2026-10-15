package models

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Movie struct {
	ID            int
	TMDBID        int
	Title         string
	Description   string
	ReleaseDate   time.Time
	PosterImage   string
	BackdropImage string
	ReviewCount   string
	AvgRating     string
}

// Define a SnippetModel type which wraps a sql.DB connection pool.
type MovieModel struct {
	DB *sql.DB
}

func (m *MovieModel) Count() (int, error) {
	var count int
	err := m.DB.QueryRow(`SELECT COUNT(*) FROM movies`).Scan(&count)
	return count, err
}

func (m *MovieModel) LocalIDForTMDB(tmdbID int) (int, error) {
	var id int
	err := m.DB.QueryRow(`SELECT id FROM movies WHERE tmdb_id = ?`, tmdbID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoRecord
	}
	return id, err
}

func (m *MovieModel) ListPage(limit, offset int) ([]Movie, error) {
	rows, err := m.DB.Query(`SELECT id, title, description, release_date, COALESCE(poster_image, ''), review_count, avg_rating, COALESCE(backdrop_image, '') FROM movies ORDER BY release_date DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	movies := make([]Movie, 0)
	for rows.Next() {
		var movie Movie
		var date sql.NullTime
		if err := rows.Scan(&movie.ID, &movie.Title, &movie.Description, &date, &movie.PosterImage, &movie.ReviewCount, &movie.AvgRating, &movie.BackdropImage); err != nil {
			return nil, err
		}
		movie.ReleaseDate = date.Time
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}

func (m *MovieModel) ImportedTMDBIDs(ids []int) (map[int]bool, error) {
	imported := make(map[int]bool)
	if len(ids) == 0 {
		return imported, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := m.DB.Query(`SELECT tmdb_id FROM movies WHERE tmdb_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		imported[id] = true
	}
	return imported, rows.Err()
}

// This will insert a new snippet into the database.
type ManualCastCredit struct {
	PersonID      int
	CharacterName string
}

func (m *MovieModel) Insert(title string, description string, release_date time.Time, poster_image string, backdropImage string, cast ...ManualCastCredit) (int, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt := `INSERT INTO movies (title, description, release_date, poster_image, backdrop_image)
	VALUES(?,?,?,?,?)`
	var date any
	if !release_date.IsZero() {
		date = release_date
	}
	result, err := tx.Exec(stmt, title, description, date, poster_image, backdropImage)
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
	for order, credit := range cast {
		_, err = tx.Exec(`INSERT INTO movie_persons (movie_id, person_id, role, cast_order, character_name) VALUES (?, ?, 'actor', ?, ?)`, id, credit.PersonID, order, credit.CharacterName)
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(id), nil

}

// This will return a specific snippet based on its id.
func (m *MovieModel) Get(id int) (Movie, error) {
	stmt := `SELECT id, title, description, release_date, COALESCE(poster_image, ''), review_count, avg_rating, COALESCE(backdrop_image, '')
	FROM movies
	WHERE id = ?`

	row := m.DB.QueryRow(stmt, id)

	var s Movie
	var releaseDate sql.NullTime

	err := row.Scan(&s.ID, &s.Title, &s.Description, &releaseDate, &s.PosterImage, &s.ReviewCount, &s.AvgRating, &s.BackdropImage)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Movie{}, ErrNoRecord
		} else {
			return Movie{}, err
		}
	}
	s.ReleaseDate = releaseDate.Time
	return s, nil

}

// Latest returns movies by newest release date, with undated movies last.
// Local ID breaks ties when release dates match.
func (m *MovieModel) Latest(limit int) ([]Movie, error) {
	stmt := `SELECT id, title, description, release_date, COALESCE(poster_image, ''), review_count, avg_rating, COALESCE(backdrop_image, '') FROM movies
	ORDER BY release_date DESC, id DESC LIMIT ?`
	rows, err := m.DB.Query(stmt, limit)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var movies []Movie
	for rows.Next() {
		var s Movie
		var releaseDate sql.NullTime
		err = rows.Scan(&s.ID, &s.Title, &s.Description, &releaseDate, &s.PosterImage, &s.ReviewCount, &s.AvgRating, &s.BackdropImage)
		if err != nil {
			return nil, err
		}
		s.ReleaseDate = releaseDate.Time
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
	WHERE avg_rating > 4.00
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
		var releaseDate sql.NullTime
		if err := rows.Scan(&movie.ID, &movie.Title, &releaseDate, &movie.PosterImage, &movie.AvgRating); err != nil {
			return nil, err
		}
		movie.ReleaseDate = releaseDate.Time
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}

// ImportTMDB saves a movie and its cast atomically. It does not call the API.
// Both tables must have a nullable tmdb_id column with a UNIQUE index.
// Existing records are reused; only a missing backdrop is filled on reimport.
// Other metadata and review statistics are preserved.
func (m *MovieModel) ImportTMDB(movie Movie, cast []ImportedCastMember) (int, error) {
	if movie.TMDBID < 1 || strings.TrimSpace(movie.Title) == "" {
		return 0, fmt.Errorf("TMDB movie must have a positive ID and a title")
	}
	for _, person := range cast {
		if person.TMDBID < 1 || strings.TrimSpace(person.Name) == "" {
			return 0, fmt.Errorf("TMDB cast member must have a positive ID and a name")
		}
	}

	tx, err := m.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var releaseDate any
	if !movie.ReleaseDate.IsZero() {
		releaseDate = movie.ReleaseDate
	}
	result, err := tx.Exec(`INSERT INTO movies
		(tmdb_id, title, description, release_date, poster_image, backdrop_image, review_count, avg_rating)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0.00)
		ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id),
		backdrop_image = IF(COALESCE(backdrop_image, '') = '', ?, backdrop_image)`,
		movie.TMDBID, movie.Title, movie.Description, releaseDate, movie.PosterImage, movie.BackdropImage, movie.BackdropImage)
	if err != nil {
		return 0, err
	}
	movieID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Use a consistent person lock order when different movies share cast.
	cast = append([]ImportedCastMember(nil), cast...)
	sort.Slice(cast, func(i, j int) bool { return cast[i].TMDBID < cast[j].TMDBID })
	for _, person := range cast {
		result, err := tx.Exec(`INSERT INTO persons (tmdb_id, name, profile_image)
			VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
			person.TMDBID, person.Name, person.ProfileImage)
		if err != nil {
			return 0, err
		}
		personID, err := result.LastInsertId()
		if err != nil {
			return 0, err
		}
		// These are local database IDs, not TMDB IDs. Preserve an existing role.
		_, err = tx.Exec(`INSERT INTO movie_persons (movie_id, person_id, role, cast_order, character_name)
			VALUES (?, ?, 'actor', ?, ?)
			ON DUPLICATE KEY UPDATE cast_order = ?, character_name = ?`, movieID, personID, person.Order, person.CharacterName, person.Order, person.CharacterName)
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(movieID), nil
}
