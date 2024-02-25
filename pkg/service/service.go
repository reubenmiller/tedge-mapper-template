package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/tedge-mapper-template/pkg/routes"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

type Entity struct {
	ID          string          `json:"@id"`
	EntityType  string          `json:"@type"`
	ParentID    string          `json:"@parent"`
	Type        string          `json:"type"`
	DisplayName string          `json:"displayName"`
	Contents    []EntityContent `json:"contents,omitempty"`
}

type EntityContent struct {
	ID     string `json:"@id"`
	Type   string `json:"@type"`
	Value  string `json:"value"`
	Schema string `json:"schema"`
}

func NewEntityStore() *EntityStore {
	return &EntityStore{
		entities: make(map[string]Entity),
	}
}

type EntityStore struct {
	mu       sync.RWMutex
	entities map[string]Entity
	cache    []byte
}

func (s *EntityStore) SerializedEntities() []byte {
	slog.Info("Serializing entries")
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

// Update multiple entities from json
func (s *EntityStore) SetFromJSON(content []byte, deleteExisting bool, ignoreErrors bool) error {

	// Marshal to simple map first so that one invalid format of
	// an entity does not stop importing the others
	rawEntries := make(map[string]json.RawMessage)
	err := json.Unmarshal(content, &rawEntries)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove all existing map entries
	if deleteExisting {
		for k := range s.entities {
			delete(s.entities, k)
		}
	}

	errList := make([]error, 0)
	// Set new values
	for k, v := range rawEntries {
		entity := &Entity{}
		if err := json.Unmarshal(v, entity); err != nil {
			errList = append(errList, err)
		} else {
			s.entities[k] = *entity
		}
	}

	if !ignoreErrors && len(errList) > 0 {
		return errors.Join(errList...)
	}

	// update cache
	b, err := json.Marshal(s.entities)
	if err != nil {
		return err
	}
	s.cache = b
	return nil
}

func (s *EntityStore) Set(key string, entity Entity) error {
	if key == "" {
		return fmt.Errorf("key can not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	slog.Info("Updating entity.", "key", key, "entity", entity)
	s.entities[key] = entity

	// update cache
	b, err := json.Marshal(s.entities)
	if err != nil {
		return err
	}
	s.cache = b
	return nil
}

func (s *EntityStore) Get(key string) (Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entity, ok := s.entities[key]

	if !ok {
		return Entity{}, fmt.Errorf("not found")
	}
	return entity, nil
}

type Service struct {
	Client        mqtt.Client
	APIClient     *APIClient
	Subscriptions map[string]byte
	Routes        []routes.Route
	EntityStore   *EntityStore
}

func (s *Service) GetVariables() string {
	return string(s.EntityStore.SerializedEntities())
}

type APIClient struct {
	*c8y.Client
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

func NewCumulocityClient(host string) *APIClient {
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
		Client: client,
	}
}

func (c *APIClient) SendRequest(ctx context.Context, options c8y.RequestOptions) (*c8y.Response, error) {
	resp, err := c.Client.SendRequest(ctx, options)
	if err != nil {
		return resp, err
	}
	return resp, err
}

var ErrNoMQTTClient = errors.New("no mqtt client")

func NewService(broker string, clientID string, cleanSession bool, httpEndpoint string, dryRun bool) (*Service, error) {
	parentTopic := "device/main//"
	tedgeTarget := fmt.Sprintf("te/device/main/service/%s", clientID)
	healthTopic := fmt.Sprintf("%s/status/health", tedgeTarget)
	opts := mqtt.NewClientOptions().SetClientID(clientID).AddBroker(broker).SetCleanSession(cleanSession).SetWill(healthTopic, `{"status":"down"}`, 1, true)
	client := mqtt.NewClient(opts)

	if !dryRun {
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			return nil, token.Error()
		}
	}

	service := &Service{
		Client:        client,
		Subscriptions: map[string]byte{},
		Routes:        []routes.Route{},
		EntityStore:   NewEntityStore(),
	}
	service.APIClient = NewCumulocityClient(httpEndpoint)
	serviceRegistrationMessage := map[string]any{
		"@type":   "service",
		"@parent": parentTopic,
	}
	msg, err := json.Marshal(serviceRegistrationMessage)
	if err != nil {
		return nil, err
	}
	client.Publish(tedgeTarget, 1, true, msg).Wait()
	client.Publish(healthTopic, 1, true, `{"status":"up"}`).Wait()
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
					for _, r := range spec.Routes {
						if !r.Disable {
							s.Routes = append(s.Routes, r)
						} else {
							slog.Info("Ignoring disabled route", "file", path, "route", r.Name)
						}
					}
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
		slog.Info("Adding mqtt route.", "topic", topic)
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
