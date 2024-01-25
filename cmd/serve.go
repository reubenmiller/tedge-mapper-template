/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mattn/go-isatty"
	"github.com/reubenmiller/tedge-mapper-template/pkg/service"
	"github.com/spf13/cobra"
)

var ArgBroker string
var ArgHTTPEndpoint string
var ArgClientID string
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

	tedge-mapper-template serve --dir routes --dir routes-simulation
	# Start the mapper and look for routes in multiple directories

	tedge-mapper-template serve --host 'otherhost:1883'
	# Start the mapper using a custom MQTT broker endpoint
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Info("Starting listener")
		debug, _ := cmd.Root().PersistentFlags().GetBool("debug")
		routeDirs, _ := cmd.Root().PersistentFlags().GetStringSlice("dir")
		libPaths, _ := cmd.Root().PersistentFlags().GetStringSlice("libdir")
		maxDepth, _ := cmd.Root().PersistentFlags().GetInt("maxdepth")
		delay, _ := cmd.Root().PersistentFlags().GetDuration("delay")
		dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry")
		deviceID, _ := cmd.Root().PersistentFlags().GetString("device-id")
		entityFile, _ := cmd.Flags().GetString("entityfile")

		useColor := true
		if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			useColor = false
		}

		app, err := service.NewDefaultService(
			&service.DefaultServiceOptions{
				Broker:                     ArgBroker,
				ClientID:                   ArgClientID,
				CleanSession:               ArgCleanSession,
				HTTPEndpoint:               ArgHTTPEndpoint,
				RouteDirs:                  routeDirs,
				MaxRouteDepth:              maxDepth,
				PostMessageDelay:           delay,
				Debug:                      debug,
				DryRun:                     dryRun,
				LibraryPaths:               libPaths,
				UseColor:                   useColor,
				EntityFile:                 entityFile,
				EnableRegistrationListener: true,
				MetaOptions: []service.MetaOption{
					service.WithMetaDefaultDeviceID(deviceID),
				},
			},
		)
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
	serveCmd.Flags().StringVar(&ArgHTTPEndpoint, "api-host", "http://127.0.0.1:8001/c8y", "HTTP endpoint that api requests should be sent to")
	serveCmd.Flags().String("entityfile", "", "Load initial entity definitions from a json file")
}
