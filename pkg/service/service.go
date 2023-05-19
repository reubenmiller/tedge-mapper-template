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
	opts := mqtt.NewClientOptions().SetClientID(clientID).AddBroker(broker).SetCleanSession(false).SetWill("tedge/health/tedge-mapper-template", `{"status":"down"}`, 1, true)
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &Service{
		Client:        client,
		Subscriptions: map[string]byte{},
	}, nil
}

func isYaml(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func (s *Service) ScanMappingFiles(dir string) []routes.Route {
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
			mappings = append(mappings, spec.Routes...)
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
	if token := s.Client.SubscribeMultiple(s.Subscriptions, nil); token.Wait() && token.Error() != nil {
		return fmt.Errorf("error subscribing to topic '%v': %v", s.Subscriptions, token.Error())
	}
	return nil
}
