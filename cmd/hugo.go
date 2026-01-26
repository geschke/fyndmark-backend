package cmd

import (
	"fmt"

	"github.com/geschke/fyndmark/pkg/hugo"
	"github.com/spf13/cobra"
)

func init() {
	hugoRunCmd.Flags().StringVar(&hugoRunSiteId, "site-id", "", "Site ID from config.comment_sites (required)")
	rootCmd.AddCommand(hugoRunCmd)
}

var hugoRunSiteId string

var hugoRunCmd = &cobra.Command{
	Use:   "hugo-run",
	Short: "Run Hugo to generate the static site for a comment site",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("hugo-run called")
		return hugo.Run(hugoRunSiteId)
	},
}
