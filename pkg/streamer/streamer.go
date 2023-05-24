package streamer

import (
	"encoding/json"

	"github.com/reubenmiller/tedge-mapper-template/pkg/template"
)

type Streamer struct {
	Engine template.Templater
}

type SimpleOutputMessage struct {
	Topic   string `json:"topic"`
	Message any    `json:"message"`
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

// TODO: Come up with a better name rather the 'Updates' field
type OutputMessage struct {
	Topic      string                `json:"topic"`
	Message    any                   `json:"message,omitempty"`
	RawMessage string                `json:"raw_message,omitempty"`
	Updates    []SimpleOutputMessage `json:"updates"`
	Skip       bool                  `json:"skip"`
	End        bool                  `json:"end"`
	Context    *bool                 `json:"context,omitempty"`
}

func NewStreamer(engine template.Templater) *Streamer {
	return &Streamer{
		Engine: engine,
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

func (s *Streamer) Process(topic, message string) (*OutputMessage, error) {
	out, err := s.Engine.Execute(topic, message)
	if err != nil {
		return nil, err
	}

	sm := &OutputMessage{}
	if err := json.Unmarshal([]byte(out), sm); err != nil {
		return nil, err
	}

	return sm, nil
}
