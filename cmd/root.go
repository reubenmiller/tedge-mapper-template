/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// Build data
var buildVersion string
var buildBranch string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tedge-mapper-template",
	Short: "Generic tedge-mapper message translator",
	Long: `The generic tedge-mapper allows users to define template
files which control the transformation of messages from one topic to another.
	`,
	Version: fmt.Sprintf("%s (branch=%s)", buildVersion, buildBranch),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		debug, _ := cmd.Root().PersistentFlags().GetBool("debug")
		verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

		logLevel := slog.LevelWarn
		if debug {
			logLevel = slog.LevelDebug
		} else if verbose {
			logLevel = slog.LevelInfo
		}

		// set global logger with custom options
		slog.SetDefault(slog.New(tint.NewHandler(colorable.NewColorableStderr(), &tint.Options{
			Level:      logLevel,
			TimeFormat: time.RFC3339,
		})))
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd.VersionTemplate()
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().Bool("debug", false, "Debug logging")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose logging")
}
