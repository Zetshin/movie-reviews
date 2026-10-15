package models

import (
	"database/sql"
	"errors"
)

type MoviePerson struct {
	MovieID  int
	PersonID int
	Role     string
}
type CastMember struct {
	ID            int
	Name          string
	ProfileImage  string
	Role          string
	CharacterName string
}

// Order belongs to this movie credit, not to the person globally.
type ImportedCastMember struct {
	Person
	Order         int
	CharacterName string
}
type FilmographyItem struct {
	Movie
	Role string
}
type MoviePersonModel struct {
	DB *sql.DB
}

type CastImportTask struct {
	PersonID     int    `json:"person_id"`
	TMDBID       int    `json:"tmdb_id"`
	Name         string `json:"name"`
	NeedsDetails bool   `json:"needs_details"`
}

const castImportQuery = `SELECT p.id, p.tmdb_id, p.name,
	(COALESCE(TRIM(p.bio), '') = '' OR p.birth_date IS NULL OR COALESCE(TRIM(p.profile_image), '') = '')
	FROM persons p JOIN movie_persons mp ON mp.person_id = p.id
	WHERE mp.movie_id = ? AND p.tmdb_id IS NOT NULL`

func (m *MoviePersonModel) ImportTasks(movieID int) ([]CastImportTask, error) {
	rows, err := m.DB.Query(castImportQuery+` ORDER BY mp.cast_order IS NULL, mp.cast_order, p.id`, movieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]CastImportTask, 0)
	for rows.Next() {
		var task CastImportTask
		if err := rows.Scan(&task.PersonID, &task.TMDBID, &task.Name, &task.NeedsDetails); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// Check membership server-side rather than trusting a submitted TMDB person ID.
func (m *MoviePersonModel) ImportTask(movieID, personID int) (CastImportTask, error) {
	var task CastImportTask
	err := m.DB.QueryRow(castImportQuery+` AND p.id = ?`, movieID, personID).Scan(&task.PersonID, &task.TMDBID, &task.Name, &task.NeedsDetails)
	if errors.Is(err, sql.ErrNoRows) {
		return CastImportTask{}, ErrNoRecord
	}
	return task, err
}

func (m *MoviePersonModel) GetCastMembers(movieID int) ([]CastMember, error) {
	stmt := `SELECT persons.id, persons.name, COALESCE(persons.profile_image, ''), COALESCE(movie_persons.role, ''), COALESCE(movie_persons.character_name, '')
	FROM persons
	JOIN movie_persons
	ON persons.id = movie_persons.person_id
	WHERE movie_persons.movie_id = ?
	ORDER BY movie_persons.cast_order IS NULL, movie_persons.cast_order ASC, persons.id ASC`
	rows, err := m.DB.Query(stmt, movieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var persons []CastMember
	for rows.Next() {
		var p CastMember
		err = rows.Scan(&p.ID, &p.Name, &p.ProfileImage, &p.Role, &p.CharacterName)
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
	stmt := `SELECT movies.id, movies.title, movies.description, movies.release_date, COALESCE(movies.poster_image, ''), movies.review_count, movies.avg_rating,movie_persons.role
	FROM movies
	JOIN movie_persons
	ON movies.id = movie_persons.movie_id
	WHERE movie_persons.person_id = ?`
	rows, err := m.DB.Query(stmt, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movies []FilmographyItem
	for rows.Next() { // ถ้ามีปัญหาเนื่องจาก pool connection หลุดมันจะออกจากลูปเราเลยต้องไปเช็คเผื่อว่ามันเป็นเพราะหลุดหรือว่า row หมดแล้วอีกทีผ่าน rows.Err()
		var s FilmographyItem
		var releaseDate sql.NullTime
		err = rows.Scan(&s.ID, &s.Title, &s.Description, &releaseDate, &s.PosterImage, &s.ReviewCount, &s.AvgRating, &s.Role)
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

func (m *MoviePersonModel) Insert(movieID int, personID int, role string) error {
	stmt := `INSERT INTO movie_persons (movie_id, person_id, role)
	VALUES (?,?,?)`
	_, err := m.DB.Exec(stmt, movieID, personID, role)
	if err != nil {
		return err
	}
	return nil
}
