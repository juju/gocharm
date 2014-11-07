package hook

import (
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"launchpad.net/errgo/errors"
)

type hookFunc struct {
	localStateName string
	run            func(ctxt *Context) error
}

// Registry allows the registration of hook functions.
type Registry struct {
	localStateName string
	hooks          map[string][]hookFunc
	commands       map[string]func()
}

// NewRegistry returns a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks:    make(map[string][]hookFunc),
		commands: make(map[string]func()),
	}
}

// Register registers the given function to be called when the
// charm hook with the given name is invoked.
// The function must not use its provided Context
// after it returns.
//
// If more than one function is registered for a given hook,
// each function will be called in turn until one returns an error;
// the context's local state will be saved with SaveState
// after each call.
func (r *Registry) Register(name string, f func(ctxt *Context) error) {
	// TODO(rog) implement validHookName
	//if !validHookName(name) {
	//	panic(fmt.Errorf("invalid hook name %q", name))
	//}
	r.hooks[name] = append(r.hooks[name], hookFunc{
		run:            f,
		localStateName: r.localStateName,
	})
}

// RegisterCommand registers the given function to be called
// when the hook is invoked with a first argument of "cmd".
// The name is relative to the registry's state namespace.
// It will panic if the same name is registered more than
// once in the same Registry.
//
// When the function is called, os.Args will be set up as
// if the function is main - the "cmd-" command selector
// will be removed.
func (r *Registry) RegisterCommand(name string, f func()) {
	// TODO check that name is vaid (non-empty, no slashes)

	name = filepath.Join(r.localStateName, name)
	if r.commands[name] != nil {
		panic(errors.Newf("command %q is already registered", name))
	}
	r.commands[name] = f
}

// NewRegistry returns a sub-registry of r. Local state
// stored by hooks registered with that will be stored relative to the
// given name within r; likewise new registries created by NewRegistry
// on it will store local state relatively to r.
//
// This enables hierarchical local storage for charm hooks.
func (r *Registry) NewRegistry(localStateName string) *Registry {
	// TODO check name is valid
	return &Registry{
		localStateName: filepath.Join(r.localStateName, localStateName),
		hooks:          r.hooks,
		commands:       r.commands,
	}
}

// RegisteredHooks returns the names of all currently
// registered hooks.
func (r *Registry) RegisteredHooks() []string {
	var names []string
	for name := range r.hooks {
		names = append(names, name)
	}
	return names
}

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

func usageError(r *Registry) error {
	var allowed []string
	for cmd := range r.commands {
		allowed = append(allowed, "cmd-"+cmd+" [arg...]")
	}
	for hook := range r.hooks {
		allowed = append(allowed, hook)
	}
	// The stop hook is always allowed.
	if r.hooks["stop"] == nil {
		allowed = append(allowed, "stop")
	}
	sort.Strings(allowed[0:len(r.commands)])
	sort.Strings(allowed[len(r.commands):])
	return errors.Newf("usage: runhook %s", strings.Join(allowed, "\n\t| runhook "))
}

// Main creates a new context from the environment and invokes the
// appropriate command or hook functions from the given
// registry or sub-registries of it.
func Main(r *Registry) (err error) {
	if len(r.hooks) == 0 && len(r.commands) == 0 {
		return fmt.Errorf("no registered hooks or commands")
	}
	if len(os.Args) < 2 {
		return usageError(r)
	}
	if strings.HasPrefix(os.Args[1], "cmd-") {
		cmdName := strings.TrimPrefix(os.Args[1], "cmd-")
		cmd := r.commands[cmdName]
		if cmd == nil {
			return usageError(r)
		}
		// Elide the command name argument.
		os.Args = append(os.Args[:1], os.Args[2:]...)
		cmd()
		return nil
	}
	ctxt, err := newContext()
	if err != nil {
		return errors.Wrap(err)
	}
	defer ctxt.close()
	defer func() {
		// Save the state after all hooks have been run.
		if saveErr := ctxt.saveState(); saveErr != nil {
			if err == nil {
				err = errors.Wrapf(saveErr, "cannot save local state")
			} else {
				ctxt.Logf("cannot save local state: %v", saveErr)
			}
		}
	}()
	hookFuncs, ok := r.hooks[ctxt.HookName]
	if !ok && ctxt.HookName != "stop" {
		ctxt.Logf("hook %q not registered", ctxt.HookName)
		return usageError(r)
	}
	for _, f := range hookFuncs {
		ctxt := ctxt.withLocalStateName(f.localStateName)
		if err := f.run(ctxt); err != nil {
			// TODO better error context here, perhaps
			// including local state name, hook name, etc.
			return errors.Wrap(err)
		}
	}
	if ctxt.HookName == "stop" {
		// We've shut down, so clean up all our local state.
		if err := os.RemoveAll(ctxt.StateDir()); err != nil {
			return errors.Wrapf(err, "cannot remove local state")
		}
	}
	return nil
}

// newContext creates a hook context from the current environment.
func newContext() (*Context, error) {
	vars := mustEnvVars
	if os.Getenv(envRelationName) != "" {
		vars = append(vars, relationEnvVars...)
	}
	for _, v := range vars {
		if os.Getenv(v) == "" {
			return nil, errors.Newf("required environment variable %q not set", v)
		}
	}
	if len(os.Args) != 2 {
		return nil, errors.New("one argument required")
	}
	hookName := os.Args[1]
	internalCtxt := &internalContext{
		info: ContextInfo{
			UUID:         os.Getenv(envUUID),
			Unit:         os.Getenv(envUnitName),
			CharmDir:     os.Getenv(envCharmDir),
			RelationName: os.Getenv(envRelationName),
			RelationId:   os.Getenv(envRelationId),
			RemoteUnit:   os.Getenv(envRemoteUnit),
			HookName:     hookName,
		},
		jujucContextId: os.Getenv(envJujuContextId),
		localState:     make(map[string]reflect.Value),
	}
	client, err := rpc.Dial("unix", os.Getenv(envSocketPath))
	if err != nil {
		return nil, errors.Newf("cannot dial uniter: %v", err)
	}
	internalCtxt.jujucClient = client
	return &Context{
		ContextInfo:     &internalCtxt.info,
		internalContext: internalCtxt,
	}, nil
}

// Close closes the context's connection to the unit agent.
func (ctxt *internalContext) close() error {
	return ctxt.jujucClient.Close()
}
