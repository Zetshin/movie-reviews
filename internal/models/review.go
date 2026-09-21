package models

import (
	"database/sql"
	"errors"
	"time"
)

type Review struct {
    ID        int
    MovieID   int
    UserID    int
    Rating    int
    Review    string
    CreatedAt time.Time
    UpdatedAt time.Time
}
type ReviewModel struct {
	DB *sql.DB
}


func (m *ReviewModel) Get(id int) (Review, error) {
	stmt := `SELECT id, movie_id, user_id, rating,review_text, created_at, updated_at FROM reviews
        	 WHERE id = ?`
	row := m.DB.QueryRow(stmt, id)
	var r Review
	err := row.Scan(&r.ID , &r.MovieID, &r.UserID, &r.Rating, &r.Review, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNoRecord
		} else {
			return Review{}, err
		}
	}

	return r, nil
}
