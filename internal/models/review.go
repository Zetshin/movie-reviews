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

func (m *ReviewModel) GetRandom() (Review, error) {
	stmt := `SELECT id, movie_id, user_id, rating, review_text, created_at, updated_at
			 FROM reviews
			 WHERE rating > 3
			 ORDER BY RAND()
			 LIMIT 1;`

	row := m.DB.QueryRow(stmt)
	var r Review
	err := row.Scan(&r.ID, &r.MovieID, &r.UserID, &r.Rating, &r.Review, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Review{}, err
	}
	return r, nil
}

// This will insert a new snippet into the database.
func (m *ReviewModel) Insert(movieID, userID int, rating int, reviewText string) (int, error) {
	stmt := `INSERT INTO reviews (movie_id, user_id, rating, review_text)
	VALUES(?,?,?,?)`
	result, err := m.DB.Exec(stmt, movieID, userID, rating, reviewText)
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

func (m *ReviewModel) Delete(reviewID int) error {
	stmt := `DELETE FROM reviews WHERE id = ?`

	result, err := m.DB.Exec(stmt, reviewID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNoRecord
	}
	return nil
}


func (m *ReviewModel) Update(rating int, reviewText string ,reviewID int) error {
	stmt := `UPDATE reviews SET rating = ?, review_text = ?
			 WHERE id = ?;`

	result, err := m.DB.Exec(stmt, rating, reviewText, reviewID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNoRecord
	}
	return nil
}
