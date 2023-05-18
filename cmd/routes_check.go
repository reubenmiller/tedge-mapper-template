/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"encoding/json"
	"io"
	"os"

	"github.com/reubenmiller/tedge-mapper-template/pkg/jsonnet"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"github.com/spf13/cobra"
)

var ArgFile string
var ArgMessage string
var ArgTopic string

// executeCmd represents the execute command
var executeCmd = &cobra.Command{
	Use:   "check",
	Short: "Check a route",
	Long: `Check a route template using data provided via the command line.

This function is useful for checking if the routes work as expected.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		debug, _ := cmd.Root().Flags().GetBool("debug")

		file, err := os.Open(ArgFile)
		if err != nil {
			return err
		}

		templateStr, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		meta := map[string]string{
			"device_id": "test",
			"type":      "thin-edge.io",
		}

		engine := jsonnet.NewEngine(
			string(templateStr),
			jsonnet.WithMetaData(meta),
			jsonnet.WithDebug(debug),
		)
		stream := streamer.NewStreamer(engine)

		sm, err := stream.Process(ArgTopic, ArgMessage)
		if err != nil {
			return err
		}

		if sm.Skip {
			cmd.PrintErrf("Ignoring message\n")
			return nil
		}

		output, err := json.Marshal(sm.Message)
		if err != nil {
			return err
		}

		cmd.Printf("Topic: %s\n", sm.Topic)
		cmd.Printf("Message:\n%s", output)
		return nil
	},
}

func init() {
	routesCmd.AddCommand(executeCmd)

	executeCmd.Flags().StringVarP(&ArgTopic, "topic", "t", "", "Topic")
	executeCmd.Flags().StringVarP(&ArgMessage, "message", "m", "", "Input message")
	executeCmd.Flags().StringVarP(&ArgFile, "file", "f", "", "Template file")
}
