package service

import (
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/reubenmiller/tedge-mapper-template/pkg/errors"
	"github.com/reubenmiller/tedge-mapper-template/pkg/routes"
	"github.com/reubenmiller/tedge-mapper-template/pkg/streamer"
	"github.com/stretchr/testify/assert"
)

func Test_RemoveContext(t *testing.T) {
	route := routes.Route{
		Name:   "Recursive route",
		Topics: []string{"in"},
		Template: routes.Template{
			Type: "jsonnet",
			Value: `
				{
					topic: 'in',
					message: {
						value: 1
					},
					context: false,
				}
			`,
		},
	}

	testcases := []struct {
		Name          string
		Route         routes.Route
		Topic         string
		Template      string
		Message       string
		ExpectedMsg   string
		ExpectedError error
	}{
		{
			Route:   route,
			Topic:   "in",
			Message: `{}`,
			Template: `
				{
					topic: 'out',
					message: {
						value: 1
					},
					context: false,
				}
			`,
			ExpectedMsg: `
				{
					"value": 1
				}
			`,
			// ExpectedError: errors.ErrRecursiveLevelExceeded,
		},
	}

	for _, c := range testcases {
		route.Template = c.Route.Template
		handler := NewStreamFactory(nil, c.Route, 2, 0)
		out, err := handler(c.Topic, c.Message)
		assert.NoError(t, err)
		assert.JSONEq(t, c.ExpectedMsg, out.MessageString())
	}

}

func Test_MaxDepthLimit(t *testing.T) {
	recursiveRoute := routes.Route{
		Name:   "Recursive route",
		Topics: []string{"in"},
		Template: routes.Template{
			Type: "jsonnet",
			Value: heredoc.Doc(`
				{
					topic: 'in',
					message: {
						value: 1
					}
				}
			`),
		},
	}
	nonRecursiveRoute := routes.Route{
		Name:   "Non recursive route",
		Topics: []string{"in"},
		Template: routes.Template{
			Type: "jsonnet",
			Value: heredoc.Doc(`
				{
					topic: 'out',
					message: {
						value: 1
					}
				}
			`),
		},
	}

	testcases := []struct {
		Name          string
		Route         routes.Route
		Topic         string
		Message       string
		Depth         int
		ExpectedIter  int
		ExpectedError error
	}{
		{
			Route:         recursiveRoute,
			Topic:         "in",
			Message:       `{}`,
			Depth:         1,
			ExpectedIter:  1,
			ExpectedError: errors.ErrRecursiveLevelExceeded,
		},
		{
			Route:         recursiveRoute,
			Topic:         "in",
			Message:       `{}`,
			Depth:         2,
			ExpectedIter:  2,
			ExpectedError: errors.ErrRecursiveLevelExceeded,
		},
		{
			Route:         nonRecursiveRoute,
			Topic:         "in",
			Message:       `{}`,
			Depth:         2,
			ExpectedIter:  1,
			ExpectedError: nil,
		},
	}

	for _, c := range testcases {
		handler := NewStreamFactory(nil, c.Route, c.Depth, 0)
		msg := &streamer.OutputMessage{
			Topic:   c.Topic,
			Message: c.Message,
		}

		var err error
		i := 0
		for i < 5 {
			if !c.Route.Match(msg.Topic) {
				break
			}
			msg, err = handler(msg.Topic, msg.MessageString())
			if err != nil {
				break
			}
			i++
		}

		assert.Equal(t, c.ExpectedIter, i, c)

		if c.ExpectedError != nil {
			assert.ErrorIs(t, err, c.ExpectedError, c)
		} else {
			assert.NoError(t, err, c)
		}
	}
}
