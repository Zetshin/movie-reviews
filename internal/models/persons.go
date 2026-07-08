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

func (m *PersonModel) Insert(name string, bio string, birthDate time.Time, profileImage string) (int, error) {
	stmt := `INSERT INTO person ( name, bio, birthDate, profileImage)
	VALUES (?,?,?,?,?)`
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
	stmt := `SELECT id, name, bio, birthDate, profileImage FROM persons
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
