package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/pkg/pipeline"
	"github.com/spf13/cobra"
)

var (
	runSiteID string
)

func init() {
	pipelineRunCmd.Flags().StringVar(&runSiteID, "site-id", "", "Site ID from config.comment_sites (required)")
	rootCmd.AddCommand(pipelineRunCmd)
}

var pipelineRunCmd = &cobra.Command{
	Use:   "pipeline-run",
	Short: "Run full pipeline (checkout → generate → hugo → commit → push)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("pipeline-run called")

		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()

		siteID = strings.TrimSpace(runSiteID)
		if siteID == "" {
			return fmt.Errorf("site_id is required (use --site-id)")
		}

		r := pipeline.Runner{
			DB:     database,
			SiteID: siteID,
		}

		runID, err := r.Run(context.Background(), "")
		if err != nil {
			return err
		}

		fmt.Printf("Pipeline finished (run_id=%d)\n", runID)
		return nil
	},
}
