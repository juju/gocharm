package main
import (
	"fmt"
	"os"
	"flag"
	"path/filepath"
	"os/exec"
	"launchpad.net/juju-core/charm"
)

var repo = flag.String("repo", "", "charm repo directory (defaults to JUJU_REPOSITORY)")
var test = flag.Bool("test", false, "run tests before building")

var exitCode = 0

func main() {
	flag.Parse()
	if *repo == "" {
		if *repo = os.Getenv("JUJU_REPOSITORY"); *repo == "" {
			fatalf("no charm repo directory specified")
		}
	}
	paths, _ := filepath.Glob(filepath.Join(*repo, "*", "*", "metadata.yaml"))
	if len(paths) == 0 {
		fatalf("no charms found")
	}
	var dirs []*charm.Dir
	for _, path := range paths {
		path := filepath.Dir(path)
		dir, err := charm.ReadDir(path)
		if err != nil {
			errorf("cannot read %q: %v", path, err)
			continue
		}
		dirs = append(dirs, dir)
	}
	for _, dir := range dirs {
		doneSomething, err := compileDir(dir)
		if err != nil {
			errorf("cannot compile %q: %v", dir.Path, err)
			continue
		}
		if doneSomething {
			if err := dir.SetDiskRevision(dir.Revision() + 1); err != nil {
				errorf("cannot bump revision on %q: %v", dir.Path, err)
			}
		}
	}
	os.Exit(exitCode)
}

func compileDir(dir *charm.Dir) (doneSomething bool, err error) {
	if info, err := os.Stat(filepath.Join(dir.Path, "src/hooks")); err != nil || !info.IsDir() {
		return false, nil
	}
	defer os.RemoveAll(filepath.Join(dir.Path, "pkg"))
	env := append(os.Environ(), "GOPATH="+dir.Path)

	if *test {
		if err := run(env, "go", "test", "-i", "./..."); err != nil {
			return false, err
		}
		if err := run(env, "go", "test", "./hooks"); err != nil {
			return false, err
		}
	}
	return true, run(env, "go", "install", "./...")
}

func run(env []string, cmd string, args ...string) error {
	c := exec.Command("go", "test", "./...")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = env
	return c.Run()
}

func errorf(f string, a ...interface{}) {
	exitCode = 1
	fmt.Fprintf(os.Stderr, "gocharm: %s\n", fmt.Sprintf(f, a...))
}

func fatalf(f string, a ...interface{}) {
	errorf(f, a...)
	os.Exit(2)
}
