package models

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Person struct {
	ID           int
	TMDBID       int
	Name         string
	Bio          string
	BirthDate    time.Time
	ProfileImage string
}

type PersonModel struct {
	DB *sql.DB
}

// Latest returns up to limit persons ordered by descending ID.
func (m *PersonModel) Latest(limit int) ([]Person, error) {
	stmt := `SELECT id, name, COALESCE(profile_image, '') FROM persons
	ORDER BY id DESC LIMIT ?`
	rows, err := m.DB.Query(stmt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var persons []Person
	for rows.Next() {
		var person Person
		if err := rows.Scan(&person.ID, &person.Name, &person.ProfileImage); err != nil {
			return nil, err
		}
		persons = append(persons, person)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return persons, nil
}

// LatestWithImages selects people for Home without changing the general Latest method.
func (m *PersonModel) LatestWithImages(limit int) ([]Person, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("person image limit must be between 1 and 100")
	}
	rows, err := m.DB.Query(`SELECT id, name, profile_image FROM persons
		WHERE profile_image IS NOT NULL AND TRIM(profile_image) <> ''
		ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var persons []Person
	for rows.Next() {
		var person Person
		if err := rows.Scan(&person.ID, &person.Name, &person.ProfileImage); err != nil {
			return nil, err
		}
		persons = append(persons, person)
	}
	return persons, rows.Err()
}

func (m *PersonModel) Insert(name string, bio string, birthDate time.Time, profileImage string) (int, error) {
	stmt := `INSERT INTO persons ( name, bio, birth_date, profile_image)
	VALUES (?,?,?,?)`
	var date any
	if !birthDate.IsZero() {
		date = birthDate
	}
	result, err := m.DB.Exec(stmt, name, bio, date, profileImage)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// Reuse the TMDB ID and fill missing details without replacing administrator edits.
func (m *PersonModel) ImportTMDB(person Person) (int, error) {
	if person.TMDBID < 1 || strings.TrimSpace(person.Name) == "" {
		return 0, errors.New("TMDB person requires an ID and name")
	}
	var birthday any
	if !person.BirthDate.IsZero() {
		birthday = person.BirthDate
	}
	result, err := m.DB.Exec(`INSERT INTO persons (tmdb_id, name, bio, birth_date, profile_image)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id),
		bio = IF(COALESCE(TRIM(bio), '') = '', ?, bio),
		birth_date = COALESCE(birth_date, ?),
		profile_image = IF(COALESCE(TRIM(profile_image), '') = '', ?, profile_image)`,
		person.TMDBID, person.Name, person.Bio, birthday, person.ProfileImage, person.Bio, birthday, person.ProfileImage)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (m *PersonModel) ImportedTMDBIDs(ids []int) (map[int]bool, error) {
	imported := make(map[int]bool)
	if len(ids) == 0 {
		return imported, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := m.DB.Query(`SELECT tmdb_id FROM persons WHERE tmdb_id IN (`+placeholders+`)`, args...)
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

func (m *PersonModel) Get(id int) (Person, error) {
	stmt := `SELECT id, name, COALESCE(bio, ''), birth_date, COALESCE(profile_image, '') FROM persons
	WHERE id = ?`
	row := m.DB.QueryRow(stmt, id)
	var p Person
	var birthDate sql.NullTime
	err := row.Scan(&p.ID, &p.Name, &p.Bio, &birthDate, &p.ProfileImage)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Person{}, ErrNoRecord
		} else {
			return Person{}, err
		}
	}
	if birthDate.Valid {
		p.BirthDate = birthDate.Time
	}
	return p, nil
}

// Search returns people matching a name for the search dropdown.
func (m *PersonModel) Search(query string, limit int) ([]Person, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("search limit must be between 1 and 100")
	}
	rows, err := m.DB.Query(`SELECT id, name, COALESCE(profile_image, '') FROM persons WHERE name LIKE ? ORDER BY name ASC, id DESC LIMIT ?`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	persons := make([]Person, 0)
	for rows.Next() {
		var person Person
		if err := rows.Scan(&person.ID, &person.Name, &person.ProfileImage); err != nil {
			return nil, err
		}
		persons = append(persons, person)
	}
	return persons, rows.Err()
}

// TMDBIDs lists only imported people, leaving manually created people untouched.
func (m *PersonModel) TMDBIDs() ([]int, error) {
	rows, err := m.DB.Query(`SELECT tmdb_id FROM persons WHERE tmdb_id IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FillTMDBDetails fills missing fields without overwriting existing edits.
func (m *PersonModel) FillTMDBDetails(tmdbID int, bio string, birthday *time.Time, profileImage string) error {
	var birthDate any
	if birthday != nil {
		birthDate = *birthday
	}
	_, err := m.DB.Exec(`UPDATE persons SET
		bio = IF(COALESCE(TRIM(bio), '') = '', ?, bio),
		birth_date = COALESCE(birth_date, ?),
		profile_image = IF(COALESCE(TRIM(profile_image), '') = '', ?, profile_image)
		WHERE tmdb_id = ?`, bio, birthDate, profileImage, tmdbID)
	return err
}

// SetTrending atomically replaces the selected ordering, preserving other people.
func (m *PersonModel) SetTrending(people []Person) error {
	if len(people) == 0 {
		return errors.New("trending people cannot be empty")
	}
	for _, p := range people {
		if p.TMDBID < 1 || p.Name == "" || p.ProfileImage == "" {
			return errors.New("trending person must have an ID, name and image")
		}
	}
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE persons SET trending_rank = NULL WHERE trending_rank IS NOT NULL`); err != nil {
		return err
	}
	for i, p := range people {
		var birthday any
		if !p.BirthDate.IsZero() {
			birthday = p.BirthDate
		}
		_, err := tx.Exec(`INSERT INTO persons (tmdb_id, name, bio, birth_date, profile_image, trending_rank)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE trending_rank = ?,
			bio = IF(COALESCE(TRIM(bio), '') = '', ?, bio),
			birth_date = COALESCE(birth_date, ?),
			profile_image = IF(COALESCE(TRIM(profile_image), '') = '', ?, profile_image)`,
			p.TMDBID, p.Name, p.Bio, birthday, p.ProfileImage, i+1,
			i+1, p.Bio, birthday, p.ProfileImage)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *PersonModel) Trending(limit int) ([]Person, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("trending people limit must be between 1 and 100")
	}
	rows, err := m.DB.Query(`SELECT id, name, profile_image FROM persons
		WHERE trending_rank IS NOT NULL AND COALESCE(profile_image, '') <> ''
		ORDER BY trending_rank ASC, id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var people []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.ProfileImage); err != nil {
			return nil, err
		}
		people = append(people, p)
	}
	return people, rows.Err()
}

// All returns every locally stored person in name order for the people catalog.
func (m *PersonModel) All() ([]Person, error) {
	rows, err := m.DB.Query(`SELECT id, name, COALESCE(profile_image, '') FROM persons ORDER BY name ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	people := make([]Person, 0)
	for rows.Next() {
		var person Person
		if err := rows.Scan(&person.ID, &person.Name, &person.ProfileImage); err != nil {
			return nil, err
		}
		people = append(people, person)
	}
	return people, rows.Err()
}
