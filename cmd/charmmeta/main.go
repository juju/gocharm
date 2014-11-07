package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"gopkg.in/juju/charm.v4"
)

var usage = `
usage: charmmeta [-r] metaTemplate [charmdir...]

The charmmeta command runs the given Go
template on the charm metadata in the charms
found in all the listed directories and prints
the results to the standard output.

If the -r flag is specified, the directories
are recursively searched for charms.

Documentation for the Meta type is here:
http://godoc.org/gopkg.in/juju/charm.v4#Meta

Documentation for the Go template package is here:
http://golang.org/pkg/text/template

For example, to print the relation names used in a charm:

charmmeta '{{range .Provides}}{{.Name}}
{{end}}{{range .Requires}}{{.Name}}
{{end}}{{range .Peers}}{{.Name}}
{{end}}' some/charm/dir
`[1:]

var recurse = flag.Bool("r", false, "recursively search directories")

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
	}
	args := flag.Args()[1:]
	tmpl, err := template.New("").Parse(flag.Arg(0))
	if err != nil {
		fatalf("cannot parse template: %v", err)
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	dirCh := make(chan string)
	go findCharms(dirCh, args)
	for path := range dirCh {
		dir, err := charm.ReadCharmDir(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "charmmeta: cannot read %q: %v\n", path, err)
			continue
		}
		err = tmpl.Execute(w, dir.Meta())
		if err != nil {
			w.Flush()
			fatalf("template execute failed: %v", err)
		}
	}
}

func findCharms(dirCh chan<- string, dirs []string) {
	defer close(dirCh)
	if !*recurse {
		for _, d := range dirs {
			dirCh <- d
		}
		return
	}
	for _, d := range dirs {
		filepath.Walk(d, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// TODO truncate recursion if we find a metadata file
			dir, file := filepath.Split(path)
			if file == "metadata.yaml" {
				dirCh <- dir
			}
			return nil
		})
	}
}

func fatalf(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "charmmeta: %s\n", fmt.Sprintf(f, a...))
	os.Exit(1)
}
