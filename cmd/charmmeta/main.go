package main

import (
	"bufio"
	"fmt"
	"launchpad.net/juju-core/charm"
	"os"
	"text/template"
)

var usage = `
usage: charmmeta metaTemplate [charmdir...]

The charmmeta command runs the given Go
template on the charm metadata in the charms
found in all the listed directories and prints
the results to the standard output.

Documentation for the Meta type is here:
http://godoc.org/launchpad.net/juju-core/charm#Meta

Documentation for the Go template package is here:
http://golang.org/pkg/text/template

For example, to print the relation names used in a charm:

charmmeta '{{range .Provides}}{{.Name}}
{{end}}{{range .Requires}}{{.Name}}
{{end}}{{range .Peers}}{{.Name}}
{{end}}' some/charm/dir
`[1:]

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	tmpl, err := template.New("").Parse(args[0])
	if err != nil {
		fatalf("cannot parse template: %v", err)
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for _, dir := range os.Args[2:] {
		dir, err := charm.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "charmmeta: cannot read %q: %v\n", dir, err)
			continue
		}
		err = tmpl.Execute(w, dir.Meta())
		if err != nil {
			w.Flush()
			fatalf("template execute failed: %v", err)
		}
	}
}

func fatalf(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "charmmeta: %s\n", fmt.Sprintf(f, a...))
	os.Exit(1)
}
