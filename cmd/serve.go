/*
Copyright © 2025 Ralf Geschke <ralf@kuerbis.org>
*/
package cmd

import (
	"github.com/geschke/fyndmark/pkg/server"
	"github.com/spf13/cobra"
)

// init configures package-level command and flag wiring.
func init() {
	rootCmd.AddCommand(serveCmd)

}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the fyndmark HTTP server.",

	Long: `Starts the fyndmark HTTP server using the configuration
provided via config file, environment variables, or CLI flags.

The server accepts comment submissions, stores them in SQLite,
and can generate markdown files inside each page bundle under
content/.../comments/ for a Git-based workflow.`,

	RunE: func(cmd *cobra.Command, args []string) error {
		database, cleanup, err := openDatabase()
		if err != nil {
			return err
		}
		defer cleanup()
		return server.Start(database)
	},
}
