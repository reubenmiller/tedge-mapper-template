package streamer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/reubenmiller/tedge-mapper-template/pkg/template"
)

type Streamer struct {
	Engine template.Templater
}

type SimpleOutputMessage struct {
	Topic   string  `json:"topic"`
	Message any     `json:"message"`
	Skip    bool    `json:"skip"`
	Delay   float32 `json:"delay"`
	Retain  bool    `json:"retain"`
	QoS     float32 `json:"qos"`
}

func (m *SimpleOutputMessage) MessageString() string {
	switch v := m.Message.(type) {
	case string:
		return v
	default:
		out, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(out)
	}
}

func (m *SimpleOutputMessage) GetQoS() byte {
	switch m.QoS {
	case 1:
		return 1
	case 2:
		return 2
	default:
		return 0
	}
}

type RestRequest struct {
	Host   string `json:"host,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

func (r *RestRequest) Validate() error {
	if r.Path == "" {
		return fmt.Errorf("path is empty")
	}
	if r.Method == "" {
		return fmt.Errorf("method is empty")
	}
	method := strings.ToUpper(r.Method)
	if method != "PUT" && method != "POST" && method != "GET" {
		return fmt.Errorf("method not allowed. only POST, PUT and GET methods are supported. got=%s", r.Method)
	}
	return nil
}

// TODO: Come up with a better name rather the 'Updates' field
type OutputMessage struct {
	Topic      string                `json:"topic"`
	Message    any                   `json:"message,omitempty"`
	RawMessage string                `json:"raw_message,omitempty"`
	Delay      float32               `json:"delay"`
	Updates    []SimpleOutputMessage `json:"updates"`
	API        *RestRequest          `json:"api,omitempty"`
	Skip       bool                  `json:"skip"`
	End        bool                  `json:"end"`
	Context    *bool                 `json:"context,omitempty"`
	Retain     bool                  `json:"retain,omitempty"`
	QoS        float32               `json:"qos,omitempty"`
}

func NewStreamer(engine template.Templater) *Streamer {
	return &Streamer{
		Engine: engine,
	}
}

func (m *OutputMessage) IsAPIRequest() bool {
	return m.API != nil
}
func (m *OutputMessage) IsMQTTMessage() bool {
	return m.Topic != ""
}
func (m *OutputMessage) GetType() string {
	if m.IsAPIRequest() {
		return "api"
	}
	return "mqtt"
}

func (m *OutputMessage) GetQoS() byte {
	switch m.QoS {
	case 1:
		return 1
	case 2:
		return 2
	default:
		return 0
	}
}

func (m *OutputMessage) DisableContext() bool {
	if m.Context == nil {
		return false
	}
	return !*m.Context
}

func (m *OutputMessage) MessageString() string {
	if m.RawMessage != "" {
		return m.RawMessage
	}

	switch v := m.Message.(type) {
	case string:
		return v
	default:
		out, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(out)
	}
}

func (s *Streamer) Process(topic, message string, variables string) (*OutputMessage, error) {
	out, err := s.Engine.Execute(topic, message, variables)
	if err != nil {
		return nil, err
	}

	sm := &OutputMessage{}
	if err := json.Unmarshal([]byte(out), sm); err != nil {
		return nil, err
	}

	return sm, nil
}
