package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/pkg/generator"
	"github.com/spf13/cobra"
)

var (
	siteID string
)

func init() {
	generateCommentsCmd.Flags().StringVar(&siteID, "site-id", "", "Site ID from config.comment_sites (required)")
	rootCmd.AddCommand(generateCommentsCmd)
}

var generateCommentsCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate markdown comment files into each page bundle (<bundle>/comments/*.md)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("generate-comments called")

		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		siteID = strings.TrimSpace(siteID)
		if siteID == "" {
			return fmt.Errorf("site_id is required (use --site-id)")
		}

		g := generator.Generator{
			DB:     database,
			SiteID: siteID,
		}

		return g.Generate(context.Background())
	},
}
