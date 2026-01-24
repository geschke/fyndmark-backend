package cmd

import (
	"fmt"

	"github.com/geschke/fyndmark/pkg/git"
	"github.com/spf13/cobra"
)

func init() {
	gitCheckoutCmd.Flags().StringVar(&gitCheckoutSiteID, "site-id", "", "Site ID from config.comment_sites (required)")
	rootCmd.AddCommand(gitCheckoutCmd)
}

var gitCheckoutSiteID string

var gitCheckoutCmd = &cobra.Command{
	Use:   "git-checkout",
	Short: "Clone the configured Hugo website repository for a comment site",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("git-checkout called")
		return git.Checkout(gitCheckoutSiteID)
	},
}
