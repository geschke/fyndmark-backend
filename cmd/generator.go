package cmd

import (
	"context"
	"fmt"

	"github.com/geschke/fyndmark/pkg/generatorcli"
	"github.com/spf13/cobra"
)

var (
	genSiteID string
)

func init() {
	generateCommentsCmd.Flags().StringVar(&genSiteID, "site-id", "", "Site ID from config.comment_sites (required)")
	rootCmd.AddCommand(generateCommentsCmd)
}

var generateCommentsCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate markdown comment files into each page bundle (<bundle>/comments/*.md)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("generate-comments called")
		return generatorcli.Generate(context.Background(), genSiteID)
	},
}
