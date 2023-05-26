package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/tedge-mapper-template/pkg/routes"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"golang.org/x/exp/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

type Service struct {
	Client        mqtt.Client
	APIClient     *APIClient
	Subscriptions map[string]byte
	Routes        []routes.Route
}

type APIClient struct {
	*c8y.Client

	OnRequestDenied  func() error
	nextTokenRequest time.Time
}

type SettingOption func() string

func WithSettings(opts ...SettingOption) string {
	for _, opt := range opts {
		if v := opt(); v != "" {
			return v
		}
	}
	return ""
}

// Get an environment variable
func WithEnvironment(key string) SettingOption {
	return func() string {
		return os.Getenv(key)
	}
}

// Get an override value
func WithValue(value string) SettingOption {
	return func() string {
		return value
	}
}

// Get a configuration value from thin-edge.io using the tedge cli
func WithTedgeSetting(key string) SettingOption {
	return func() string {
		_, err := exec.LookPath(TedgeBinary)
		if err != nil {
			slog.Info("Could not find the tedge binary", "error", err)
			return ""
		}
		output, err := exec.Command("tedge", "config", "get", key).Output()
		if err != nil {
			slog.Info("Tried getting the c8y.http setting from tedge failed.", "error", err)
			return ""
		}
		return string(bytes.TrimSpace(output))
	}
}

func NewCumulocityClient(host string, tokenRequestor func() error) *APIClient {
	host = WithSettings(
		WithValue(host),
		WithEnvironment("C8Y_HOST"),
		WithTedgeSetting("c8y.http"),
	)
	// TODO: go-c8y should handle adding a https:// prefix by default
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	username := WithSettings(
		WithEnvironment("C8Y_USER"),
	)
	tenant := WithSettings(
		WithEnvironment("C8Y_TENANT"),
	)
	password := WithSettings(
		WithEnvironment("C8Y_PASSWORD"),
	)
	token := WithSettings(
		WithEnvironment("C8Y_TOKEN"),
	)

	// Support Basic Auth if env variables are set (as this makes it easier to run in non thin-edge.io environments)
	client := c8y.NewClient(nil, host, tenant, username, password, true)

	if password == "" {
		client.AuthorizationMethod = c8y.AuthMethodOAuth2Internal
	}

	if token != "" {
		client.SetToken(token)
	}

	slog.Info("c8y client.", "host", client.BaseURL, "user", client.Username)

	return &APIClient{
		Client:          client,
		OnRequestDenied: tokenRequestor,
	}
}

func (c *APIClient) renewToken() {
	if c.OnRequestDenied != nil {
		// Limit how often the permission denied response is called
		now := time.Now()
		if now.After(c.nextTokenRequest) {
			if err := c.OnRequestDenied(); err != nil {
				slog.Error("Could not request.", "err", err)
			}
			// TODO: Make minimum interval configurable
			c.nextTokenRequest = now.Add(60 * time.Second)
		} else {
			slog.Info("Permission denied back-off timer has not expired. nextTokenRequestAfter=%s", now.Format(time.RFC3339))
		}
	}
}

func (c *APIClient) SendRequest(ctx context.Context, options c8y.RequestOptions) (*c8y.Response, error) {
	if c.Token == "" && c.Password == "" {
		return nil, errors.New("client has no authorization credentials")
	}

	resp, err := c.Client.SendRequest(ctx, options)
	if err != nil {
		return resp, err
	}
	if resp != nil {
		// Only handle HTTP Status Code 401 requests
		if resp.StatusCode() == http.StatusUnauthorized {
			c.renewToken()
		}
	}
	return resp, err
}

func (s *Service) ReceiveCumulocityToken(value []byte) error {
	slog.Info("Received new c8y token", "len", len(value))

	if s.APIClient != nil {
		if len(value) > 0 {
			value = bytes.TrimPrefix(value, []byte("71,"))
			slog.Info("Setting token to api client")
			s.APIClient.SetToken(string(value))
		}
	}
	return nil
}

var ErrNoMQTTClient = errors.New("no mqtt client")

