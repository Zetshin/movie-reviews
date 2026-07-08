package models

import (
	"database/sql"
)

type MoviePerson struct {
	MovieID  int
	PersonID int
	Role     string
}
type CastMember struct {
	Name         string
	ProfileImage string
	Role         string
}
type FilmographyItem struct {
	Movie
	Role string
}
type MoviePersonModel struct {
	DB *sql.DB
}

func (m *MoviePersonModel) GetCastMembers(movieID int) ([]CastMember, error) {
	stmt := `SELECT persons.name, persons.profile_image, movie_person.role
	FROM persons
	JOIN movie_person 
	ON persons.id = movie_person.person_id
	WHERE movie_person.movie_id = ?`
	rows, err := m.DB.Query(stmt, movieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var persons []CastMember
	for rows.Next() {
		var p CastMember
		err = rows.Scan(&p.Name, &p.ProfileImage, &p.Role)
		if err != nil {
			return nil, err
		}
		persons = append(persons, p)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return persons, nil
}

func (m *MoviePersonModel) GetFilmography(personID int) ([]FilmographyItem, error) {
	stmt := `SELECT movies.*, movie_person.role
	From movies
	JOIN movie_person
	ON movies.id = movie_person.movie_id
	WHERE movie_person.person_id = ?`
	rows, err := m.DB.Query(stmt, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movies []FilmographyItem
	for rows.Next() { // ถ้ามีปัญหาเนื่องจาก pool connection หลุดมันจะออกจากลูปเราเลยต้องไปเช็คเผื่อว่ามันเป็นเพราะหลุดหรือว่า row หมดแล้วอีกทีผ่าน rows.Err()
		var s FilmographyItem
		err = rows.Scan(&s.ID, &s.Title, &s.Description, &s.ReleaseDate, &s.PosterImage, &s.ReviewCount, &s.AvgRating, &s.Role)
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
