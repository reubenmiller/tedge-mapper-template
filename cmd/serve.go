/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/reubenmiller/tedge-mapper-template/pkg/errors"
	"github.com/reubenmiller/tedge-mapper-template/pkg/jsonnet"
	"github.com/reubenmiller/tedge-mapper-template/pkg/routes"
	"github.com/reubenmiller/tedge-mapper-template/pkg/service"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/exp/slog"
)

var MaxRecursion int64 = 3

func NewStreamFactory(client mqtt.Client, route routes.Route, opts ...jsonnet.TemplateOption) func(string, string) error {
	engine := jsonnet.NewEngine(
		route.Template.Value,
		opts...,
	)
	stream := streamer.NewStreamer(engine)

	if route.PreProcessor != nil {
		route.PreparePreProcessor()
	}

	return func(topic, message string) error {
		slog.Info("Route activated on message.", "route", route.Name, "topic", topic, "message", message)

		if route.HasPreprocessor() {
			slog.Debug("Applying preprocessor to message")
			v, err := route.ExecutePreprocessor(message)
			if err != nil {
				return err
			}
			slog.Debug("Preprocessor output.", "output", v)
			message = v
		}

		sm, err := stream.Process(topic, message)
		if err != nil {
			slog.Error("Invalid process output.", err)
			return err
		}

		if sm != nil {
			output, err := json.Marshal(sm.Message)
			if err != nil {
				slog.Warn("Preprocessor error.", "error", err)
				return err
			}

			if route.Match(sm.Topic) {
				if n := gjson.GetBytes(output, "__te.lvl"); n.Exists() {
					if n.Int() > MaxRecursion {
						slog.Warn("Nested level exceeded.", "topic", sm.Topic, "message", string(output))
						return errors.ErrRecursiveLevelExceeded
					}
				}
			}

			if sm.Final {
				if o, err := sjson.SetBytes(output, "__te.lvl", MaxRecursion); err == nil {
					output = o
					slog.Info("Set final message.", "topic", sm.Topic, "message", string(output))
				}
			}

			if sm.Skip {
				slog.Info("skip.", "topic", sm.Topic, "message", string(output))
			} else {
				if sm.RawMessage != "" {
					slog.Info("Publishing new message.", "topic", sm.Topic, "message", sm.RawMessage)
					client.Publish(sm.Topic, 0, false, sm.RawMessage)
				} else {
					slog.Info("Publishing new message.", "topic", sm.Topic, "message", string(output))
					client.Publish(sm.Topic, 0, false, output)
				}

				time.Sleep(2 * time.Second)
			}
		}
		return nil
	}
}

var ArgDir string
var ArgBroker string
var ArgClientID string

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the translation service",
	Long: `Run the translator which transforms MQTT messages on a matching topics to
	new MQTT messages
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Info("Starting listener")
		debug, _ := cmd.Root().PersistentFlags().GetBool("debug")

		app, err := service.NewService(ArgBroker, ArgClientID)
		if err != nil {
			return err
		}

		envMap := map[string]string{}
		for _, env := range os.Environ() {
			key, value, found := strings.Cut(env, "=")
			if found && value != "" {
				envMap[key] = value
			}
		}

		meta := map[string]any{
			"device_id": "test",
			"type":      "thin-edge.io",
			"env":       envMap,
		}

		mappings := app.ScanMappingFiles(ArgDir)

		for _, mapping := range mappings {
			if !mapping.Skip {
				slog.Info("Registering route.", "name", mapping.Name, "topic", mapping.Topic)
				err = app.Register(
					mapping.Topic,
					1,
					NewStreamFactory(
						app.Client,
						mapping,
						jsonnet.WithMetaData(meta),
						jsonnet.WithDebug(debug),
					),
				)
				if err != nil {
					return nil
				}
			} else {
				slog.Info("Ignoring route marked as skip.", "name", mapping.Name, "topic", mapping.Topic)
			}
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
	serveCmd.Flags().StringVar(&ArgDir, "dir", "./testdata", "Directory where the routes are stored")
	serveCmd.Flags().StringVar(&ArgBroker, "host", "localhost:1883", "Broker endpoint (can included port number)")
	serveCmd.Flags().StringVarP(&ArgClientID, "clientid", "i", "tedge-mapper-template", "MQTT client id")
}
