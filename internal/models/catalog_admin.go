package models

import (
	"database/sql"
	"errors"
)

// Update changes editorial fields only; imported IDs and review statistics remain intact.
func (m *MovieModel) Update(movie Movie) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockMovieForReview(tx, movie.ID); err != nil {
		return err
	}
	var releaseDate any
	if !movie.ReleaseDate.IsZero() {
		releaseDate = movie.ReleaseDate
	}
	_, err = tx.Exec(`UPDATE movies SET title = ?, description = ?, release_date = ?, poster_image = ?, backdrop_image = ? WHERE id = ?`, movie.Title, movie.Description, releaseDate, movie.PosterImage, movie.BackdropImage, movie.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a movie and its dependent records together, or rolls everything back.
func (m *MovieModel) Delete(id int) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockMovieForReview(tx, id); err != nil {
		return err
	}
	statements := []string{
		`DELETE rl FROM review_likes rl JOIN reviews r ON r.id = rl.review_id WHERE r.movie_id = ?`,
		`DELETE FROM reviews WHERE movie_id = ?`,
		`DELETE FROM movie_persons WHERE movie_id = ?`,
		`DELETE FROM movies WHERE id = ?`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func lockPerson(tx *sql.Tx, id int) error {
	var existing int
	err := tx.QueryRow(`SELECT id FROM persons WHERE id = ? FOR UPDATE`, id).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoRecord
	}
	return err
}

func (m *PersonModel) Update(person Person) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockPerson(tx, person.ID); err != nil {
		return err
	}
	var birthDate any
	if !person.BirthDate.IsZero() {
		birthDate = person.BirthDate
	}
	_, err = tx.Exec(`UPDATE persons SET name = ?, bio = ?, birth_date = ?, profile_image = ? WHERE id = ?`, person.Name, person.Bio, birthDate, person.ProfileImage, person.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Movies are retained when a person is removed from the local catalog.
func (m *PersonModel) Delete(id int) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockPerson(tx, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM movie_persons WHERE person_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM persons WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
