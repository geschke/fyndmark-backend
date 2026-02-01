package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/pkg/git"
	"github.com/spf13/cobra"
)

var (
	gitSiteID    string
	gitCommitMsg string
)

func init() {

	// Shared flag for all git subcommands
	gitCheckoutCmd.Flags().StringVar(&gitSiteID, "site-id", "", "Site ID from config.comment_sites (required)")
	gitCommitCmd.Flags().StringVar(&gitSiteID, "site-id", "", "Site ID from config.comment_sites (required)")
	gitPushCmd.Flags().StringVar(&gitSiteID, "site-id", "", "Site ID from config.comment_sites (required)")

	gitCommitCmd.Flags().StringVar(&gitCommitMsg, "message", "Update generated content", "Commit message")

	rootCmd.AddCommand(gitCheckoutCmd)
	rootCmd.AddCommand(gitCommitCmd)
	rootCmd.AddCommand(gitPushCmd)

}

var gitCheckoutCmd = &cobra.Command{
	Use:   "git-checkout",
	Short: "Clone the configured Hugo website repository for a comment site",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("git-checkout called")
		gitSiteID = strings.TrimSpace(gitSiteID)
		if gitSiteID == "" {
			return fmt.Errorf("site_id is required (use --site-id)")
		}

		r := git.GitRunner{
			SiteID: gitSiteID,
		}

		return r.Checkout(context.Background())
	},
}

var gitCommitCmd = &cobra.Command{
	Use:   "git-commit",
	Short: "Commit all changes in the checked out website repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("git-commit called")
		gitSiteID = strings.TrimSpace(gitSiteID)
		if gitSiteID == "" {
			return fmt.Errorf("site_id is required (use --site-id)")
		}

		r := git.GitRunner{
			SiteID: gitSiteID,
		}

		return r.Commit(context.Background(), gitCommitMsg)
	},
}

var gitPushCmd = &cobra.Command{
	Use:   "git-push",
	Short: "Push the current branch of the checked out website repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("git-push called")
		gitSiteID = strings.TrimSpace(gitSiteID)
		if gitSiteID == "" {
			return fmt.Errorf("site_id is required (use --site-id)")
		}

		r := git.GitRunner{
			SiteID: gitSiteID,
		}

		return r.Push(context.Background())
	},
}
