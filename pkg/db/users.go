package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID          int64  `json:"ID"`
	Password    string `json:"Password,omitempty"`
	FirstName   string `json:"FirstName,omitempty"`
	LastName    string `json:"LastName,omitempty"`
	Email       string `json:"Email,omitempty"`
	DateCreated int64  `json:"DateCreated,omitempty"`
	DateUpdated int64  `json:"DateUpdated,omitempty"`
}

func normalizeUser(u User) (User, error) {
	u.Password = strings.TrimSpace(u.Password)
	u.FirstName = strings.TrimSpace(u.FirstName)
	u.LastName = strings.TrimSpace(u.LastName)
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))

	if u.Email == "" {
		return User{}, fmt.Errorf("email is required")
	}
	if u.Password == "" {
		// Store hashed password. Leave hashing to the caller/controller/service.
		return User{}, fmt.Errorf("password is required")
	}

	now := time.Now().Unix()
	if u.DateCreated == 0 {
		u.DateCreated = now
	}
	u.DateUpdated = now

	return u, nil
}

func (d *DB) CreateUser(ctx context.Context, u User) (int64, error) {
	if d == nil || d.SQL == nil {
		return 0, fmt.Errorf("db not initialized")
	}

	u, err := normalizeUser(u)
	if err != nil {
		return 0, err
	}

	res, err := d.SQL.ExecContext(ctx, `
INSERT INTO users (
  password, firstname, lastname, email, date_created, date_updated
) VALUES (?, ?, ?, ?, ?, ?);
`, u.Password, u.FirstName, u.LastName, u.Email, u.DateCreated, u.DateUpdated)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create user last_insert_id: %w", err)
	}
	return id, nil
}

func scanUser(row *sql.Row) (User, error) {
	var u User
	if err := row.Scan(
		&u.ID,
		&u.Password,
		&u.FirstName,
		&u.LastName,
		&u.Email,
		&u.DateCreated,
		&u.DateUpdated,
	); err != nil {
		return User{}, err
	}
	return u, nil
}

func (d *DB) GetUserByID(ctx context.Context, id int64) (User, bool, error) {
	if d == nil || d.SQL == nil {
		return User{}, false, fmt.Errorf("db not initialized")
	}
	if id <= 0 {
		return User{}, false, fmt.Errorf("id must be > 0")
	}

	row := d.SQL.QueryRowContext(ctx, `
SELECT id, password, firstname, lastname, email, date_created, date_updated
  FROM users
 WHERE id = ?
 LIMIT 1;
`, id)

	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get user by id: %w", err)
	}

	return u, true, nil
}

func (d *DB) GetUserByEmail(ctx context.Context, email string) (User, bool, error) {
	if d == nil || d.SQL == nil {
		return User{}, false, fmt.Errorf("db not initialized")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return User{}, false, fmt.Errorf("email is required")
	}

	row := d.SQL.QueryRowContext(ctx, `
SELECT id, password, firstname, lastname, email, date_created, date_updated
  FROM users
 WHERE email = ?
 LIMIT 1;
`, email)

	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get user by email: %w", err)
	}

	return u, true, nil
}

// UpdateUser updates the user record by ID and always sets date_updated to now.
// Returns true if a row was updated, false if the user was not found.
func (d *DB) UpdateUser(ctx context.Context, u User) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if u.ID <= 0 {
		return false, fmt.Errorf("id must be > 0")
	}

	u.Password = strings.TrimSpace(u.Password)
	u.FirstName = strings.TrimSpace(u.FirstName)
	u.LastName = strings.TrimSpace(u.LastName)
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))

	now := time.Now().Unix()

	// Always update firstname/lastname (empty is allowed) and date_updated.
	// Update email/password only when explicitly provided (non-empty).
	setParts := []string{
		"firstname = ?",
		"lastname = ?",
		"date_updated = ?",
	}
	args := []any{u.FirstName, u.LastName, now}

	if u.Email != "" {
		setParts = append(setParts, "email = ?")
		args = append(args, u.Email)
	}
	if u.Password != "" {
		setParts = append(setParts, "password = ?")
		args = append(args, u.Password)
	}

	args = append(args, u.ID)

	query := fmt.Sprintf(`
UPDATE users
   SET %s
 WHERE id = ?;
`, strings.Join(setParts, ",\n       "))

	res, err := d.SQL.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("update user: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update user rows affected: %w", err)
	}
	if affected > 0 {
		return true, nil
	}

	// If the update didn't change anything (e.g. called twice in the same second),
	// treat it as success if the user exists.
	var one int
	err = d.SQL.QueryRowContext(ctx, `SELECT 1 FROM users WHERE id = ? LIMIT 1;`, u.ID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("update user existence check: %w", err)
	}
	return true, nil
}

