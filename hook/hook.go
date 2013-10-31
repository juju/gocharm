// The hook package provides a Go interface to the
// Juju charm hook commands. It is designed to be used
// alongside the gocharm command (launchpad.net/juju-utils/cmd/gocharm)
package hook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"launchpad.net/errgo/errors"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
)

// Context provides the context for a running Juju hook.
type Context struct {
	// Valid for all hooks
	UUID     string
	Unit     string
	CharmDir string
	HookName string

	// Valid for relation-related hooks.
	RelationName string
	RelationId   string
	RemoteUnit   string

	jujucContextId string
	jujucClient    *rpc.Client

	// localState maps from file path to the data to be
	// stored there.
	localState map[string]interface{}

	localStateName string
}

func (ctxt *Context) stateDir() string {
	return filepath.Join(ctxt.CharmDir, "localstate", ctxt.localStateName)
}

// LocalState reads charm local state for the given name, which should
// be valid to use as a file name, and uses it to fill out the value
// pointed to by ptr, which should be marshallable and unmarshallable as
// JSON. When the hook has completed, the value will be saved back to
// the same place, enabling persistent state to be saved.
//
// LocalState will panic if it is called twice for the same name without
// an intervening call to SaveState.
func (ctxt *Context) LocalState(name string, ptr interface{}) error {
	// TODO check that name is valid (no slash, no .json extension)
	path := filepath.Join(ctxt.stateDir(), name) + ".json"
	if _, ok := ctxt.localState[path]; ok {
		panic(errors.Newf("LocalState called twice for %q", path))
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err)
		}
		ctxt.localState[path] = ptr
		return nil
	}
	if err := json.Unmarshal(data, ptr); err != nil {
		return errors.Wrap(err)
	}
	ctxt.localState[path] = ptr
	return nil
}

// SaveState saves the state of all the values passed to LocalState.
// If this succeeds, LocalState may be called again for any name.
func (ctxt *Context) SaveState() error {
	made := make(map[string]bool)
	for path, ptr := range ctxt.localState {
		if dir := filepath.Dir(path); !made[dir] {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return errors.Wrap(err)
			}
			made[dir] = true
		}
		data, err := json.Marshal(ptr)
		if err != nil {
			return errors.Wrapf(err, "could not marshal state %q", path)
		}
		if err := ioutil.WriteFile(path, data, 0600); err != nil {
			return errors.Wrapf(err, "could not save state to %q", path)
		}
	}
	ctxt.localState = make(map[string]interface{})
	return nil
}

// CommandName returns a value that can be used to
// make runhook run the given command when passed
// as its first argument.
// The command name is relative to the registry
// from which ctxt was created.
// TODO better explanation and an example.
func (ctxt *Context) CommandName(name string) string {
	// TODO panic if name is empty?
	return filepath.Join("cmd-" + filepath.Join(ctxt.localStateName, name))
}

func (ctxt *Context) IsRelationHook() bool {
	return ctxt.RelationName != ""
}

func (ctxt *Context) OpenPort(proto string, port int) error {
	_, err := ctxt.run("open-port", fmt.Sprintf("%d/%s", port, proto))
	return errors.Wrap(err)
}

func (ctxt *Context) ClosePort(proto string, port int) error {
	_, err := ctxt.run("close-port", fmt.Sprintf("%d/%s", port, proto))
	return errors.Wrap(err)
}

// PrivateAddress returns the public address of the local unit.
func (ctxt *Context) PublicAddress() (string, error) {
	out, err := ctxt.run("unit-get", "public-address")
	if err != nil {
		return "", errors.Wrap(err)
	}
	return strings.TrimSpace(string(out)), nil
}

