// Gocharm processes one or more Juju charms with hooks written in Go.
// All hooks are compiled into a single Go executable, bin/runhook, implemented by
// the runhook package, which must be implemented in the src/runhook
// directory inside the charm. It ignores charms without that directory.
// Note that it compiles the Go executable in cross-compilation mode,
// so cgo-based packages will not work and the resulting charm will
// only work on linux-amd64-based images.
//
// Gocharm increments the revision number of any charms that it
// compiles.
//
// The runhook package must implement a RegisterHooks function which
// must register any hooks required by calling hook.RegisterHook (see
// gopkg.in/juju-utils.v0/hook).
//
// Gocharm runs runhook.RegisterHooks locally to find out what hooks are
// registered, and automatically writes stubs in the hooks directory.
// When the charm is deployed, these will call the runhook executable
// and arrange for registered hook functions to be called. It takes care
// not to overwrite any hooks that may contain custom user changes - it
// might be necessary to remove or change these by hand if gocharm
// prints a warning message about this.
//
// The runhook package is compiled with the charm directory inserted
// before the start of GOPATH, meaning that charm-specific packages can
// be defined and used from runhook.
//
// TODO allow a mode that does not compile locally, installing golang
// on the remote node and compiling the code there.
//
// TODO add -clean flag.
//
// TODO use godeps to freeze dependencies into the charm.
//
// TODO examples.
//
// TODO validate metadata against actual registered hooks.
// If there's a hook registered against a relation that's
// not declared, or there's a hook declared but no hooks are
// registered for it, return an error.
//
// TODO allow code to register relations, and either
// validate against charm metadata or actually modify the
// charm metadata in place (would require a charm.WriteMeta
// function and users might not like that, as it may mess up formatting)
// package hook; func (r *Registry) RegisterRelation(rel charm.Relation)
//
// TODO allow code to register configuration options,
// and
// TODO allow install and start hooks to be omitted if desired - generate them
// automatically if necessary.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"
)

var repo = flag.String("repo", "", "charm repo directory (defaults to $JUJU_REPOSITORY)")
var test = flag.Bool("test", false, "run tests before building")
var verbose = flag.Bool("v", false, "print information about charms being built")
var all = flag.Bool("all", false, "compile all charms under $JUJU_REPOSITORY")

var exitCode = 0

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: gocharm [flags] [charm...]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	// TODO accept charm name arguments on the command line
	// to restrict the build to those charms only.
	if *repo == "" {
		if *repo = os.Getenv("JUJU_REPOSITORY"); *repo == "" {
			fatalf("no charm repo directory specified")
		}
	}
	var dirs []*charm.CharmDir
	if flag.NArg() > 0 {
		dirs = readNamedCharms(flag.Args())
	} else if *all {
		dirs = readAllCharms()
	} else {
		dir, err := charmFromCurrentDir()
		if err != nil {
			fatalf("%v", err)
		}
		dirs = append(dirs, dir)
	}
	if len(dirs) == 0 {
		fatalf("no charms found")
	}
	for _, dir := range dirs {
		processCharm(dir)
	}
	os.Exit(exitCode)
}

func appendCharmDir(dirs []*charm.CharmDir, path string) []*charm.CharmDir {
	dir, err := charm.ReadCharmDir(path)
	if err != nil {
		errorf("cannot read %q: %v", path, err)
		return dirs
	}
	return append(dirs, dir)
}

func readAllCharms() []*charm.CharmDir {
	var dirs []*charm.CharmDir
	paths, _ := filepath.Glob(filepath.Join(*repo, "*", "*", "metadata.yaml"))
	for _, path := range paths {
		dirs = appendCharmDir(dirs, filepath.Dir(path))
	}
	return dirs
}

func readNamedCharms(names []string) []*charm.CharmDir {
	series, err := readRepoSeries()
	if err != nil {
		return nil
	}
	var dirs []*charm.CharmDir
	for _, name := range names {
		if strings.Index(name, "/") != -1 {
			dirs = appendCharmDir(dirs, filepath.Join(*repo, filepath.FromSlash(name)))
			continue
		}
		for _, s := range series {
			path := filepath.Join(*repo, s, name)
			if _, err := os.Stat(filepath.Join(path, "metadata.yaml")); err != nil {
				continue
			}
			dirs = appendCharmDir(dirs, path)
		}
	}
	return dirs
}

func readRepoSeries() ([]string, error) {
	infos, err := ioutil.ReadDir(*repo)
	if err != nil {
		return nil, errors.Newf("cannot read charm repo dir: %v", err)
	}
	var series []string
	for _, info := range infos {
		if info.IsDir() {
			series = append(series, info.Name())
		}
	}
	if len(series) == 0 {
		return nil, errors.Newf("no series found in charm repository %q", *repo)
	}
	return series, nil
}

func charmFromCurrentDir() (*charm.CharmDir, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	repoDir := filepath.Clean(*repo)
	if !strings.HasPrefix(dir, repoDir+string(filepath.Separator)) {
		return nil, errors.Newf("current directory is not inside charm repository %q", *repo)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "metadata.yaml")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, errors.New("cannot find charm containing current directory")
		}
		dir = parent
	}
	return charm.ReadCharmDir(dir)
}
