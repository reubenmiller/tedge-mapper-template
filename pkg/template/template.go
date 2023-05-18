package template

type Templater interface {
	Execute(string, string) (string, error)
	Debug() bool
}
