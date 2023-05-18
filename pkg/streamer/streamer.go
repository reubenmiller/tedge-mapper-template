package streamer

import (
	"encoding/json"

	"github.com/reubenmiller/tedge-mapper-template/pkg/template"
)

type Streamer struct {
	Engine template.Templater
}

type OutputMessage struct {
	Topic      string `json:"topic"`
	Message    any    `json:"message,omitempty"`
	RawMessage string `json:"raw_message,omitempty"`
	Skip       bool   `json:"skip"`
	End        bool   `json:"end"`
}

func NewStreamer(engine template.Templater) *Streamer {
	return &Streamer{
		Engine: engine,
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
