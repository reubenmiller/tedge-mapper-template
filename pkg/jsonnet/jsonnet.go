package jsonnet

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	_jsonnet "github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/teris-io/shortid"
	"github.com/tidwall/gjson"
)

var HeaderMarker = "\n###\n"

type JsonnetEngine struct {
	vm       *_jsonnet.VM
	template string
	Options  EngineOptions
}

type EngineOptions struct {
	Debug        bool
	DryRun       bool
	UseColor     bool
	LibraryPaths []string
	Meta         any
}

type TemplateOption func(*EngineOptions) *EngineOptions

func WithDebug(v bool) TemplateOption {
	return func(opt *EngineOptions) *EngineOptions {
		opt.Debug = v
		return opt
	}
}

func WithDryRun(v bool) TemplateOption {
	return func(opt *EngineOptions) *EngineOptions {
		opt.DryRun = v
		return opt
	}
}

func WithColorStackTrace(v bool) TemplateOption {
	return func(opt *EngineOptions) *EngineOptions {
		opt.UseColor = v
		return opt
	}
}

func WithMetaData(v any) TemplateOption {
	return func(opt *EngineOptions) *EngineOptions {
		opt.Meta = v
		return opt
	}
}

func WithLibraryPaths(paths ...string) TemplateOption {
	return func(opt *EngineOptions) *EngineOptions {
		opt.LibraryPaths = paths
		return opt
	}
}

type vmConfig struct {
	evalJpath []string
}

func makeVMConfig() vmConfig {
	return vmConfig{
		evalJpath: []string{},
	}
}

func NewJsonnetVM(useColor bool, paths ...string) *_jsonnet.VM {

	vm := _jsonnet.MakeVM()

	if useColor {
		vm.ErrorFormatter.SetColorFormatter(color.New(color.FgRed).Fprintf)
	}

	vmConfig := makeVMConfig()
	for i := len(paths) - 1; i >= 0; i-- {
		slog.Info("Adding jsonnet path.", "path", paths[i])
		vmConfig.evalJpath = append(vmConfig.evalJpath, paths[i])
	}

	vm.Importer(&_jsonnet.FileImporter{
		JPaths: vmConfig.evalJpath,
	})

	return vm
}

func NewEngine(tmpl string, opts ...TemplateOption) *JsonnetEngine {
	engine := &JsonnetEngine{}
	config := &EngineOptions{}
	for _, opt := range opts {
		opt(config)
	}

	engine.vm = NewJsonnetVM(config.UseColor, config.LibraryPaths...)

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
	sb.WriteString("local _ = {Now: function() std.native('Now')(), Get: function(o, key, defaultValue=null) std.native('Get')(o, key, defaultValue), ReplacePattern: function(s, from, to='') std.native('ReplacePattern')(s, from, to),ID: function() std.native('ID')(),};\n")

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

func getParameter(parameters []interface{}, i int) any {
	if len(parameters) > 0 && i < len(parameters) {
		return parameters[i]
	}
	return nil
}

func (e *JsonnetEngine) Debug() bool {
	return e.Options.Debug
}

func (e *JsonnetEngine) DryRun() bool {
	return e.Options.DryRun
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

	e.vm.NativeFunction(&_jsonnet.NativeFunction{
		Name:   "Get",
		Params: ast.Identifiers{"obj", "prop", "default"},
		Func: func(parameters []interface{}) (interface{}, error) {
			obj := getParameter(parameters, 0)
			key := getStringParameter(parameters, 1)
			defaultValue := getParameter(parameters, 2)

			// TODO: Try to avoid converting from map to json again
			objB, err := json.Marshal(obj)
			if err != nil {
				return defaultValue, nil
			}

			if v := gjson.GetBytes(objB, key); v.Exists() {
				return v.Value(), nil
			}
			return defaultValue, nil
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

func (e *JsonnetEngine) Execute(topic, input string, variables string) (string, error) {
	e.vm.ExtVar("message", "do something")
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("local topic = '%s';\n", topic))

	inputIsObject := json.Valid([]byte(input))

	if inputIsObject {
		sb.WriteString(fmt.Sprintf("local _input = %s;\n", input))
	} else {
		sb.WriteString(fmt.Sprintf("local _input = '%s';\n", input))
	}

	slog.Info("json template.", "variables", variables)
	if variables == "" {
		variables = "{}"
	}
	sb.WriteString(fmt.Sprintf("local variables = %s;\n", variables))

	sb.WriteString("local message = if std.isObject(_input) then _input + {_ctx:: null} else _input;\n")
	if inputIsObject {
		sb.WriteString("local ctx = {lvl:0} + if std.isObject(_input) then std.get(_input, '_ctx', {}) else {};\n")
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