// PrivateAddress returns the private address of the local unit.
func (ctxt *Context) PrivateAddress() (string, error) {
	out, err := ctxt.run("unit-get", "private-address")
	if err != nil {
		return "", errors.Wrap(err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Log logs a message through the juju logging facility.
func (ctxt *Context) Logf(f string, a ...interface{}) error {
	_, err := ctxt.run("juju-log", fmt.Sprintf(f, a...))
	return errors.Wrap(err)
}

// GetRelation returns the value with the given key from the
// relation and unit that triggered the hook execution.
// It is equivalent to GetRelationUnit(RelationId, RemoteUnit, key).
func (ctxt *Context) GetRelation(key string) (string, error) {
	r, err := ctxt.GetRelationUnit(ctxt.RelationId, ctxt.RemoteUnit, key)
	if err != nil {
		return "", errors.Wrap(err)
	}
	return r, nil
}

// GetRelationUnit returns the value with the given key
// from the given unit associated with the relation with the
// given id.
func (ctxt *Context) GetRelationUnit(relationId, unit, key string) (string, error) {
	var val string
	if err := ctxt.runJson(&val, "relation-get", "--format", "json", "-r", relationId, "--", key, unit); err != nil {
		return "", errors.Wrap(err)
	}
	return val, nil
}

// GetAllRelation returns all the settings for the relation
// and unit that triggered the hook execution.
// It is equivalent to GetAllRelationUnit(RelationId, RemoteUnit).
func (ctxt *Context) GetAllRelation() (map[string]string, error) {
	m, err := ctxt.GetAllRelationUnit(ctxt.RelationId, ctxt.RemoteUnit)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return m, nil
}

// GetAllRelationUnit returns all the settings from the given unit associated
// with the relation with the given id.
func (ctxt *Context) GetAllRelationUnit(relationId, unit string) (map[string]string, error) {
	var val map[string]string
	if err := ctxt.runJson(&val, "relation-get", "-r", relationId, "--format", "json", "--", "-", unit); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

// RelationIds returns all the relation ids associated
// with the relation with the given name,
func (ctxt *Context) RelationIds(relationName string) ([]string, error) {
	var val []string
	if err := ctxt.runJson(&val, "relation-ids", "--format", "json", "--", relationName); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

func (ctxt *Context) RelationUnits(relationId string) ([]string, error) {
	var val []string
	if err := ctxt.runJson(&val, "relation-list", "--format", "json", "-r", relationId); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

// AllRelationUnits returns a map from all the relation ids
// for the relation with the given name to all the
// units with that name
func (ctxt *Context) AllRelationUnits(relationName string) (map[string][]string, error) {
	allUnits := make(map[string][]string)
	ids, err := ctxt.RelationIds(relationName)
	if err != nil {
		return nil, errors.Newf("cannot get relation ids: %v", err)
	}
	for _, id := range ids {
		units, err := ctxt.RelationUnits(id)
		if err != nil {
			return nil, errors.Newf("cannot get relation units for id %q: %v", id, err)
		}
		allUnits[id] = units
	}
	return allUnits, nil
}

// SetRelation sets the given key-value pairs on the current relation instance.
func (ctxt *Context) SetRelation(keyvals ...string) error {
	err := ctxt.SetRelationWithId(ctxt.RelationId, keyvals...)
	return errors.Wrap(err)
}

// SetRelationWithId sets the given key-value pairs
// on the relation with the given id.
func (ctxt *Context) SetRelationWithId(relationId string, keyvals ...string) error {
	if len(keyvals)%2 != 0 {
		return errors.Newf("invalid key/value count")
	}
	if len(keyvals) == 0 {
		return nil
	}
	args := make([]string, 0, 3+len(keyvals)/2)
	args = append(args, "-r", relationId, "--")
	for i := 0; i < len(keyvals); i += 2 {
		args = append(args, fmt.Sprintf("%s=%s", keyvals[i], keyvals[i+1]))
	}
	_, err := ctxt.run("relation-set", args...)
	return errors.Wrap(err)
}

// GetConfig returns the charm configuration value for the given
// key. Both int- and float-typed values will be returned as float64.
func (ctxt *Context) GetConfig(key string) (interface{}, error) {
	var val interface{}
	if err := ctxt.runJson(&val, "config-get", "--format", "json", "--", key); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

func (ctxt *Context) GetAllConfig() (map[string]interface{}, error) {
	var val map[string]interface{}
	if err := ctxt.runJson(&val, "config-get", "--format", "json"); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

func (ctxt *Context) run(cmd string, args ...string) (stdout []byte, err error) {
	req := jujuc.Request{
		ContextId: ctxt.jujucContextId,
		// We will never use a command that uses a path name,
		// but jujuc checks for an absolute path.
		Dir:         "/",
		CommandName: cmd,
		Args:        args,
	}
	// log.Printf("run req %#v", req)
	var resp jujuc.Response
	err = ctxt.jujucClient.Call("Jujuc.Main", req, &resp)
	if err != nil {
		return nil, errors.Newf("cannot call jujuc.Main: %v", err)
	}
	if resp.Code == 0 {
		return resp.Stdout, nil
	}
	errText := strings.TrimSpace(string(resp.Stderr))
	if strings.HasPrefix(errText, "error: ") {
		errText = errText[len("error: "):]
	}
	return nil, errors.New(errText)
}

func (ctxt *Context) runJson(dst interface{}, cmd string, args ...string) error {
	out, err := ctxt.run(cmd, args...)
	if err != nil {
		return errors.Wrap(err)
	}
	if err := json.Unmarshal(out, dst); err != nil {
		return errors.Newf("cannot parse command output %q into %T: %v", out, dst, err)
	}
	return nil
}
