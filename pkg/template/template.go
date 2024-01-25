package template

type Templater interface {
	Execute(string, string, string) (string, error)
	Debug() bool
	DryRun() bool
}
