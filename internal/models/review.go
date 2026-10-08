package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Review struct {
	ID        int
	MovieID   int
	UserID    int
	Username  string
	Rating    float64
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
		orderBy = "ORDER BY like_count DESC, r.created_at DESC, r.id DESC"

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

func (m *ReviewModel) GetRandom(movieID int) (Review, error) {
	stmt := `SELECT r.id, r.movie_id, r.user_id, r.rating,
                r.review_text, r.created_at, r.updated_at,
                u.username
         FROM reviews AS r
         JOIN users AS u ON r.user_id = u.id
         WHERE r.movie_id = ?
         AND   r.rating >= 3
         ORDER BY RAND()
         LIMIT 1`

	row := m.DB.QueryRow(stmt, movieID)
	var r Review
	err := row.Scan(&r.ID, &r.MovieID, &r.UserID, &r.Rating, &r.Review, &r.CreatedAt, &r.UpdatedAt, &r.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNoRecord
		}
		return Review{}, err
	}
	return r, nil
}

// Insert saves the review and refreshes its movie's statistics atomically.
func (m *ReviewModel) Insert(movieID, userID int, rating float64, reviewText string) (int, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := lockMovieForReview(tx, movieID); err != nil {
		return 0, err
	}

	stmt := `INSERT INTO reviews (movie_id, user_id, rating, review_text)
	VALUES(?,?,?,?)`
	result, err := tx.Exec(stmt, movieID, userID, rating, reviewText)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := updateReviewStats(tx, movieID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(id), nil

}

func (m *ReviewModel) Delete(reviewID int) error {
	tx, movieID, err := m.beginReviewChange(reviewID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt := `DELETE FROM reviews WHERE id = ?`

	_, err = tx.Exec(stmt, reviewID)
	if err != nil {
		return err
	}
	if err := updateReviewStats(tx, movieID); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *ReviewModel) Update(rating float64, reviewText string, reviewID int) error {
	tx, movieID, err := m.beginReviewChange(reviewID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt := `UPDATE reviews SET rating = ?, review_text = ?
			 WHERE id = ?;`

	_, err = tx.Exec(stmt, rating, reviewText, reviewID)
	if err != nil {
		return err
	}
	if err := updateReviewStats(tx, movieID); err != nil {
		return err
	}
	return tx.Commit()
}

// Lock the movie first, then confirm and lock the review. All review writes
// use this order so concurrent changes for the same movie cannot race.
func (m *ReviewModel) beginReviewChange(reviewID int) (*sql.Tx, int, error) {
	var movieID int
	err := m.DB.QueryRow(`SELECT movie_id FROM reviews WHERE id = ?`, reviewID).Scan(&movieID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNoRecord
	}
	if err != nil {
		return nil, 0, err
	}
	tx, err := m.DB.Begin()
	if err != nil {
		return nil, 0, err
	}
	if err := lockMovieForReview(tx, movieID); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	var id int
	err = tx.QueryRow(`SELECT id FROM reviews WHERE id = ? AND movie_id = ? FOR UPDATE`, reviewID, movieID).Scan(&id)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrNoRecord
		}
		return nil, 0, err
	}
	return tx, movieID, nil
}
