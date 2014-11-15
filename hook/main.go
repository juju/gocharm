package hook

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"launchpad.net/errgo/errors"
)

const (
	envUUID          = "JUJU_ENV_UUID"
	envUnitName      = "JUJU_UNIT_NAME"
	envCharmDir      = "CHARM_DIR"
	envJujuContextId = "JUJU_CONTEXT_ID"
	envRelationName  = "JUJU_RELATION"
	envRelationId    = "JUJU_RELATION_ID"
	envRemoteUnit    = "JUJU_REMOTE_UNIT"
	envSocketPath    = "JUJU_AGENT_SOCKET"
)

var mustEnvVars = []string{
	envUUID,
	envUnitName,
	envCharmDir,
	envJujuContextId,
	envSocketPath,
}

var relationEnvVars = []string{
	envRelationName,
	envRelationId,
	envRemoteUnit,
}

// Main creates a new context from the environment and invokes the
// appropriate command or hook functions from the given
// registry or sub-registries of it.
//
// The ctxt value holds the context that will be passed
// to the hooks; the state value is used to retrieve
// and save persistent state.
//
// This function is designed to be called by gocharm
// generated code only.
func Main(r *Registry, ctxt *Context, state PersistentState) (err error) {
	if ctxt.RunCommandName != "" {
		cmd := r.commands[ctxt.RunCommandName]
		if cmd == nil {
			return usageError(r)
		}
		cmd(ctxt.RunCommandArgs)
		return nil
	}
	// Retrieve all persistent state.
	// TODO read all of the state in one operation from a single file?
	if err := loadState(r, state); err != nil {
		return errors.Wrap(err)
	}
	// Notify everyone about the context.
	for _, setter := range r.contexts {
		if err := setter(ctxt); err != nil {
			return errors.Wrapf(err, "cannot set context")
		}
	}
	defer func() {
		// All the hooks have now run; save the state.
		saveErr := saveState(r, state)
		if saveErr == nil {
			return
		}
		if err == nil {
			err = errors.Wrapf(saveErr, "cannot save local state")
			return
		}
		ctxt.Logf("cannot save local state: %v", saveErr)
	}()

	// The wildcard hook always runs after any other
	// registered hooks.
	hookFuncs := r.hooks[ctxt.HookName]

	if len(hookFuncs) == 0 {
		ctxt.Logf("hook %q not registered", ctxt.HookName)
		return usageError(r)
	}
	hookFuncs = append(hookFuncs, r.hooks["*"]...)
	for _, f := range hookFuncs {
		if err := f.run(); err != nil {
			// TODO better error context here, perhaps
			// including local state name, hook name, etc.
			return errors.Wrap(err)
		}
	}
	return nil
}

func loadState(r *Registry, state PersistentState) error {
	for _, val := range r.state {
		data, err := state.Load(val.registryName)
		if err != nil {
			return errors.Wrapf(err, "cannot load state for %s", val.registryName)
		}
		if data == nil {
			continue
		}
		if err := json.Unmarshal(data, val.val); err != nil {
			return errors.Wrapf(err, "cannot unmarshal state for %s", val.registryName)
		}
	}
	return nil
}

func saveState(r *Registry, state PersistentState) (err error) {
	for _, val := range r.state {
		data, err := json.Marshal(val.val)
		if err != nil {
			return errors.Wrapf(err, "cannot marshal state for %s", val.registryName)
		}
		if err := state.Save(val.registryName, data); err != nil {
			return errors.Wrapf(err, "cannot save state for %s", val.registryName)
		}
	}
	return nil
}

func usageError(r *Registry) error {
	var allowed []string
	for cmd := range r.commands {
		allowed = append(allowed, "cmd-"+cmd+" [arg...]")
	}
	for hook := range r.hooks {
		allowed = append(allowed, hook)
	}
	sort.Strings(allowed[0:len(r.commands)])
	sort.Strings(allowed[len(r.commands):])
	return errors.Newf("usage: runhook %s", strings.Join(allowed, "\n\t| runhook "))
}

func nop() error {
	return nil
}

// RegisterMainHooks registers any hooks that
// are needed by any charm. It should be
// called after any other Register functions.
//
// This function is designed to be called by gocharm
// generated code only.
func RegisterMainHooks(r *Registry) {
	// We always need install and start hooks.
	r.RegisterHook("install", nop)
	r.RegisterHook("start", nop)
	// TODO Perhaps... ensure that we have a stop hook, and make
	// it clean up our persistent state. But that may not be
	// right if "stop" is considered something we can start
	// from again.
}

// NewContextFromEnvironment creates a hook context from the current
// environment, using the given tool runner to acquire information to
// populate the context, and the given registry to determine which
// relations to fetch information for.
//
// It also returns the persistent state associated with the context
// unless called in a command-running context.
//
// The caller is responsible for calling Close on the returned
// context.
func NewContextFromEnvironment(r *Registry) (*Context, PersistentState, error) {
	if len(os.Args) < 2 {
		return nil, nil, usageError(r)
	}
	hookName := os.Args[1]
	if strings.HasPrefix(hookName, "cmd-") {
		return &Context{
			RunCommandName: strings.TrimPrefix(hookName, "cmd-"),
			RunCommandArgs: os.Args[2:],
		}, nil, nil
	}
	vars := mustEnvVars
	if os.Getenv(envRelationName) != "" {
		vars = append(vars, relationEnvVars...)
	}
	for _, v := range vars {
		if os.Getenv(v) == "" {
			return nil, nil, errors.Newf("required environment variable %q not set", v)
		}
	}
	if len(os.Args) != 2 {
		return nil, nil, errors.New("one argument required")
	}
	runner, err := newToolRunnerFromEnvironment()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot make runner")
	}
	ctxt := &Context{
		UUID:         os.Getenv(envUUID),
		Unit:         UnitId(os.Getenv(envUnitName)),
		CharmDir:     os.Getenv(envCharmDir),
		RelationName: os.Getenv(envRelationName),
		RelationId:   RelationId(os.Getenv(envRelationId)),
		RemoteUnit:   UnitId(os.Getenv(envRemoteUnit)),
		HookName:     hookName,
		Runner:       runner,
	}

	// Populate the relation fields of the ContextInfo
	ctxt.RelationIds = make(map[string][]RelationId)
	ctxt.Relations = make(map[RelationId]map[UnitId]map[string]string)
	for name := range r.RegisteredRelations() {
		ids, err := ctxt.relationIds(name)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "cannot get relation ids for relation %q", name)
		}
		ctxt.RelationIds[name] = ids
		for _, id := range ids {
			units := make(map[UnitId]map[string]string)
			unitIds, err := ctxt.relationUnits(id)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "cannot get unit ids for relation id %q", id)
			}
			for _, unitId := range unitIds {
				settings, err := ctxt.getAllRelationUnit(id, unitId)
				if err != nil {
					return nil, nil, errors.Wrapf(err, "cannot get settings for relation %s, unit %s", id, unitId)
				}
				units[unitId] = settings
			}
			ctxt.Relations[id] = units
		}
	}
	return ctxt, NewDiskState(ctxt.StateDir()), nil
}
