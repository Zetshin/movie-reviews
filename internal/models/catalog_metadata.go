package models

import (
	"database/sql"
	"strings"
)

// Explicit migration; no schema changes run during page requests.
func EnsureCatalogMetadataSchema(db *sql.DB) error {
	columns := []struct{ table, column, statement string }{
		{"movie_persons", "cast_order", `ALTER TABLE movie_persons ADD COLUMN cast_order INT NULL`},
		{"movie_persons", "character_name", `ALTER TABLE movie_persons ADD COLUMN character_name TEXT NULL`},
		{"reviews", "is_spoiler", `ALTER TABLE reviews ADD COLUMN is_spoiler BOOLEAN NOT NULL DEFAULT FALSE`},
	}
	for _, column := range columns {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, column.table, column.column).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err := db.Exec(column.statement); err != nil {
				return err
			}
		}
	}
	return nil
}

// Keep the order of the supplied TMDB list; SQL IN does not guarantee that order.
func (m *MovieModel) HeroByTMDBIDs(ids []int, limit int) ([]Movie, error) {
	if len(ids) == 0 {
		return []Movie{}, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := m.DB.Query(`SELECT id, tmdb_id, title, description, release_date, COALESCE(poster_image, ''), review_count, avg_rating, backdrop_image
		FROM movies WHERE tmdb_id IN (`+placeholders+`) AND COALESCE(TRIM(backdrop_image), '') <> ''`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[int]Movie)
	for rows.Next() {
		var movie Movie
		var date sql.NullTime
		if err := rows.Scan(&movie.ID, &movie.TMDBID, &movie.Title, &movie.Description, &date, &movie.PosterImage, &movie.ReviewCount, &movie.AvgRating, &movie.BackdropImage); err != nil {
			return nil, err
		}
		movie.ReleaseDate = date.Time
		byID[movie.TMDBID] = movie
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	movies := make([]Movie, 0)
	for _, id := range ids {
		if movie, ok := byID[id]; ok {
			movies = append(movies, movie)
			delete(byID, id)
		}
		if len(movies) >= limit {
			break
		}
	}
	return movies, nil
}

func (m *MovieModel) ImportedIDs() ([]int, error) {
	rows, err := m.DB.Query(`SELECT tmdb_id FROM movies WHERE tmdb_id IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int, 0)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
