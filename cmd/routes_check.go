/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"encoding/json"

	"github.com/reubenmiller/tedge-mapper-template/pkg/jsonnet"
	"github.com/reubenmiller/tedge-mapper-template/pkg/service"
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

	tedge-mapper-template routes check -t 'c8y/s/ds' -m '524,DeviceSerial,http://www.my.url,type'
	# Check handling of routes for the 'c8y/s/ds' topic with a given message payload

	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		debug, _ := cmd.Root().PersistentFlags().GetBool("debug")
		routeDir, _ := cmd.Flags().GetString("dir")
		topic, _ := cmd.Flags().GetString("topic")
		message, _ := cmd.Flags().GetString("message")

		app, err := service.NewDefaultService(ArgBroker, ArgClientID, ArgDir, ArgMaxDepth, ArgDelay, debug)
		if err != nil {
			return err
		}

		meta := service.NewMetaData()
		routes := app.ScanMappingFiles(routeDir)

		slog.Debug("Total routes.", "count", len(routes))

		for _, route := range routes {
			if !route.Skip {
				if route.Match(topic) {
					handler := service.NewStreamFactory(nil, route, 1, 0,
						jsonnet.WithMetaData(meta),
						jsonnet.WithDebug(debug),
					)

					output, err := handler(topic, message)
					if err != nil {
						slog.Error("handler returned an error.", "err", err)
					} else {
						cmd.Printf("topic\t%s\n", output.Topic)
						cmd.Printf("skip\t%v\n", output.Skip)
						cmd.Printf("end\t%v\n", output.End)
						if output.RawMessage != "" {
							cmd.Printf("%s", output.RawMessage)
						} else {
							out, err := json.Marshal(output.Message)
							if err != nil {
								cmd.Printf("%s", err)
							} else {
								cmd.Printf("%s", out)
							}
						}
					}
				} else {
					slog.Debug("Route did not match topic.", "route", route.Name, "root_topic", route.Topic, "topic", topic)
				}
			}
		}
		return nil
	},
}

func init() {
	routesCmd.AddCommand(executeCmd)

	executeCmd.Flags().StringP("topic", "t", "", "Topic")
	executeCmd.Flags().StringP("message", "m", "", "Input message")
	executeCmd.Flags().StringP("file", "f", "", "Template file")
	executeCmd.Flags().String("dir", "routes", "Route directory")
}
