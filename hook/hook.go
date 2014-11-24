// The hook package provides a Go interface to the
// Juju charm hook commands. It is designed to be used
// alongside the gocharm command (github.com/juju/gocharm/cmd/gocharm)
//
// TODO explain more about relations, relation ids and relation units.
package hook

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/names"
	"launchpad.net/errgo/errors"
)

// RelationId is the type of the id of a relation. A relation with
// a given id corresponds to a relation as created by the
// juju add-relation command.
type RelationId string

// UnitId is the type of the id of a unit.
type UnitId string

func (id UnitId) Tag() names.UnitTag {
	return names.NewUnitTag(string(id))
}

// Context provides information about the
// hook context. It should be treated as read-only.
type Context struct {
	// registryName holds the name of the registry that
	// the context is associated with.
	registryName string

	// Fields valid for all hooks

	// UUID holds the globally unique environment id.
	UUID string

	// Unit holds the name of the current charm's unit.
	Unit UnitId

	// CharmDir holds the directory that the charm is running from.
	CharmDir string

	// HookName holds the name of the currently running hook.
	HookName string

	// Relations holds all the relation data available to the charm.
	// For each relation id, it holds all the units that have joined
	// that relation, and within that, all the relation settings for
	// each of those units.
	//
	// This does not include settings for the charm unit itself.
	Relations map[RelationId]map[UnitId]map[string]string

	// RelationIds holds the relation ids for each relation declared
	// in the charm. For example, if the charm has a relation named
	// "webserver" in its metadata.yaml, the current ids for that
	// relation (i.e. all the relations have have been made by the
	// user) will be in RelationIds["webserver"].
	RelationIds map[string][]RelationId

	// Fields valid for relation-related hooks only.

	// RelationName holds the name of the relation that
	// the current relation hook is running for. It will be
	// one of the relation names declared in the metadata.yaml file.
	RelationName string

	// RelationId holds the id of the relation that
	// the current relation hook is running for. See RelationIds above.
	RelationId RelationId

	// RemoteUnit holds the id of the unit that the current
	// relation hook is running for. This will be empty
	// for a relation-broken hook.
	RemoteUnit UnitId

	// Runner is used to run hook tools by methods on the context.
	Runner ToolRunner

	// RunCommandName holds the name of the command, when
	// the runhook executable is run as a command.
	// If this is set, none of the other fields will be valid.
	// This will never be set when the context is passed
	// into any hook function.
	RunCommandName string

	// RunCommandArgs holds any arguments that were passed to
	// the above command.
	RunCommandArgs []string
}

// Relation holds the current relation settings for the unit
// that triggered the current hook. It will panic if
// the current hook is not a relation-related hook.
func (ctxt *Context) Relation() map[string]string {
	if ctxt.RemoteUnit == "" || ctxt.RelationId == "" {
		panic(fmt.Errorf("Relation called in non-relation hook %s", ctxt.HookName))
	}
	return ctxt.Relations[ctxt.RelationId][ctxt.RemoteUnit]
}

// Close closes ctxt.Runner, if it is not nil.
func (ctxt *Context) Close() error {
	if ctxt.Runner != nil {
		return ctxt.Runner.Close()
	}
	return nil
}

// withRegistryName returns a Context that's the same as
// ctxt but is associated with the registry with the given name.
func (ctxt *Context) withRegistryName(registryName string) *Context {
	ctxt1 := *ctxt
	ctxt1.registryName = registryName
	return &ctxt1
}

// hookStateDir is where hook local state will be stored.
// TODO would /etc/init be a better place for this?
var hookStateDir = "/var/lib/juju-localstate"

// StateDir returns the path to the directory where local state for the
// given context will be stored. The directory is relative to the
// registry through which the context was created. It is not guaranteed
// to exist.
func (ctxt *Context) StateDir() string {
	return filepath.Join(hookStateDir, ctxt.UUID+"-"+ctxt.UnitTag(), ctxt.registryName)
}

// CommandName returns a value that can be used to make runhook run the
// given command when passed as its first argument. The command run
// will be the command registered with RegisterCommand on the registry
// this context is derived from.
// TODO better explanation and an example.
func (ctxt *Context) CommandName() string {
	return "cmd-" + ctxt.registryName
}

func (ctxt *Context) IsRelationHook() bool {
	return ctxt.RelationName != ""
}

// UnitTag returns the tag of the current unit, useful for
// using as a file name.
func (ctxt *Context) UnitTag() string {
	return names.NewUnitTag(string(ctxt.Unit)).String()
}

func (ctxt *Context) OpenPort(proto string, port int) error {
	_, err := ctxt.Runner.Run("open-port", fmt.Sprintf("%d/%s", port, proto))
	return errors.Wrap(err)
}

func (ctxt *Context) ClosePort(proto string, port int) error {
	_, err := ctxt.Runner.Run("close-port", fmt.Sprintf("%d/%s", port, proto))
	return errors.Wrap(err)
}

// PublicAddress returns the public address of the local unit.
func (ctxt *Context) PublicAddress() (string, error) {
	out, err := ctxt.Runner.Run("unit-get", "public-address")
	if err != nil {
		return "", errors.Wrap(err)
	}
	return strings.TrimSpace(string(out)), nil
}

