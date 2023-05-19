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

func NewStreamFactory(client mqtt.Client, route routes.Route, maxDepth int64, postDelay time.Duration, opts ...jsonnet.TemplateOption) MessageHandler {
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

	return func(topic, message string) (*streamer.OutputMessage, error) {
		slog.Info("Route activated on message.", "route", route.Name, "topic", topic, "message", message)

		if route.HasPreprocessor() {
			slog.Debug("Applying preprocessor to message")
			v, err := route.ExecutePreprocessor(message)
			if err != nil {
				return nil, err
			}
			slog.Debug("Preprocessor output.", "output", v)
			message = v
		}

		sm, err := stream.Process(topic, message)
		if err != nil {
			slog.Error("Invalid process output.", err)
			return nil, err
		}

		// TODO: Can sm ever by nil, if not then remove useless condition
		if sm == nil {
			return nil, nil
		}

		output, err := json.Marshal(sm.Message)
		if err != nil {
			slog.Warn("Preprocessor error.", "error", err)
			return nil, err
		}

		if route.Match(sm.Topic) {
			if n := gjson.GetBytes(output, "_ctx.lvl"); n.Exists() {
				if n.Int() > maxDepth {
					slog.Warn("Nested level exceeded.", "topic", sm.Topic, "message", string(output))
					return nil, errors.ErrRecursiveLevelExceeded
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
				if client != nil {
					client.Publish(sm.Topic, 0, false, sm.RawMessage)
				}
			} else {
				slog.Info("Publishing new message.", "topic", sm.Topic, "message", string(output))
				if client != nil {
					client.Publish(sm.Topic, 0, false, output)
				}
			}

			// Prevent posting to quickly
			time.Sleep(postDelay)
		}

		// Update modified output message (with updated context)
		if err := json.Unmarshal(output, &sm.Message); err != nil {
			return nil, err
		}

		return sm, nil
	}
}

func NewMetaData() map[string]any {
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
	return meta
}

func NewDefaultService(broker string, clientID string, routeDir string, maxdepth int64, postDelay time.Duration, debug bool) (*Service, error) {
	app, err := NewService(broker, clientID)
	if err != nil {
		return nil, err
	}

	meta := NewMetaData()
	routes := app.ScanMappingFiles(routeDir)

	for _, route := range routes {
		if !route.Skip {
			slog.Info("Registering route.", "name", route.Name, "topic", route.Topic)
			err = app.Register(
				route.Topic,
				1,
				NewStreamFactory(
					app.Client,
					route,
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
			slog.Info("Ignoring route marked as skip.", "name", route.Name, "topic", route.Topic)
		}
	}
	return app, nil
}
