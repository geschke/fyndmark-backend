package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/pkg/pipeline"
	"github.com/spf13/cobra"
)

var (
	runSiteKey string
)

func init() {
	pipelineRunCmd.Flags().StringVar(&runSiteKey, "site-key", "", "Site Key from config.comment_sites (required)")
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

		siteKey = strings.TrimSpace(runSiteKey)
		if siteKey == "" {
			return fmt.Errorf("site key is required (use --site-key)")
		}

		r := pipeline.Runner{
			DB:      database,
			SiteKey: siteKey,
		}

		runID, err := r.Run(context.Background(), "")
		if err != nil {
			return err
		}

		fmt.Printf("Pipeline finished (run_id=%d)\n", runID)
		return nil
	},
}