// PrivateAddress returns the private address of the local unit.
func (ctxt *Context) PrivateAddress() (string, error) {
	out, err := ctxt.Runner.Run("unit-get", "private-address")
	if err != nil {
		return "", errors.Wrap(err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Log logs a message through the juju logging facility.
func (ctxt *Context) Logf(f string, a ...interface{}) error {
	_, err := ctxt.Runner.Run("juju-log", fmt.Sprintf(f, a...))
	return errors.Wrap(err)
}

// getAllRelationUnit returns all the settings from the given unit associated
// with the relation with the given id.
func (ctxt *Context) getAllRelationUnit(relationId RelationId, unit UnitId) (map[string]string, error) {
	var val map[string]string
	if err := ctxt.runJson(&val, "relation-get", "-r", string(relationId), "--format", "json", "--", "-", string(unit)); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

// relationIds returns all the relation ids associated
// with the relation with the given name.
func (ctxt *Context) relationIds(relationName string) ([]RelationId, error) {
	var val []RelationId
	if err := ctxt.runJson(&val, "relation-ids", "--format", "json", "--", relationName); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

// relationUnits returns all the units associated with the given relation id.
func (ctxt *Context) relationUnits(relationId RelationId) ([]UnitId, error) {
	var val []UnitId
	if err := ctxt.runJson(&val, "relation-list", "--format", "json", "-r", string(relationId)); err != nil {
		return nil, errors.Wrap(err)
	}
	return val, nil
}

// SetRelation sets the given key-value pairs on the current relation instance.
func (ctxt *Context) SetRelation(keyvals ...string) error {
	err := ctxt.SetRelationWithId(ctxt.RelationId, keyvals...)
	return errors.Wrap(err)
}

// SetRelationWithId sets the given key-value pairs
// on the relation with the given id.
func (ctxt *Context) SetRelationWithId(relationId RelationId, keyvals ...string) error {
	if len(keyvals)%2 != 0 {
		return errors.Newf("invalid key/value count")
	}
	if len(keyvals) == 0 {
		return nil
	}
	args := make([]string, 0, 3+len(keyvals)/2)
	args = append(args, "-r", string(relationId), "--")
	for i := 0; i < len(keyvals); i += 2 {
		args = append(args, fmt.Sprintf("%s=%s", keyvals[i], keyvals[i+1]))
	}
	_, err := ctxt.Runner.Run("relation-set", args...)
	return errors.Wrap(err)
}

// GetConfig reads the charm configuration value for the given
// key into the value pointed to by val, which should be
// a pointer to one of the possible configuration option
// types (string, int, float64 or boolean).
// To find out whether a value has actually been set (is non-null)
// pass a pointer to a pointer to the desired type.
func (ctxt *Context) GetConfig(key string, val interface{}) error {
	if err := ctxt.runJson(val, "config-get", "--format", "json", "--", key); err != nil {
		return errors.Wrapf(err, "cannot get configuration option %q", key)
	}
	return nil
}

// GetConfigString returns the charm configuration value for the given
// key as a string. It returns the empty string if the value has not been
// set.
func (ctxt *Context) GetConfigString(key string) (string, error) {
	var val string
	if err := ctxt.GetConfig(key, &val); err != nil {
		return "", errors.Wrap(err)
	}
	return val, nil
}

// GetConfigString returns the charm configuration value for the given
// key as an int. It returns zero if the value has not been
// set.
func (ctxt *Context) GetConfigInt(key string) (int, error) {
	var val int
	if err := ctxt.GetConfig(key, &val); err != nil {
		return 0, errors.Wrap(err)
	}
	return val, nil
}

// GetConfigString returns the charm configuration value for the given
// key as a float64. It returns zero if the value has not been
// set.
func (ctxt *Context) GetConfigFloat64(key string) (float64, error) {
	var val float64
	if err := ctxt.GetConfig(key, &val); err != nil {
		return 0, errors.Wrap(err)
	}
	return val, nil
}

// GetConfigString returns the charm configuration value for the given
// key as a bool. It returns false if the value has not been
// set.
func (ctxt *Context) GetConfigBool(key string) (bool, error) {
	var val bool
	if err := ctxt.GetConfig(key, &val); err != nil {
		return false, errors.Wrap(err)
	}
	return val, nil
}

// GetAllConfig unmarshals all the configuration values from
// a JSON object into the given value, which should be a pointer
// to a struct or a map. To get all values without knowing
// what they might be, pass in a pointer to a map[string]interface{}
// value,
func (ctxt *Context) GetAllConfig(val interface{}) error {
	if err := ctxt.runJson(&val, "config-get", "--format", "json"); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (ctxt *Context) runJson(dst interface{}, cmd string, args ...string) error {
	out, err := ctxt.Runner.Run(cmd, args...)
	if err != nil {
		return errors.Wrap(err)
	}
	if err := json.Unmarshal(out, dst); err != nil {
		return errors.Wrapf(err, "cannot parse command output %q", out)
	}
	return nil
}
