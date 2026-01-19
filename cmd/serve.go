/*
Copyright © 2025 Ralf Geschke <ralf@kuerbis.org>
*/
package cmd

import (
	"fmt"

	"github.com/geschke/fyndmark/pkg/server"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)

}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the fyndmark HTTP server.",

	Long: `Starts the fyndmark form-to-email HTTP server using the
configuration provided via config file, environment variables,
or CLI flags.

The server exposes the endpoint:
  POST /api/feedbackmail/:formid

Each form is defined in the config and can specify its own
fields, recipients, CORS settings, and optional Turnstile validation.`,

	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("serve called")
		return server.Start()
	},
}
