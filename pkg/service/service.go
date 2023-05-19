package service

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/reubenmiller/tedge-mapper-template/pkg/routes"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"golang.org/x/exp/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

type Service struct {
	Client        mqtt.Client
	Subscriptions map[string]byte
}

func NewService(broker string, clientID string) (*Service, error) {
	healthTopic := fmt.Sprintf("tedge/health/%s", clientID)
	opts := mqtt.NewClientOptions().SetClientID(clientID).AddBroker(broker).SetCleanSession(false).SetWill(healthTopic, `{"status":"down"}`, 1, true)
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}
	client.Publish(healthTopic, 1, true, `{"status":"up"}`)

	return &Service{
		Client:        client,
		Subscriptions: map[string]byte{},
	}, nil
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

func (s *Service) Register(topic string, qos byte, handler MessageHandler) error {
	handlerWrapper := func(c mqtt.Client, m mqtt.Message) {
		handler(m.Topic(), string(m.Payload()))
	}
	s.Subscriptions[topic] = qos
	s.Client.AddRoute(topic, handlerWrapper)
	return nil
}

func (s *Service) StartSubscriptions() error {
	if len(s.Subscriptions) == 0 {
		slog.Warn("No routes were detected, so nothing to subscribe to")
		return nil
	}
	if token := s.Client.SubscribeMultiple(s.Subscriptions, nil); token.Wait() && token.Error() != nil {
		return fmt.Errorf("error subscribing to topic '%v': %v", s.Subscriptions, token.Error())
	}
	return nil
}
