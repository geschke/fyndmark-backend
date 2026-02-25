package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/pkg/generator"
	"github.com/spf13/cobra"
)

var (
	siteKey string
)

// init configures package-level command and flag wiring.
func init() {
	generateCommentsCmd.Flags().StringVar(&siteKey, "site-key", "", "Site Key from config.comment_sites (required)")
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

		siteKey = strings.TrimSpace(siteKey)
		if siteKey == "" {
			return fmt.Errorf("site-key is required (use --site-key)")
		}

		g := generator.Generator{
			DB:      database,
			SiteKey: siteKey,
		}

		return g.Generate(context.Background())
	},
}
