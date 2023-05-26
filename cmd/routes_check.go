/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/reubenmiller/tedge-mapper-template/pkg/jsonnet"
	"github.com/reubenmiller/tedge-mapper-template/pkg/service"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// executeCmd represents the execute command
var executeCmd = &cobra.Command{
	Use:   "check",
	Short: "Check a route",
	Long: `Check a route template using data provided via the command line (no MQTT broker required).

This function is useful for checking if the routes work as expected by testing
them against a given topic and message.

Examples:

	tedge-mapper-template routes check -t 'c8y/s/ds' -m '524,DeviceSerial,http://www.my.url,type' --device-id sim_tedge0
	# Check handling of routes for the 'c8y/s/ds' topic with a given message payload.
	# A custom device-id is also provided for testing.

	tedge-mapper-template routes check -t 'c8y/s/ds' -m ./operation.json
	# Check handling of routes and read the message from file

	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		debug, _ := cmd.Root().PersistentFlags().GetBool("debug")
		routeDirs, _ := cmd.Root().PersistentFlags().GetStringSlice("dir")
		libPaths, _ := cmd.Root().PersistentFlags().GetStringSlice("libdir")
		topic, _ := cmd.Flags().GetString("topic")
		message, _ := cmd.Flags().GetString("message")
		compact, _ := cmd.Flags().GetBool("compact")
		maxDepth, _ := cmd.Root().PersistentFlags().GetInt("maxdepth")
		delay, _ := cmd.Root().PersistentFlags().GetDuration("delay")
		deviceID, _ := cmd.Root().PersistentFlags().GetString("device-id")
		// dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry")
		// Force dry run
		dryRun := true

		useColor := true
		if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			useColor = false
		}

		if _, err := os.Stat(message); err == nil {
			messageFile := message
			slog.Info("Reading input message from file. path=%", messageFile)
			file, err := os.Open(messageFile)
			if err != nil {
				return err
			}
			b, err := io.ReadAll(file)
			if err != nil {
				return err
			}

			message = string(b)
		}

		app, err := service.NewDefaultService(ArgBroker, ArgClientID, ArgCleanSession, "", routeDirs, maxDepth, delay, debug, true, []service.MetaOption{
			service.WithMetaDefaultDeviceID(deviceID),
		}, libPaths, useColor)
		if err != nil {
			return err
		}

		// TODO: Provide the meta data as part of the service
		// and access via app.GetMeta()
		meta := service.NewMetaData(
			service.WithMetaDefaultDeviceID(deviceID),
		)

		slog.Debug("Total routes.", "count", len(app.Routes))

		queue := make(chan streamer.OutputMessage)
		done := make(chan struct{})

		addMessage := func(topic string, message string) {
			queue <- streamer.OutputMessage{
				Topic:   topic,
				Message: message,
			}
		}

		go addMessage(topic, message)

	loop:
		for {
			select {
			case imsg := <-queue:
				go func(msg streamer.OutputMessage) {
					foundRoute := false
					for _, route := range app.Routes {
						if !route.Skip {
							if !route.Match(msg.Topic) {
								slog.Debug("Route did not match topic.", "route", route.Name, "root_topic", route.DisplayTopics(), "topic", topic)
								continue
							}

							foundRoute = true
							// cmd.Printf("Route:\t%s\n", route.Name)
							handler := service.NewStreamFactory(nil, nil, route, maxDepth, 0,
								jsonnet.WithMetaData(meta),
								jsonnet.WithDebug(debug),
								jsonnet.WithDryRun(dryRun),
								// jsonnet.WithColorStackTrace(useColor),
							)

							output, err := handler(msg.Topic, msg.MessageString())
							if err != nil {
								slog.Error("handler returned an error.", "err", err)
								done <- struct{}{}
								return
							}

							if stop := service.DisplayMessage(fmt.Sprintf("%s (%s)", route.Name, route.DisplayTopics()), &imsg, output, cmd.OutOrStdout(), compact, useColor); stop {
								done <- struct{}{}
								return
							}
							slog.Info("Queuing new message")
							go addMessage(output.Topic, output.MessageString())
						}
					}
					if !foundRoute {
						slog.Info("No matching routes found. Stopping")
						done <- struct{}{}
					}
				}(imsg)

			case <-done:
				break loop
			}
		}
		return nil
	},
}

func init() {
	routesCmd.AddCommand(executeCmd)

	executeCmd.Flags().StringP("topic", "t", "", "Topic")
	executeCmd.Flags().StringP("message", "m", "", "Input message. Accepts a string or a path to a file")
	executeCmd.Flags().StringP("file", "f", "", "Template file")
	executeCmd.Flags().Bool("compact", false, "Print output message in compact format (not pretty printed)")
}