// DeleteUser deletes the user record by ID.
// Returns true if a row was deleted, false if the user was not found.
func (d *DB) DeleteUser(ctx context.Context, id int64) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if id <= 0 {
		return false, fmt.Errorf("id must be > 0")
	}

	res, err := d.SQL.ExecContext(ctx, `DELETE FROM users WHERE id = ?;`, id)
	if err != nil {
		return false, fmt.Errorf("delete user: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete user rows affected: %w", err)
	}
	return affected > 0, nil
}

func (d *DB) ListUsers(ctx context.Context) ([]User, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, firstname, lastname, email, date_created, date_updated
  FROM users
 ORDER BY id ASC;
`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID,
			&u.FirstName,
			&u.LastName,
			&u.Email,
			&u.DateCreated,
			&u.DateUpdated,
		); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return out, nil
}

func (d *DB) GetUserIDByEmail(ctx context.Context, email string) (int64, bool, error) {
	if d == nil || d.SQL == nil {
		return 0, false, fmt.Errorf("db not initialized")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, false, fmt.Errorf("email is required")
	}

	var id int64
	err := d.SQL.QueryRowContext(ctx, `
SELECT id
  FROM users
 WHERE email = ?
 LIMIT 1;
`, email).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get user id by email: %w", err)
	}
	return id, true, nil
}

func (d *DB) UserExistsByID(ctx context.Context, userID int64) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return false, fmt.Errorf("userID must be > 0")
	}

	var one int
	err := d.SQL.QueryRowContext(ctx, `
SELECT 1
  FROM users
 WHERE id = ?
 LIMIT 1;
`, userID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("user exists by id query: %w", err)
	}
	return true, nil
}

func (d *DB) GrantUserSite(ctx context.Context, userID int64, siteID string) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return false, fmt.Errorf("userID must be > 0")
	}
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return false, fmt.Errorf("siteID is required")
	}

	res, err := d.SQL.ExecContext(ctx, `
INSERT INTO user_sites (user_id, site_id)
VALUES (?, ?)
ON CONFLICT(user_id, site_id) DO NOTHING;
`, userID, siteID)
	if err != nil {
		return false, fmt.Errorf("grant user site: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("grant user site rows affected: %w", err)
	}
	return affected > 0, nil
}

func (d *DB) RevokeUserSite(ctx context.Context, userID int64, siteID string) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return false, fmt.Errorf("userID must be > 0")
	}
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return false, fmt.Errorf("siteID is required")
	}

	res, err := d.SQL.ExecContext(ctx, `
DELETE FROM user_sites
 WHERE user_id = ?
   AND site_id = ?;
`, userID, siteID)
	if err != nil {
		return false, fmt.Errorf("revoke user site: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("revoke user site rows affected: %w", err)
	}
	return affected > 0, nil
}

func (d *DB) ListUserSites(ctx context.Context, userID int64) ([]string, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("userID must be > 0")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT site_id
  FROM user_sites
 WHERE user_id = ?
 ORDER BY site_id ASC;
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user sites: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]string, 0)
	for rows.Next() {
		var siteID string
		if err := rows.Scan(&siteID); err != nil {
			return nil, fmt.Errorf("scan user site: %w", err)
		}
		out = append(out, siteID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user sites: %w", err)
	}
	return out, nil
}
