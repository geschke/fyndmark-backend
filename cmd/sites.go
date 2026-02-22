package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(sitesCmd)
	sitesCmd.AddCommand(sitesListCmd)
}

var sitesCmd = &cobra.Command{
	Use:   "sites",
	Short: "Manage sites",
}

var sitesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sites",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		list, err := database.ListSites(ctx)
		if err != nil {
			return err
		}

		if len(list) == 0 {
			fmt.Println("(no sites)")
			return nil
		}

		for _, s := range list {
			fmt.Printf(
				"id=%d site_key=%s title=%q status=%s created_at=%d updated_at=%d\n",
				s.ID,
				s.SiteKey,
				s.Title,
				s.Status,
				s.CreatedAt,
				s.UpdatedAt,
			)
		}

		return nil
	},
}
