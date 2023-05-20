package jsonnet

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	_jsonnet "github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/teris-io/shortid"
)

var HeaderMarker = "\n###\n"

type JsonnetEngine struct {
	vm       *_jsonnet.VM
	template string
	Options  EngineOptions
}

type EngineOptions struct {
	Debug bool
	Meta  any
}

type TemplateOption func(*EngineOptions) *EngineOptions

func WithDebug(v bool) TemplateOption {
	return func(opt *EngineOptions) *EngineOptions {
		opt.Debug = v
		return opt
	}
}

func WithMetaData(v any) TemplateOption {
	return func(opt *EngineOptions) *EngineOptions {
		opt.Meta = v
		return opt
	}
}

func NewEngine(tmpl string, opts ...TemplateOption) *JsonnetEngine {
	vm := _jsonnet.MakeVM()
	engine := &JsonnetEngine{
		vm: vm,
	}

	config := &EngineOptions{}
	for _, opt := range opts {
		opt(config)
	}

	sb := strings.Builder{}
	metaD, err := json.Marshal(config.Meta)
	if err == nil {
		if strings.HasPrefix(string(metaD), "{") && strings.HasSuffix(string(metaD), "}") {
			sb.WriteString(fmt.Sprintf("local meta = %s;\n", metaD))
		} else if strings.HasPrefix(string(metaD), "{") && strings.HasPrefix(string(metaD), "}") {
			sb.WriteString(fmt.Sprintf("local meta = %s;\n", metaD))
		} else {
			sb.WriteString(fmt.Sprintf("local meta = '%s';\n", metaD))
		}
	} else {
		sb.WriteString("local meta = {};\n")
	}
	sb.WriteString("local _ = {Now: function() std.native('Now')(), ReplacePattern: function(s, from, to='') std.native('ReplacePattern')(s, from, to),ID: function() std.native('ID')(),};\n")

	sb.WriteString(removeHeader(tmpl))
	engine.template = sb.String()
	engine.Options = *config

	engine.addFunctions()
	return engine

}

func getStringParameter(parameters []interface{}, i int) string {
	if len(parameters) > 0 && i < len(parameters) {
		return fmt.Sprintf("%v", parameters[i])
	}
	return ""
}

func (e *JsonnetEngine) Debug() bool {
	return e.Options.Debug
}

func (e *JsonnetEngine) addFunctions() {
	e.vm.NativeFunction(&_jsonnet.NativeFunction{
		Name: "Now",
		Func: func(parameters []interface{}) (interface{}, error) {
			return time.Now().Format(time.RFC3339Nano), nil
		},
	})

	e.vm.NativeFunction(&_jsonnet.NativeFunction{
		Name: "NowNano",
		Func: func(parameters []interface{}) (interface{}, error) {
			return time.Now().Format(time.RFC3339Nano), nil
		},
	})

	e.vm.NativeFunction(&_jsonnet.NativeFunction{
		Name:   "ReplacePattern",
		Params: ast.Identifiers{"value", "from", "to"},
		Func: func(parameters []interface{}) (interface{}, error) {
			value := getStringParameter(parameters, 0)
			from := getStringParameter(parameters, 1)
			to := getStringParameter(parameters, 2)

			pattern, err := regexp.Compile(from)
			if err != nil {
				return "", err
			}
			return pattern.ReplaceAllString(value, to), nil
		},
	})
	e.vm.NativeFunction(&_jsonnet.NativeFunction{
		Name: "ID",
		Func: func(parameters []interface{}) (interface{}, error) {
			v, err := shortid.Generate()
			if err != nil {
				return "", err
			}
			return v, nil
		},
	})
}

// Remove the header as this is just used to locally test the template
func removeHeader(tmpl string) string {
	if i := strings.Index(tmpl, HeaderMarker); i != -1 {
		return tmpl[i:]
	}
	return tmpl
}

func (e *JsonnetEngine) Execute(topic, input string) (string, error) {
	e.vm.ExtVar("message", "do something")
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("local topic = '%s';\n", topic))

	inputIsObject := json.Valid([]byte(input))

	if inputIsObject {
		sb.WriteString(fmt.Sprintf("local _input = %s;\n", input))
	} else {
		sb.WriteString(fmt.Sprintf("local _input = '%s';\n", input))
	}

	sb.WriteString("local message = if std.isObject(_input) then _input + {_ctx:: null} else _input;\n")
	if inputIsObject {
		sb.WriteString("local ctx = {lvl:0} + std.get(_input, '_ctx', {});\n")
	} else {
		sb.WriteString("local ctx = {lvl:0};\n")
	}
	sb.WriteString(e.template)
	sb.WriteString(" + {message+: {_ctx+: ctx + {lvl: std.get(ctx, 'lvl', 0) + 1}}}")
	output, err := e.vm.EvaluateAnonymousSnippet("file", sb.String())

	if e.Debug() {
		fmt.Printf("Template: \n\n%s\n\n", sb.String())
	}
	return output, err
}
