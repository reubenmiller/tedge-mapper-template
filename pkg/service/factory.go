package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/fatih/color"
	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/tedge-mapper-template/pkg/errors"
	"github.com/reubenmiller/tedge-mapper-template/pkg/jsonnet"
	"github.com/reubenmiller/tedge-mapper-template/pkg/routes"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
	"golang.org/x/exp/slog"
)

var TedgeBinary = "tedge"

func NewStreamFactory(client mqtt.Client, apiClient *APIClient, route routes.Route, maxDepth int, postDelay time.Duration, opts ...jsonnet.TemplateOption) MessageHandler {

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
				// TODO: Should preprocessor errors be logged instead of returning early
				return nil, fmt.Errorf("preprocessor error. %s, message=%s", err, message)
			} else {
				slog.Debug("Preprocessor m.", "output", v)
				message = v
			}
		}

		sm, err := stream.Process(topic, message)
		if err != nil {
			slog.Error("Invalid process m.", err)
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

		// Check if there are any message to be sent before processing the main message
		for _, m := range sm.Updates {
			if m.Skip {
				continue
			}
			switch m.Message.(type) {
			case string:
				slog.Info("Publishing update message.", "topic", m.Topic, "message", m.Message)
				if client != nil {
					client.Publish(m.Topic, 0, false, m.Message)
				}
			default:
				preMsg, preErr := json.Marshal(m.Message)
				if preErr != nil {
					slog.Warn("Invalid update message.", "error", preErr)
				} else {
					slog.Info("Publishing update message.", "topic", m.Topic, "message", string(preMsg))
					if client != nil {
						client.Publish(m.Topic, 0, false, preMsg)
					}
				}
			}
		}

		// Apply depth limit to all messages, and not just a message
		// which generates a message from the same topic to protect against
		// infinite loops via multiple routes, e.g.: A -> B -> C -> A (not just A -> A)
		if n := gjson.GetBytes(output, "_ctx.lvl"); n.Exists() {
			if n.Int() > int64(maxDepth) {
				slog.Warn("Nested level exceeded.", "topic", sm.Topic, "message", string(output), "limit", maxDepth)
				return nil, errors.ErrRecursiveLevelExceeded
			}
		}

		if sm.End {
			if o, err := sjson.SetBytes(output, "_ctx.lvl", maxDepth); err == nil {
				output = o
				slog.Info("Setting end message.", "topic", sm.Topic, "message", string(output))
			}
		}

		if sm.DisableContext() {
			// TODO: Check that the message will not trigger other routes (since the infinite loop is being disabled)
			if o, err := sjson.DeleteBytes(output, "_ctx"); err == nil {
				output = o
				slog.Info("Removing context from message.", "topic", sm.Topic, "message", string(output))
			} else {
				slog.Info("Failed to remove context from message.", "topic", sm.Topic, "message", string(output), "error", err)
			}
		}

		if sm.Skip {
			slog.Info("skip.", "topic", sm.Topic, "message", string(output))
		} else {
			// TODO: Switch to using the .MessageString() method
			if sm.IsMQTTMessage() {
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
			}

			if sm.IsAPIRequest() {
				if err := sm.API.Validate(); err != nil {
					slog.Error("Invalid api request.", "error", err)
					return nil, err
				}
				if err := SendAPIRequest(apiClient, sm.API.Host, sm.API.Method, sm.API.Path, sm.MessageString()); err != nil {
					slog.Error("Failed to send api request.", "error", err)
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

func SendAPIRequest(client *APIClient, host, method, path, body string) (err error) {
	if client == nil {
		return fmt.Errorf("api client is not set")
	}
	r := strings.NewReader(body)
	opt := c8y.RequestOptions{
		Host:             host,       // if host is empty, then the default host in the c8y client is used
		NoAuthentication: host != "", // but don't send the auth token to prevent sending credentials to potentially unsecured service
		Method:           method,
		Path:             path,
		Accept:           "application/json",
		Body:             r,
	}

	resp, err := client.SendRequest(context.Background(), opt)
	if err != nil {
		return err
	}
	slog.Info("Sent request", "response", resp.JSON().Raw)
	return nil
}

func GetCumulocityURL() (string, error) {
	_, err := exec.LookPath(TedgeBinary)
	if err != nil {
		return "", err
	}
	cmd, err := exec.Command("tedge", "config", "get", "c8y.http").Output()
	return string(cmd), err
}

func NewMetaData() map[string]any {
	envMap := map[string]string{}
	for _, env := range os.Environ() {
		// Only include env variables starting with TEDGE_ROUTE
		// to limit amount of spam in the templates and to limit
		// exposing potential secrets to templates
		if !strings.HasPrefix(env, "TEDGE_ROUTE") {
			continue
		}
		key, value, found := strings.Cut(env, "=")
		if found && value != "" {
			envMap[key] = value
		}
	}

	meta := map[string]any{
		"env": envMap,
	}

	// Add tedge config
	if _, err := exec.LookPath(TedgeBinary); err == nil {
		cmd, err := exec.Command("tedge", "config", "list").Output()
		if err != nil {
			slog.Warn("Could not get tedge config.", "error", err)
		} else {
			buf := bytes.NewBuffer(cmd)
			scanner := bufio.NewScanner(buf)
			for scanner.Scan() {
				if key, value, found := strings.Cut(scanner.Text(), "="); found {
					keyNormalized := strings.ReplaceAll(key, ".", "_")
					if keyNormalized != "" && value != "" {
						meta[keyNormalized] = value
					}

				}
			}
		}
	} else {
		// TESTING ONLY: Provide a way for testing without having tedge installed
		// The environment variables will be normalized to mimic the tedge config list
		// settings.
		for _, env := range os.Environ() {
			// Only include env variables starting with TEDGE_ROUTE
			// to limit amount of spam in the templates and to limit
			// exposing potential secrets to templates
			if !strings.HasPrefix(env, "TEDGE_") && !strings.HasPrefix(env, "TEDGE_ROUTE") {
				continue
			}
			key, value, found := strings.Cut(env, "=")
			if found && value != "" {
				keyNormalized := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(key, "TEDGE_"), ".", "_"))
				meta[keyNormalized] = value
			}
		}
	}
	return meta
}

func NewDefaultService(broker string, clientID string, cleanSession bool, httpEndpoint string, routeDir string, maxdepth int, postDelay time.Duration, debug bool, dryRun bool) (*Service, error) {
	app, err := NewService(broker, clientID, cleanSession, httpEndpoint, dryRun)
	if err != nil {
		return nil, err
	}

	meta := NewMetaData()
	routes := app.ScanMappingFiles(routeDir)

	for _, route := range routes {
		if !route.Skip {
			slog.Info("Registering route.", "name", route.Name, "topics", route.DisplayTopics())
			err = app.Register(
				route.Topics,
				1,
				NewStreamFactory(
					app.Client,
					app.APIClient,
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
			slog.Info("Ignoring route marked as skip.", "name", route.Name, "topics", route.DisplayTopics())
		}
	}
	return app, nil
}

func DisplayMessage(name string, in, out *streamer.OutputMessage, w io.Writer, compact bool) bool {

	header := color.New(color.Bold).Add(color.BgCyan)
	header.Fprintf(w, "Route: %s", name)
	fmt.Fprint(w, "\n")

	fmt.Fprint(w, "\nInput Message\n")
	fmt.Fprintf(w, "  %-10v%v\n", "topic:", in.Topic)

	if !out.Skip {
		// Display and update messages
		if len(out.Updates) > 0 {
			fmt.Fprintf(w, "\nOutput Updates\n")
			for _, update := range out.Updates {
				if !update.Skip {
					fmt.Fprintf(w, "  %-10s%v\n", "topic:", update.Topic)
					displayJsonMessage(w, update.Message, compact)
				}
			}
		}
	}

	fmt.Fprintf(w, "\nOutput Message (%s)\n", out.GetType())
	if out.IsMQTTMessage() {
		fmt.Fprintf(w, "  %-10s%v\n", "topic:", out.Topic)
		fmt.Fprintf(w, "  %-10s%v\n", "end:", out.End)
	}

	if out.IsAPIRequest() {
		// API message don't chain, so no point printing the 'end' meta info
		fmt.Fprintf(w, "  %-10s%v %v\n", "request:", out.API.Method, out.API.Path)
	}

	if !out.Skip {
		if out.RawMessage != "" {
			fmt.Fprintf(w, "%s\n", out.RawMessage)
		} else {
			displayJsonMessage(w, out.Message, compact)
		}
	}
	return out.Skip || out.End
}

func displayJsonMessage(w io.Writer, value any, compact bool) {
	var outB []byte
	var err error

	if strValue, ok := value.(string); ok {
		fmt.Fprintf(w, "\n%s\n\n", strValue)
		return
	}

	if compact {
		outB, err = json.Marshal(value)
	} else {
		outB, err = json.MarshalIndent(value, "", "  ")
		if err == nil {
			outB = pretty.Color(outB, nil)
		}
	}
	if err != nil {
		fmt.Fprintf(w, "\n%s\n\n", err)
	} else {
		fmt.Fprintf(w, "\n%s\n\n", outB)
	}
}
