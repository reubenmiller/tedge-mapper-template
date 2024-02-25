package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
)

var TedgeBinary = "tedge"

func optionalDelay(delaySec float32, f func()) {
	// Don't bother with sub second delays
	if delaySec > 0.9 {
		time.AfterFunc(time.Duration(int(delaySec*1000))*time.Millisecond, f)
	} else {
		f()
	}
}

func WithMQTTPublisher(client mqtt.Client, topic string, qos byte, retain bool, message any) func() {
	return func() {
		client.Publish(topic, qos, retain, message)
	}
}

func WithRESTRequest(client *APIClient, host, method, path string, message any) func() {
	return func() {
		if err := SendAPIRequest(client, host, method, path, message); err != nil {
			slog.Warn("Failed to send api request.", "error", err)
		}
	}
}

type VariablesFactory func() string

func NewStreamFactory(client mqtt.Client, apiClient *APIClient, route routes.Route, variablesFactory VariablesFactory, maxDepth int, postDelay time.Duration, opts ...jsonnet.TemplateOption) MessageHandler {

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

	variablesFunc := func() string { return "" }

	if variablesFactory != nil {
		variablesFunc = variablesFactory
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

		sm, err := stream.Process(topic, message, variablesFunc())
		if err != nil {
			slog.Error("Template error.", "route", route.Name)

			// Print error to stderr directly as sometimes errors are nicely formatted
			fmt.Fprint(os.Stderr, err.Error())
			return nil, errors.ErrTemplateException
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
				if client != nil && !engine.DryRun() {
					optionalDelay(m.Delay, WithMQTTPublisher(client, m.Topic, m.GetQoS(), m.Retain, m.Message))
				}
			default:
				preMsg, preErr := json.Marshal(m.Message)
				if preErr != nil {
					slog.Warn("Invalid update message.", "error", preErr)
				} else {
					slog.Info("Publishing update message.", "topic", m.Topic, "message", string(preMsg))
					if client != nil && !engine.DryRun() {
						optionalDelay(m.Delay, WithMQTTPublisher(client, m.Topic, m.GetQoS(), m.Retain, preMsg))
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
				if sm.RawMessage != nil {
					slog.Info("Publishing new raw message.", "topic", sm.Topic, "message", *sm.RawMessage, "retain", sm.Retain, "delay", sm.Delay)
					if client != nil && !engine.DryRun() {
						optionalDelay(sm.Delay, WithMQTTPublisher(client, sm.Topic, sm.GetQoS(), sm.Retain, *sm.RawMessage))
					}
				} else {
					slog.Info("Publishing new message.", "topic", sm.Topic, "message", string(output), "delay", sm.Delay)
					if client != nil && !engine.DryRun() {
						optionalDelay(sm.Delay, WithMQTTPublisher(client, sm.Topic, sm.GetQoS(), sm.Retain, output))
					}
				}
			}

			if sm.IsAPIRequest() {
				if err := sm.API.Validate(); err != nil {
					slog.Error("Invalid api request.", "error", err)
					return nil, err
				}
				if !engine.DryRun() {
					optionalDelay(sm.Delay, WithRESTRequest(apiClient, sm.API.Host, sm.API.Method, sm.API.Path, sm.API.Body))
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

func SendAPIRequest(client *APIClient, host, method, path string, body any) (err error) {
	if client == nil {
		return fmt.Errorf("api client is not set")
	}

	opt := c8y.RequestOptions{
		Host:             host,       // if host is empty, then the default host in the c8y client is used
		NoAuthentication: host != "", // but don't send the auth token to prevent sending credentials to potentially unsecured service
		Method:           method,
		Path:             path,
		Accept:           "application/json",
		Body:             body,
	}

	resp, err := client.SendRequest(context.Background(), opt)
	if err != nil {
		return err
	}
	slog.Info("Sent request.", "response", resp.JSON().Raw)
	return nil
}

type MetaOption func(m map[string]any)

func WithMetaDefaultValue(key string, value any) MetaOption {
	return func(meta map[string]any) {
		keyNormalized := strings.ToLower(strings.ReplaceAll(key, ".", "_"))
		if keyNormalized != "" {
			meta[keyNormalized] = value
		}
	}
}

func WithMetaDefaultDeviceID(value string) MetaOption {
	return func(meta map[string]any) {
		meta["device_id"] = value
	}
}

func WithMetaHostname() MetaOption {
	return func(meta map[string]any) {
		hostname, err := os.Hostname()
		if err != nil {
			slog.Warn("Could not get hostname.", "error", err)
			// use empty value so that templates don't need to do null checks
			meta["hostname"] = ""
		} else {
			meta["hostname"] = hostname
		}
	}
}

func NewMetaData(defaults ...MetaOption) map[string]any {
	meta := map[string]any{}
	meta["env"] = map[string]string{}

	// Apply any defaults given by the user
	for _, opt := range defaults {
		opt(meta)
	}

	WithMetaHostname()(meta)

	for _, env := range os.Environ() {
		// Only include env variables starting with ROUTE_
		// to limit amount of spam in the templates and to limit
		// exposing potential secrets to templates
		if !strings.HasPrefix(env, "ROUTE_") {
			continue
		}
		key, value, found := strings.Cut(env, "=")
		if found && value != "" {
			meta["env"].(map[string]string)[key] = value
		}
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
			// Only include env variables starting with TEDGE_
			// to limit amount of spam in the templates and to limit
			// exposing potential secrets to templates
			if !strings.HasPrefix(env, "TEDGE_") {
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

type DefaultServiceOptions struct {
	Broker                     string
	ClientID                   string
	CleanSession               bool
	HTTPEndpoint               string
	RouteDirs                  []string
	MaxRouteDepth              int
	PostMessageDelay           time.Duration
	Debug                      bool
	DryRun                     bool
	MetaOptions                []MetaOption
	LibraryPaths               []string
	UseColor                   bool
	EntityFile                 string
	EnableRegistrationListener bool
}

func NewDefaultService(opts *DefaultServiceOptions) (*Service, error) {
	app, err := NewService(opts.Broker, opts.ClientID, opts.CleanSession, opts.HTTPEndpoint, opts.DryRun)
	if err != nil {
		return nil, err
	}

	meta := NewMetaData(opts.MetaOptions...)

	if opts.EntityFile != "" {
		if _, err := os.Stat(opts.EntityFile); err == nil {
			entityFileContents, readErr := os.ReadFile(opts.EntityFile)
			if readErr != nil {
				return nil, readErr
			}
			slog.Info("Loading initial entity definitions from file.", "file", opts.EntityFile, "contents", string(entityFileContents))
			app.EntityStore.SetFromJSON(entityFileContents, true, true)
		}
	}

	// Handle entity registration independently
	registrationClient := mqtt.NewClient(mqtt.NewClientOptions().SetClientID(opts.ClientID + "_regListener").AddBroker(opts.Broker).SetCleanSession(opts.CleanSession))
	if token := registrationClient.Connect(); !token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	if opts.EnableRegistrationListener {
		registerCallback := func(c mqtt.Client, m mqtt.Message) {
			slog.Info("Received registration message.", "topic", m.Topic(), "message", string(m.Payload()))
			nonEmptyParts := make([]string, 0)
			for _, part := range strings.Split(m.Topic(), "/") {
				if part != "" {
					nonEmptyParts = append(nonEmptyParts, part)
				}
			}
			name := strings.Join(nonEmptyParts, "/")

			entity := Entity{}
			if err := json.Unmarshal(m.Payload(), &entity); err != nil {
				slog.Warn("Invalid registration payload.", err)
				return
			}

			slog.Info("Registering entity", "topic", m.Topic(), "name", name)

			if err := app.EntityStore.Set(name, entity); err != nil {
				slog.Warn("Could not register entity", "error", err)
				return
			}

			slog.Info("Registered entity successfully", "entity", slog.StringValue(fmt.Sprintf("%#v", entity)))
		}

		regTopics := map[string]byte{
			"te/+/+/+/+": 1,
		}
		if token := registrationClient.SubscribeMultiple(regTopics, registerCallback); token.Wait() && token.Error() != nil {
			return nil, fmt.Errorf("error subscribing to topic '%v': %v", regTopics, token.Error())
		}
	}

	routes := app.ScanMappingFiles(opts.RouteDirs)
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
					app.GetVariables,
					opts.MaxRouteDepth,
					opts.PostMessageDelay,
					jsonnet.WithMetaData(meta),
					jsonnet.WithDebug(opts.Debug),
					jsonnet.WithDryRun(opts.DryRun),
					jsonnet.WithLibraryPaths(opts.LibraryPaths...),
					jsonnet.WithColorStackTrace(opts.UseColor),
				),
			)
			if err != nil {
				slog.Warn("Failed to register route. It will be ignored.", "name", route.Name, "error", err)
			}
		} else {
			slog.Info("Ignoring route marked as skip.", "name", route.Name, "topics", route.DisplayTopics())
		}
	}
	return app, nil
}

func DisplayMessage(name string, in, out *streamer.OutputMessage, w io.Writer, compact bool, useColor bool) bool {

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
					if update.Delay > 0 {
						fmt.Fprintf(w, "  %-10s%v (delayed: %.1fs)\n", "topic:", update.Topic, update.Delay)
					} else {
						fmt.Fprintf(w, "  %-10s%v\n", "topic:", update.Topic)
					}
					displayJsonMessage(w, update.Message, compact, useColor)
				}
			}
		}
	}

	if !out.Skip {
		fmt.Fprintf(w, "\nOutput Message (%s)\n", out.GetType())
	}
	if out.IsMQTTMessage() && !out.Skip {
		fmt.Fprintf(w, "  %-10s%v\n", "topic:", out.Topic)
		if out.End {
			fmt.Fprintf(w, "  %-10s%v\n", "end:", out.End)
		}
	}

	if out.IsAPIRequest() && !out.Skip {
		// API message don't chain, so no point printing the 'end' meta info
		fmt.Fprintf(w, "  %-10s%v %v\n", "request:", out.API.Method, out.API.Path)
	}

	if !out.Skip {
		if out.RawMessage != nil {
			fmt.Fprintf(w, "%s\n", *out.RawMessage)
		} else {
			displayJsonMessage(w, out.Message, compact, useColor)
		}
	}
	return out.Skip || out.End
}

func displayJsonMessage(w io.Writer, value any, compact, useColor bool) {
	var outB []byte
	var err error

	if strValue, ok := value.(string); ok {
		fmt.Fprintf(w, "\n%s\n\n", strValue)
		return
	}

	if compact {
		outB, err = json.Marshal(value)
	} else {
		outB, err = json.MarshalIndent(value, "    ", "  ")
		if err == nil {
			if useColor {
				outB = pretty.Color(outB, nil)
			}
		}
	}
	if err != nil {
		fmt.Fprintf(w, "\n    %s\n\n", err)
	} else {
		fmt.Fprintf(w, "\n    %s\n\n", outB)
	}
}
