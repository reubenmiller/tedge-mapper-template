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
}

type APIClient struct {
	*c8y.Client

	OnRequestDenied  func() error
	nextTokenRequest time.Time
}

func NewCumulocityClient(host string, tokenRequestor func() error) *APIClient {
	if host == "" {
		host = os.Getenv("C8Y_HOST")
	}

	if host == "" {
		v, tErr := GetCumulocityURL()
		if tErr == nil {
			host = v
		} else {
			slog.Info("Tried getting the c8y.http setting from tedge failed.", "error", tErr)
		}
	}

	// Support Basic Auth if env variables are set (as this makes it easier to run in non thin-edge.io environments)
	username := os.Getenv("C8Y_USER")
	password := os.Getenv("C8Y_PASSWORD")
	client := c8y.NewClient(nil, host, "", username, password, true)

	if password == "" {
		client.AuthorizationMethod = c8y.AuthMethodOAuth2Internal
	}

	if token := os.Getenv("C8Y_TOKEN"); token != "" {
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

func c8yTokenUpdater(client *c8y.Client) func(c mqtt.Client, m mqtt.Message) {
	return func(c mqtt.Client, m mqtt.Message) {
		if client != nil {
			if len(m.Payload()) > 0 {
				slog.Info("Received new c8y token", "len", len(m.Payload()))
				client.SetToken(string(m.Payload()))
			}
		}
	}
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

func (s *Service) ScanMappingFiles(dir string) []routes.Route {
	slog.Info("Scanning for routes.", "path", dir)
	mappings := make([]routes.Route, 0)
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
				mappings = append(mappings, spec.Routes...)
			} else {
				slog.Info("Skipping routes as file is marked as disabled.", "file", path)
			}
		}
		return nil
	})
	if err != nil {
		slog.Default().Warn("Error whilst looking for files.", "err", err)
	}
	return mappings
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
