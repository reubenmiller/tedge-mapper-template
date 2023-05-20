package routes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCSVPreprocessor(t *testing.T) {
	route := &Route{
		PreProcessor: &PreProcessor{
			Type: "csv",
			Fields: []string{
				"id",
				"serial",
				"software.*.name",
				"software.*.version",
				"software.*.url",
			},
		},
	}

	if err := route.PreparePreProcessor(); err != nil {
		t.Errorf("Prepare preprocessor failed. got=%s", err)
	}

	out, err := route.ExecutePreprocessor(`510,mydevice,"hello world 1",1.0.0,http://hello.world.com,"hello world 2",2.0.0,http://hello.world2.com`)
	if err != nil {
		t.Errorf("Expected no error. got=%s", err)
	}

	newMessage := struct {
		ID       string `json:"id"`
		Serial   string `json:"serial"`
		Software []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Url     string `json:"url"`
		} `json:"software"`
	}{}

	err = json.Unmarshal([]byte(out), &newMessage)
	if err != nil {
		t.Errorf("Expected output to be json. got=%s", err)
	}

	assert.Equal(t, newMessage.ID, "510")
	assert.Equal(t, newMessage.Serial, "mydevice")
	assert.Len(t, newMessage.Software, 2)
	assert.Equal(t, newMessage.Software[0].Name, "hello world 1")
	assert.Equal(t, newMessage.Software[0].Version, "1.0.0")
	assert.Equal(t, newMessage.Software[0].Url, "http://hello.world.com")
	assert.Equal(t, newMessage.Software[1].Name, "hello world 2")
	assert.Equal(t, newMessage.Software[1].Version, "2.0.0")
	assert.Equal(t, newMessage.Software[1].Url, "http://hello.world2.com")
}

func Test_RoutePatternMatch(t *testing.T) {
	testcases := []struct {
		TopicPattern string
		Topic        string
		Expected     bool
	}{
		{
			TopicPattern: "in",
			Topic:        "in",
			Expected:     true,
		},
		{
			TopicPattern: "in",
			Topic:        "out",
			Expected:     false,
		},
		{
			TopicPattern: "input/+/something",
			Topic:        "input/value/something",
			Expected:     true,
		},
		{
			TopicPattern: "input/+/something",
			Topic:        "input/value/something/else",
			Expected:     false,
		},
		{
			TopicPattern: "input/+/something/#",
			Topic:        "input/value/something/else",
			Expected:     true,
		},
		{
			TopicPattern: "input/+/+/+",
			Topic:        "input/one/two/three",
			Expected:     true,
		},
	}

	for _, c := range testcases {
		route := Route{
			Topic: c.TopicPattern,
		}
		assert.Equal(t, c.Expected, route.Match(c.Topic))
	}
}
