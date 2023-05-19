/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reubenmiller/tedge-mapper-template/pkg/service"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

var ArgMaxDepth int64 = 3
var ArgBroker string
var ArgClientID string
var ArgDelay time.Duration
var ArgCleanSession bool

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the mapper and subscribe to the MQTT broker",
	Long: `Run the translator which transforms MQTT messages on a matching topics to
new messages.

Examples:

	tedge-mapper-template serve --dir routes
	# Start the mapper and use any routes defined under the ./routes directory

	tedge-mapper-template serve --host 'otherhost:1883'
	# Start the mapper using a custom MQTT broker endpoint
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Info("Starting listener")
		debug, _ := cmd.Root().PersistentFlags().GetBool("debug")
		routeDir, _ := cmd.Root().PersistentFlags().GetString("dir")

		app, err := service.NewDefaultService(ArgBroker, ArgClientID, ArgCleanSession, routeDir, ArgMaxDepth, ArgDelay, debug)
		if err != nil {
			return err
		}

		if err := app.StartSubscriptions(); err != nil {
			return err
		}

		// Wait for termination signal
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop

		slog.Info("Shutting down...")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&ArgBroker, "host", "localhost:1883", "Broker endpoint (can included port number)")
	serveCmd.Flags().BoolVar(&ArgCleanSession, "clean", true, "Clean session")
	serveCmd.Flags().StringVarP(&ArgClientID, "clientid", "i", "tedge-mapper-template", "MQTT client id")
	serveCmd.Flags().Int64Var(&ArgMaxDepth, "max-depth", 3, "Maximum recursion depth")
	serveCmd.Flags().DurationVar(&ArgDelay, "delay", 2*time.Second, "Delay to wait after publishing a message (to prevent spamming)")
}
