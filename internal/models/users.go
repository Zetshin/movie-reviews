package models

import (
	"database/sql"
	"errors"  // New import
	"strings" // New import
	"time"

	"github.com/go-sql-driver/mysql" // New import
	"golang.org/x/crypto/bcrypt"     // New import
)

// Define a new User struct. Notice how the field names and types align
// with the columns in the database "users" table?
type User struct {
	ID             int
	Name           string
	Email          string
	HashedPassword []byte
	Created        time.Time
	IsAdmin        bool
	AvatarImage    string
}

// Define a new UserModel struct which wraps a database connection pool.
type UserModel struct {
	DB *sql.DB
}

// Get returns profile information without loading the user's password hash.
func (m *UserModel) Get(id int) (User, error) {
	stmt := `SELECT id, username, email, created, is_admin, COALESCE(avatar_image, '') FROM users WHERE id = ?`
	var user User
	err := m.DB.QueryRow(stmt, id).Scan(&user.ID, &user.Name, &user.Email, &user.Created, &user.IsAdmin, &user.AvatarImage)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNoRecord
		}
		return User{}, err
	}
	return user, nil
}

// We'll use the Insert method to add a new record to the "users" table.
func (m *UserModel) Insert(name, email, password string) error {
	// Create a bcrypt hash of the plain-text password.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	stmt := `INSERT INTO users (username, email, hashed_password, created)
    VALUES(?, ?, ?, UTC_TIMESTAMP())`
	// Use the Exec() method to insert the user details and hashed password
	// into the users table.
	_, err = m.DB.Exec(stmt, name, email, string(hashedPassword))
	if err != nil {
		// If there is an error, we use the errors.AsType() function to check
		// whether the error matches the type *mysql.MySQLError, and if it does,
		// assign it to the mySQLError variable.
		if mySQLError, ok := errors.AsType[*mysql.MySQLError](err); ok {
			// Error 1062 is a duplicate key. Match any unique key on the email
			// column: MySQL reports it as "users.email" or "users.users_uc_email"
			// depending on how the index was named.
			if mySQLError.Number == 1062 && strings.Contains(mySQLError.Message, "email") {
				return ErrDuplicateEmail
			}
		}
		// For all other errors, return them as normal.
		return err
	}
	return nil
}

// We'll use the Authenticate method to verify whether a user exists with
// the provided email address and password. This will return the relevant
// user ID if they do.
func (m *UserModel) Authenticate(email, password string) (int, error) {
	// Retrieve the id and hashed password associated with the given email. If
	// no matching email exists we return the ErrInvalidCredentials error.
	var id int
	var hashedPassword []byte
	stmt := "SELECT id, hashed_password FROM users WHERE email = ?"
	err := m.DB.QueryRow(stmt, email).Scan(&id, &hashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Spend the same bcrypt time as a real check so response timing
			// does not reveal which email addresses are registered.
			bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
			return 0, ErrInvalidCredentials
		} else {
			return 0, err
		}
	}
	// Check whether the hashed password and plain-text password provided match.
	// If they don't, we return the ErrInvalidCredentials error.
	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return 0, ErrInvalidCredentials
		} else {
			return 0, err
		}
	}
	// Otherwise, the password is correct. Return the user ID.
	return id, nil
}

// dummyPasswordHash is a real bcrypt hash with the same cost as stored
// passwords. It is used only to equalise login timing for unknown emails.
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equaliser-not-a-password"), 12)

// Exists reports whether a user with the given ID is still present.
func (m *UserModel) Exists(id int) (bool, error) {
	var exists bool
	err := m.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`, id).Scan(&exists)
	return exists, err
}

func (m *UserModel) IsAdmin(id int) (bool, error) {
	var isAdmin bool

	stmt := "SELECT is_admin FROM users WHERE id = ?"

	err := m.DB.QueryRow(stmt, id).Scan(&isAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNoRecord
		}
		return false, err
	}

	return isAdmin, nil
}

// SetAvatar returns the old path only after the new path has been committed.
func (m *UserModel) SetAvatar(userID int, path string) (string, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var old string
	err = tx.QueryRow(`SELECT COALESCE(avatar_image, '') FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoRecord
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`UPDATE users SET avatar_image = ? WHERE id = ?`, path, userID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return old, nil
}
