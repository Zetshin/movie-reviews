package models

import (
	"database/sql"
	"errors"
	"time"
)

type Person struct {
	ID           int
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

func (m *PersonModel) Insert(name string, bio string, birthDate time.Time, profileImage string) (int, error) {
	stmt := `INSERT INTO persons ( name, bio, birth_date, profile_image)
	VALUES (?,?,?,?)`
	result, err := m.DB.Exec(stmt, name, bio, birthDate, profileImage)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (m *PersonModel) Get(id int) (Person, error) {
	stmt := `SELECT id, name, bio, birth_date, COALESCE(profile_image, '') FROM persons
	WHERE id = ?`
	row := m.DB.QueryRow(stmt, id)
	var p Person
	err := row.Scan(&p.ID, &p.Name, &p.Bio, &p.BirthDate, &p.ProfileImage)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Person{}, ErrNoRecord
		} else {
			return Person{}, err
		}
	}
	return p, nil
}
