package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/geschke/fyndmark/pkg/db"
	"github.com/geschke/fyndmark/pkg/users"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userDeleteCmd)
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userGrantCmd)
	userCmd.AddCommand(userRevokeCmd)
	userCmd.AddCommand(userSitesCmd)

	userCreateCmd.Flags().StringVar(&userCreateEmail, "email", "", "User email (required)")
	userCreateCmd.Flags().StringVar(&userCreateFirstName, "first-name", "", "First name (optional)")
	userCreateCmd.Flags().StringVar(&userCreateLastName, "last-name", "", "Last name (optional)")
	userCreateCmd.Flags().BoolVar(&userCreatePasswordStdin, "password-stdin", false, "Read password from stdin (recommended)")
	userCreateCmd.Flags().StringVar(&userCreatePassword, "password", "", "Password (NOT recommended; may leak via shell history)")

	userDeleteCmd.Flags().Int64Var(&userDeleteID, "id", 0, "User id")
	userDeleteCmd.Flags().StringVar(&userDeleteEmail, "email", "", "User email")

	userGrantCmd.Flags().Int64Var(&userGrantID, "id", 0, "User id")
	userGrantCmd.Flags().StringVar(&userGrantEmail, "email", "", "User email")
	userGrantCmd.Flags().StringVar(&userGrantSiteID, "site", "", "Site id")
	_ = userGrantCmd.MarkFlagRequired("site")

	userRevokeCmd.Flags().Int64Var(&userRevokeID, "id", 0, "User id")
	userRevokeCmd.Flags().StringVar(&userRevokeEmail, "email", "", "User email")
	userRevokeCmd.Flags().StringVar(&userRevokeSiteID, "site", "", "Site id")
	_ = userRevokeCmd.MarkFlagRequired("site")

	userSitesCmd.Flags().Int64Var(&userSitesID, "id", 0, "User id")
	userSitesCmd.Flags().StringVar(&userSitesEmail, "email", "", "User email")

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

var (
	userGrantID     int64
	userGrantEmail  string
	userGrantSiteID string
)

var userGrantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Grant a user access to a site",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		siteID := strings.TrimSpace(userGrantSiteID)
		if siteID == "" {
			return fmt.Errorf("--site is required")
		}
		exists, err := database.SiteExists(ctx, siteID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("site not found (site_id=%s)", siteID)
		}

		userID, err := resolveCLIUserID(ctx, database, userGrantID, userGrantEmail)
		if err != nil {
			return err
		}

		created, err := database.GrantUserSite(ctx, userID, siteID)
		if err != nil {
			return err
		}
		if created {
			fmt.Printf("Granted site (user_id=%d site_id=%s)\n", userID, siteID)
		} else {
			fmt.Printf("Already granted (user_id=%d site_id=%s)\n", userID, siteID)
		}
		return nil
	},
}

var (
	userRevokeID     int64
	userRevokeEmail  string
	userRevokeSiteID string
)

var userRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke a user's access to a site",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		siteID := strings.TrimSpace(userRevokeSiteID)
		if siteID == "" {
			return fmt.Errorf("--site is required")
		}
		exists, err := database.SiteExists(ctx, siteID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("site not found (site_id=%s)", siteID)
		}

		userID, err := resolveCLIUserID(ctx, database, userRevokeID, userRevokeEmail)
		if err != nil {
			return err
		}

		deleted, err := database.RevokeUserSite(ctx, userID, siteID)
		if err != nil {
			return err
		}
		if deleted {
			fmt.Printf("Revoked site (user_id=%d site_id=%s)\n", userID, siteID)
		} else {
			fmt.Printf("Not present (user_id=%d site_id=%s)\n", userID, siteID)
		}
		return nil
	},
}

var (
	userSitesID    int64
	userSitesEmail string
)

var userSitesCmd = &cobra.Command{
	Use:   "sites",
	Short: "List site assignments for a user",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		userID, err := resolveCLIUserID(ctx, database, userSitesID, userSitesEmail)
		if err != nil {
			return err
		}

		siteIDs, err := database.ListUserSites(ctx, userID)
		if err != nil {
			return err
		}
		if len(siteIDs) == 0 {
			fmt.Println("(no sites)")
			return nil
		}
		for _, siteID := range siteIDs {
			fmt.Printf("site_id=%s\n", siteID)
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

func resolveCLIUserID(ctx context.Context, database *db.DB, id int64, email string) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if (id > 0 && email != "") || (id <= 0 && email == "") {
		return 0, fmt.Errorf("provide exactly one of --id or --email")
	}

	if id > 0 {
		exists, err := database.UserExistsByID(ctx, id)
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, fmt.Errorf("user not found (id=%d)", id)
		}
		return id, nil
	}

	userID, found, err := database.GetUserIDByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, fmt.Errorf("user not found (email=%s)", email)
	}
	return userID, nil
}