func RequestCumulocityToken(client mqtt.Client) func() error {
	return func() error {
		if client == nil {
			return ErrNoMQTTClient
		}
		slog.Info("Requesting new token")
		client.Publish("c8y/s/uat", 0, false, "")
		return nil
	}
}

func NewService(broker string, clientID string, cleanSession bool, httpEndpoint string, dryRun bool) (*Service, error) {
	healthTopic := fmt.Sprintf("tedge/health/%s", clientID)
	opts := mqtt.NewClientOptions().SetClientID(clientID).AddBroker(broker).SetCleanSession(cleanSession).SetWill(healthTopic, `{"status":"down"}`, 1, true)
	client := mqtt.NewClient(opts)

	if !dryRun {
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			return nil, token.Error()
		}
	}

	// TODO sub scribe to
	service := &Service{
		Client:        client,
		Subscriptions: map[string]byte{},
		Routes:        []routes.Route{},
	}
	service.APIClient = NewCumulocityClient(httpEndpoint, RequestCumulocityToken(client))

	client.AddRoute("c8y/s/dat", func(c mqtt.Client, m mqtt.Message) {
		service.ReceiveCumulocityToken(m.Payload())
	})
	service.Subscriptions["c8y/s/dat"] = 0

	client.Publish(healthTopic, 1, true, `{"status":"up"}`)
	go func(c mqtt.Client) {
		<-time.After(10 * time.Second)
		service.APIClient.renewToken()
	}(client)

	return service, nil
}

func isYaml(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func (s *Service) GetRoutes() []routes.Route {
	return s.Routes
}

func (s *Service) ClearRoutes() {
	s.Routes = nil
}

// Scan for routes from a directory. It will automatically add routes to the existing list.
// Use an explicit call to ClearRoutes if you want to clear existing routes before calling this function.
// Note: Currently routes are not unregistered from the MQTT client. For this to occur the MQTT client needs
// to be stopped and destroyed.
func (s *Service) ScanMappingFiles(dirs []string) []routes.Route {
	if s.Routes == nil {
		s.Routes = make([]routes.Route, 0)
	}

	for _, dir := range dirs {
		slog.Info("Scanning for routes.", "path", dir)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsDir() {
				return nil
			}
			if isYaml(d.Name()) {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				b, err := io.ReadAll(file)
				if err != nil {
					return err
				}

				spec := &routes.Specification{}
				if err := yaml.Unmarshal(b, spec); err != nil {
					return err
				}
				if !spec.Disable {
					s.Routes = append(s.Routes, spec.Routes...)
				} else {
					slog.Info("Skipping routes as file is marked as disabled.", "file", path)
				}
			}
			return nil
		})
		if err != nil {
			slog.Default().Warn("Error whilst looking for files.", "err", err)
		}
	}
	return s.Routes
}

type MessageHandler func(topic string, message_in string) (message_out *streamer.OutputMessage, err error)

func (s *Service) Register(topics []string, qos byte, handler MessageHandler) error {
	handlerWrapper := func(c mqtt.Client, m mqtt.Message) {
		slog.Info("Received message.", "topic", m.Topic(), "payload_len", len(m.Payload()))
		handler(m.Topic(), string(m.Payload()))
	}

	for _, topic := range topics {
		if _, exists := s.Subscriptions[topic]; exists {
			slog.Warn("Duplicate topic detected. The new handler will replace the previous one.", "topic", topic)
		}
		s.Subscriptions[topic] = qos
		s.Client.AddRoute(topic, handlerWrapper)
	}
	return nil
}

func (s *Service) StartSubscriptions() error {
	if len(s.Subscriptions) == 0 {
		slog.Warn("No routes were detected, so nothing to subscribe to")
		return nil
	}
	slog.Info("Subscribing to MQTT topics.", "topics", s.Subscriptions)
	if token := s.Client.SubscribeMultiple(s.Subscriptions, nil); token.Wait() && token.Error() != nil {
		return fmt.Errorf("error subscribing to topic '%v': %v", s.Subscriptions, token.Error())
	}
	return nil
}
