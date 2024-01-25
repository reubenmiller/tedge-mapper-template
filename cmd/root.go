/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/spf13/cobra"
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
		silent, _ := cmd.Root().PersistentFlags().GetBool("silent")
		loglevel, _ := cmd.Root().PersistentFlags().GetString("loglevel")
		showTimestamps, _ := cmd.Root().PersistentFlags().GetBool("timestamps")

		logLevel := GetLogLevel(loglevel)
		if debug {
			logLevel = slog.LevelDebug
		} else if silent {
			logLevel = slog.LevelWarn
		}

		// set global logger with custom options
		logfmt := time.RFC3339
		if !showTimestamps {
			logfmt = " "
		}
		slog.SetDefault(slog.New(tint.NewHandler(colorable.NewColorableStderr(), &tint.Options{
			Level:      logLevel,
			TimeFormat: logfmt,
		})))
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// By default run the serve command
		return serveCmd.RunE(cmd, args)
	},
}

func GetLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "info", "information":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "debug":
		return slog.LevelDebug
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
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
	rootCmd.PersistentFlags().Bool("debug", false, "Enable template debugging")
	rootCmd.PersistentFlags().BoolP("silent", "s", false, "Silent mode. Only log warnings and errors (shortcut for --loglevel=warn)")
	rootCmd.PersistentFlags().String("loglevel", "info", "Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().Bool("timestamps", true, "Show date/time in log entries")
	rootCmd.PersistentFlags().StringSlice("dir", []string{"routes"}, "Route directory (more than 1 can be provided)")
	rootCmd.PersistentFlags().StringSlice("libdir", []string{"lib"}, "Library directory (only used by jsonnet)")
	rootCmd.PersistentFlags().Int("maxdepth", 10, "Maximum recursion depth")
	rootCmd.PersistentFlags().Duration("delay", 2*time.Second, "Delay to wait after publishing a message (by the same route) (to prevent spamming)")
	rootCmd.PersistentFlags().Bool("dry", false, "Dry run mode. Don't send any requests")
	rootCmd.PersistentFlags().String("device-id", "", "Default device.id to use if the tedge configuration is not provided")
}
