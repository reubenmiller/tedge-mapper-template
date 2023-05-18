package service

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/reubenmiller/tedge-mapper-template/pkg/errors"
	"github.com/reubenmiller/tedge-mapper-template/pkg/jsonnet"
	"github.com/reubenmiller/tedge-mapper-template/pkg/routes"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/exp/slog"
)

func NewStreamFactory(client mqtt.Client, route routes.Route, maxDepth int64, postDelay time.Duration, opts ...jsonnet.TemplateOption) func(string, string) error {
	if maxDepth <= 0 {
		maxDepth = 3
	}

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
				if n := gjson.GetBytes(output, "_ctx.lvl"); n.Exists() {
					if n.Int() > maxDepth {
						slog.Warn("Nested level exceeded.", "topic", sm.Topic, "message", string(output))
						return errors.ErrRecursiveLevelExceeded
					}
				}
			}

			if sm.End {
				if o, err := sjson.SetBytes(output, "_ctx.lvl", maxDepth); err == nil {
					output = o
					slog.Info("Setting end message.", "topic", sm.Topic, "message", string(output))
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

				// Prevent posting to quickly
				time.Sleep(postDelay)
			}
		}
		return nil
	}
}

func NewDefaultService(broker string, clientID string, routeDir string, maxdepth int64, postDelay time.Duration, debug bool) (*Service, error) {
	app, err := NewService(broker, clientID)
	if err != nil {
		return nil, err
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

	mappings := app.ScanMappingFiles(routeDir)

	for _, mapping := range mappings {
		if !mapping.Skip {
			slog.Info("Registering route.", "name", mapping.Name, "topic", mapping.Topic)
			err = app.Register(
				mapping.Topic,
				1,
				NewStreamFactory(
					app.Client,
					mapping,
					maxdepth,
					postDelay,
					jsonnet.WithMetaData(meta),
					jsonnet.WithDebug(debug),
				),
			)
			if err != nil {
				return nil, err
			}
		} else {
			slog.Info("Ignoring route marked as skip.", "name", mapping.Name, "topic", mapping.Topic)
		}
	}
	return app, nil
}
