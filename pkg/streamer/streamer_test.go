package streamer

import (
	"testing"

	"github.com/reubenmiller/tedge-mapper-template/pkg/jsonnet"
	"github.com/stretchr/testify/assert"
)

func Test_StreamerProcessJsonnet(t *testing.T) {
	testcases := []struct {
		Topic             string
		Template          string
		Message           string
		ExpectedTopic     string
		ExpectedMsgIsText bool
		ExpectedMsg       string
		ExpectedEnd       bool
		ExpectedSkip      bool
		ExpectedErr       error
	}{
		{
			Topic:    "in",
			Template: `{}`,
			Message:  `{}`,
			ExpectedMsg: `
				{
					"_ctx":{
						"lvl": 1
					}
				}
			`,
			ExpectedErr: nil,
		},
		{
			Topic: "in",
			Template: `
				{
					message: {
						t: topic,
					},
				}
			`,
			Message: `{}`,
			ExpectedMsg: `
				{
					"t": "in",
					"_ctx":{
						"lvl": 1
					}
				}
			`,
		},
		{
			Topic: "in",
			Template: `
				{
					topic: 'fixed',
					message: {},
				}
			`,
			Message:       `{}`,
			ExpectedTopic: "fixed",
			ExpectedMsg: `
				{
					"_ctx":{
						"lvl": 1
					}
				}
			`,
		},
		{
			Topic: "in",
			Template: `
				{
					topic: 'fixed',
					message: {},
				}
			`,
			Message:       `{}`,
			ExpectedTopic: "fixed",
			ExpectedMsg: `
				{
					"_ctx":{
						"lvl": 1
					}
				}
			`,
		},
		{
			Topic: "in",
			Template: `
				{
					topic: 'fixed',
					message: {
						_ctx: {
							otherdata: {
								disable: true,
							},
						},
					},
				}
			`,
			Message: `
				{
					"_ctx": {
						"lvl": 2
					}
				}
			`,
			ExpectedTopic: "fixed",
			ExpectedMsg: `
				{
					"_ctx":{
						"lvl": 3,
						"otherdata": {
							"disable": true
						}
					}
				}
			`,
		},
		{
			Topic: "in",
			Template: `
				{
					topic: 'fixed',
					raw_message: '201,custom template',
					message: {
						_ctx: {
							otherdata: {
								disable: true,
							},
						},
					},
				}
			`,
			Message:           `{}`,
			ExpectedTopic:     "fixed",
			ExpectedMsgIsText: true,
			ExpectedMsg:       `201,custom template`,
		},
	}

	for _, c := range testcases {
		engine := jsonnet.NewEngine(c.Template)
		stream := NewStreamer(engine)
		out, err := stream.Process(c.Topic, c.Message, "")

		if c.ExpectedErr != nil {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}

		if c.ExpectedMsg != "" {
			if c.ExpectedMsgIsText {
				assert.Equal(t, c.ExpectedMsg, out.MessageString())
			} else {
				assert.JSONEq(t, c.ExpectedMsg, out.MessageString())
			}
		}
		assert.Equal(t, c.ExpectedTopic, out.Topic)
		assert.Equal(t, c.ExpectedEnd, out.End)
		assert.Equal(t, c.ExpectedSkip, out.Skip)
	}
}

func Test_StreamerProcessJsonnetWithUpdateMessages(t *testing.T) {
	testcases := []struct {
		Topic             string
		Template          string
		Message           string
		ExpectedTopic     string
		ExpectedMsgIsText bool
		ExpectedMsg       string
		ExpectedEnd       bool
		ExpectedSkip      bool
		ExpectedErr       error
		ExpectedUpdates   []struct {
			IsText  bool
			Topic   string
			Payload string
		}
	}{
		{
			Topic: "in",
			Template: `
				{
					topic: 'fixed',
					raw_message: '201,custom template',
					updates: [
						{topic: 'other/topic1', message: '201,do something'},
						{
							topic: 'other/topic2',
							message: {
								text: 'Complex message',
							}
						},
					]
				}
			`,
			Message:           `{}`,
			ExpectedTopic:     "fixed",
			ExpectedMsgIsText: true,
			ExpectedMsg:       `201,custom template`,
			ExpectedUpdates: []struct {
				IsText  bool
				Topic   string
				Payload string
			}{
				{
					IsText:  true,
					Topic:   "other/topic1",
					Payload: `201,do something`,
				},
				{
					IsText: false,
					Topic:  "other/topic1",
					Payload: `{
						"text": "Complex message"
					}`,
				},
			},
		},
	}

	for _, c := range testcases {
		engine := jsonnet.NewEngine(c.Template)
		stream := NewStreamer(engine)
		out, err := stream.Process(c.Topic, c.Message, "")

		if c.ExpectedErr != nil {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}

		assert.Len(t, out.Updates, len(c.ExpectedUpdates))
		for i, expectedUpdate := range c.ExpectedUpdates {
			if expectedUpdate.IsText {
				assert.Equal(t, expectedUpdate.Payload, out.Updates[i].MessageString())
			} else {
				assert.JSONEq(t, expectedUpdate.Payload, out.Updates[i].MessageString())
			}
		}

		if c.ExpectedMsg != "" {
			if c.ExpectedMsgIsText {
				assert.Equal(t, c.ExpectedMsg, out.MessageString())
			} else {
				assert.JSONEq(t, c.ExpectedMsg, out.MessageString())
			}
		}
		assert.Equal(t, c.ExpectedTopic, out.Topic)
		assert.Equal(t, c.ExpectedEnd, out.End)
		assert.Equal(t, c.ExpectedSkip, out.Skip)
	}
}
