package routes

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"
)

type Specification struct {
	Disable bool    `yaml:"disable"`
	Routes  []Route `yaml:"routes"`
}

type Route struct {
	Name         string        `yaml:"name"`
	Topics       []string      `yaml:"topics"`
	Skip         bool          `yaml:"skip"`
	Template     Template      `yaml:"template"`
	PreProcessor *PreProcessor `yaml:"preprocessor,omitempty"`
}

type Template struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
	Path  string `yaml:"path"`
}

type PreProcessor struct {
	Type           string   `yaml:"type"`
	Fields         []string `yaml:"fields"`
	fixedFields    []string
	variableFields []string
}

func (r *Route) DisplayTopics(sep ...string) string {
	delim := ", "
	if len(sep) > 0 {
		delim = sep[0]
	}
	return strings.Join(r.Topics, delim)
}

func (r *Route) PreparePreProcessor() error {
	if r.PreProcessor == nil {
		return nil
	}
	pp := r.PreProcessor
	fixed := make([]string, 0)
	variableFields := make([]string, 0)
	hasVariable := false

	for _, field := range pp.Fields {
		if strings.Contains(field, "*") {
			hasVariable = true
			variableFields = append(variableFields, field)
		} else {
			fixed = append(fixed, field)
		}
	}

	pp.fixedFields = fixed
	if hasVariable {
		pp.variableFields = variableFields
	}
	return nil
}

type VariableFields struct {
	Fields []string `yaml:"fields"`
}

func (r *Route) HasPreprocessor() bool {
	return r.PreProcessor != nil
}

func Parse(r io.Reader) (*Specification, error) {
	spec := &Specification{}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// helpers

// match takes a slice of strings which represent the route being tested having been split on '/'
// separators, and a slice of strings representing the topic string in the published message, similarly
// split.
// The function determines if the topic string matches the route according to the MQTT topic rules
// and returns a boolean of the outcome
func match(route []string, topic []string) bool {
	if len(route) == 0 {
		return len(topic) == 0
	}

	if len(topic) == 0 {
		return route[0] == "#"
	}

	if route[0] == "#" {
		return true
	}

	if (route[0] == "+") || (route[0] == topic[0]) {
		return match(route[1:], topic[1:])
	}
	return false
}

func routeIncludesTopic(route, topic string) bool {
	return match(routeSplit(route), strings.Split(topic, "/"))
}

// removes $share and sharename when splitting the route to allow
// shared subscription routes to correctly match the topic
func routeSplit(route string) []string {
	var result []string
	if strings.HasPrefix(route, "$share") {
		result = strings.Split(route, "/")[2:]
	} else {
		result = strings.Split(route, "/")
	}
	return result
}

// match takes the topic string of the published message and does a basic compare to the
// string of the current Route, if they match it returns true
func (r *Route) Match(topic string) bool {
	for _, routeTopic := range r.Topics {
		if routeTopic == topic || routeIncludesTopic(routeTopic, topic) {
			return true
		}
	}
	return false
}

func (r *Route) ExecutePreprocessor(in string) (string, error) {
	if !r.HasPreprocessor() {
		return in, nil
	}

	inR := strings.NewReader(in)
	csvReader := csv.NewReader(inR)

	outS := "{}"

	fields, err := csvReader.Read()
	if err == io.EOF {
		return "", nil
	}

	if err != nil {
		// ignore invalid csv
		return "", nil
	}

	if len(fields) >= len(r.PreProcessor.fixedFields) {
		for i, name := range r.PreProcessor.fixedFields {
			if name != "" && name != "-" {
				if s, err := sjson.Set(outS, name, fields[i]); err == nil {
					outS = s
				}
			}
		}
	}

	if len(r.PreProcessor.variableFields) > 0 {

		startIndex := len(r.PreProcessor.fixedFields)

		if len(fields) >= startIndex+len(r.PreProcessor.variableFields) {
			i := startIndex
			j := 0
			chunkSize := len(r.PreProcessor.variableFields)

			for i < len(fields) {
				name := r.PreProcessor.variableFields[j%chunkSize]
				if name != "" && name != "-" {
					curField := strings.Replace(name, "*", fmt.Sprintf("%d", j/chunkSize), 1)
					if s, err := sjson.Set(outS, curField, fields[i]); err == nil {
						outS = s
					}
				}
				j++
				i++

			}
		}
	}

	// Include the raw message in the payload (so it can be referenced for other operations)
	if s, err := sjson.Set(outS, "payload", in); err == nil {
		outS = s
	}

	return outS, nil
}
