package models

import (
	"database/sql"
	"fmt"
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
	LikeCount int
}
type ReviewModel struct {
	DB *sql.DB
}

func (m *ReviewModel) ListByMovieID(
	movieID int,
	sort string,
	limit int,
	offset int,
) ([]Review, error) {
	var orderBy string

	switch sort {
	case "popular":
		orderBy = "ORDER BY like_count DESC, r.created_atผ DESC, r.id DESC"

	default:
		orderBy = "ORDER BY r.created_at DESC, r.id DESC"
	}

	stmt := fmt.Sprintf(`
        SELECT r.id, r.movie_id, r.user_id, r.rating, r.review_text, r.created_at, r.updated_at, 
		COUNT(rl.user_id) AS like_count
        FROM reviews AS r
        LEFT JOIN review_likes AS rl 
		ON r.id = rl.review_id
        WHERE r.movie_id = ?
        GROUP BY r.id, r.movie_id, r.user_id, r.rating, r.review_text,
                 r.created_at, r.updated_at
        %s
        LIMIT ? OFFSET ?`, orderBy)

	rows, err := m.DB.Query(stmt, movieID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reviews []Review
	for rows.Next() {
		var r Review
		err = rows.Scan(&r.ID, &r.MovieID, &r.UserID, &r.Rating, &r.Review, &r.CreatedAt, &r.UpdatedAt, &r.LikeCount)
		if err != nil {
			return nil, err
		}

		reviews = append(reviews, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return reviews, nil

}

func (m *ReviewModel) CountByMovieID(movieID int) (int, error) {
	stmt := `SELECT COUNT(*) FROM reviews WHERE movie_id = ?`
	var count int
	err := m.DB.QueryRow(stmt, movieID).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
