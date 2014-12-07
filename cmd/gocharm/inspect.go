package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v4"
)

func registeredCharmInfo(pkg, tempDir string) (*charmInfo, error) {
	code := generateCode(inspectCode, pkg)
	inspectExe := filepath.Join(tempDir, "inspect")
	err := compile(filepath.Join(tempDir, "inspect.go"), inspectExe, code, false)
	if err != nil {
		return nil, errgo.Notef(err, "cannot build hook inspection code")
	}
	c := exec.Command(inspectExe)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return nil, errgo.Notef(err, "failed to run inspect")
	}
	var out charmInfo
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal %q", err)
	}
	if len(out.Hooks) == 0 {
		return nil, errgo.New("no hooks registered")
	}
	if *verbose {
		log.Printf("registered hooks: %v", out.Hooks)
		log.Printf("%d registered relations", len(out.Relations))
		log.Printf("%d registered config options", len(out.Config))
	}
	return &out, nil
}

// charmInfo holds the information we glean
// from inspecting the hook registry.
// Note that this must be kept in sync with the
// version in inspectCode below.
type charmInfo struct {
	Hooks     []string
	Relations map[string]charm.Relation
	Config    map[string]charm.Option
}

var inspectCode = template.Must(template.New("").Parse(`
// {{.AutogenMessage}}

package main

import (
	"encoding/json"
	"gopkg.in/juju/charm.v4"
	"os"

	inspect {{.CharmPackage | printf "%q"}}
	{{.HookPackage | printf "%q"}}
)

// charmInfo must be kept in sync with the charmInfo
// type above.
type charmInfo struct {
	Hooks     []string
	Relations map[string]charm.Relation
	Config    map[string]charm.Option
}

func main() {
	r := hook.NewRegistry()
	inspect.RegisterHooks(r)
	hook.RegisterMainHooks(r)
	data, err := json.Marshal(charmInfo{
		Hooks:     r.RegisteredHooks(),
		Relations: r.RegisteredRelations(),
		Config:    r.RegisteredConfig(),
	})
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(data)
}
`))
