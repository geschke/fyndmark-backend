package users

import (
	"context"
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/pkg/db"
)

type CreateParams struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
}

func Create(ctx context.Context, database *db.DB, p CreateParams) (int64, error) {
	if database == nil {
		return 0, fmt.Errorf("db is nil")
	}

	email := strings.ToLower(strings.TrimSpace(p.Email))
	if email == "" {
		return 0, fmt.Errorf("email is required")
	}

	pwHash, err := HashPassword(p.Password, DefaultArgon2idParams)
	if err != nil {
		return 0, err
	}

	id, err := database.CreateUser(ctx, db.User{
		Email:     email,
		Password:  pwHash,
		FirstName: p.FirstName,
		LastName:  p.LastName,
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func DeleteByID(ctx context.Context, database *db.DB, id int64) (bool, error) {
	if database == nil {
		return false, fmt.Errorf("db is nil")
	}
	return database.DeleteUser(ctx, id)
}

func DeleteByEmail(ctx context.Context, database *db.DB, email string) (bool, error) {
	if database == nil {
		return false, fmt.Errorf("db is nil")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, fmt.Errorf("email is required")
	}

	u, found, err := database.GetUserByEmail(ctx, email)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return database.DeleteUser(ctx, u.ID)
}

func List(ctx context.Context, database *db.DB) ([]db.User, error) {
	if database == nil {
		return nil, fmt.Errorf("db is nil")
	}
	return database.ListUsers(ctx)
}
