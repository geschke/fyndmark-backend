package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/geschke/fyndmark/pkg/users"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userDeleteCmd)
	userCmd.AddCommand(userListCmd)

	userCreateCmd.Flags().StringVar(&userCreateEmail, "email", "", "User email (required)")
	userCreateCmd.Flags().StringVar(&userCreateFirstName, "first-name", "", "First name (optional)")
	userCreateCmd.Flags().StringVar(&userCreateLastName, "last-name", "", "Last name (optional)")
	userCreateCmd.Flags().BoolVar(&userCreatePasswordStdin, "password-stdin", false, "Read password from stdin (recommended)")
	userCreateCmd.Flags().StringVar(&userCreatePassword, "password", "", "Password (NOT recommended; may leak via shell history)")

	userDeleteCmd.Flags().Int64Var(&userDeleteID, "id", 0, "User id")
	userDeleteCmd.Flags().StringVar(&userDeleteEmail, "email", "", "User email")

	_ = userCreateCmd.MarkFlagRequired("email")
}

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
}

var (
	userCreateEmail         string
	userCreateFirstName     string
	userCreateLastName      string
	userCreatePassword      string
	userCreatePasswordStdin bool
)

var userCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a user (email + password)",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		email := strings.TrimSpace(userCreateEmail)
		if email == "" {
			return fmt.Errorf("--email is required")
		}

		pw, err := readPassword(cmd, userCreatePassword, userCreatePasswordStdin)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		id, err := users.Create(ctx, database, users.CreateParams{
			Email:     email,
			Password:  pw,
			FirstName: userCreateFirstName,
			LastName:  userCreateLastName,
		})
		if err != nil {
			return err
		}

		fmt.Printf("User created (id=%d email=%s)\n", id, strings.ToLower(email))
		return nil
	},
}

var (
	userDeleteID    int64
	userDeleteEmail string
)

var userDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a user by id or email",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if userDeleteID > 0 {
			deleted, err := users.DeleteByID(ctx, database, userDeleteID)
			if err != nil {
				return err
			}
			if !deleted {
				return fmt.Errorf("user not found (id=%d)", userDeleteID)
			}
			fmt.Printf("User deleted (id=%d)\n", userDeleteID)
			return nil
		}

		email := strings.TrimSpace(userDeleteEmail)
		if email == "" {
			return fmt.Errorf("provide either --id or --email")
		}

		deleted, err := users.DeleteByEmail(ctx, database, email)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("user not found (email=%s)", strings.ToLower(email))
		}
		fmt.Printf("User deleted (email=%s)\n", strings.ToLower(email))
		return nil
	},
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		list, err := users.List(ctx, database)
		if err != nil {
			return err
		}

		if len(list) == 0 {
			fmt.Println("(no users)")
			return nil
		}

		for _, u := range list {
			fmt.Printf("id=%d email=%s name=%s %s created_at=%d updated_at=%d\n",
				u.ID,
				u.Email,
				u.FirstName,
				u.LastName,
				u.DateCreated,
				u.DateUpdated,
			)
		}
		return nil
	},
}

func readPassword(cmd *cobra.Command, flagValue string, fromStdin bool) (string, error) {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue, nil
	}

	if !fromStdin {
		return "", errors.New("password is required (use --password-stdin or --password)")
	}

	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	pw := strings.TrimSpace(string(b))
	if pw == "" {
		return "", errors.New("password is empty")
	}
	return pw, nil
}
