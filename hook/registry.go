package hook

import (
	"launchpad.net/errgo/errors"
	"net/rpc"
	"os"
	"path/filepath"
)

type hookFunc struct {
	localStateName string
	run            func(ctxt *Context) error
}

// Registry allows the registration of hook functions.
type Registry struct {
	localStateName string
	hooks          map[string][]hookFunc
}

// NewRegistry returns a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks: make(map[string][]hookFunc),
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

// MainFunc will be called by Main if not nil and the runhook executable
// is invoked with "main" as its first argument. os.Args be changed to
// exclude the original first argument, so this function can be written
// as if it was a regular main function.
//
// This enables a runhook executable to build in other functionality
// that is not directly executed as part of a hook.
var MainFunc func()

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

// Main creates a new context from the environment
// and invokes the appropriate hook function registered
// in the given registry.
// It expects to find the hook name in os.Args[1]; this
// may also be "main" in which case it will invoke MainFunc.
func Main(r *Registry) error {
	if len(os.Args) < 2 {
		return errors.New("usage: runhook hook-name|main [mainargs...]")
	}
	if os.Args[1] == "main" {
		if MainFunc == nil {
			return errors.New("no main function registered")
		}
		// Elide "main" argument.
		os.Args = append(os.Args[:1], os.Args[2:]...)
		MainFunc()
		return nil
	}
	ctxt, err := NewContext()
	if err != nil {
		return errors.Wrap(err)
	}
	defer ctxt.Close()
	hookFuncs, ok := r.hooks[ctxt.HookName]
	if !ok {
		return errors.Newf("hook %q not registered", ctxt.HookName)
	}
	for _, f := range hookFuncs {
		ctxt.localStateName = f.localStateName
		if err := f.runHook(ctxt); err != nil {
			// TODO better error context here, perhaps
			// including local state name, hook name, etc.
			return errors.Wrap(err)
		}
	}
	return nil
}

func (f hookFunc) runHook(ctxt *Context) (err error) {
	defer func() {
		if saveErr := ctxt.SaveState(); saveErr != nil {
			if err == nil {
				err = saveErr
			} else {
				ctxt.Logf("cannot save local state: %v", saveErr)
			}
		}
	}()
	return f.run(ctxt)
}

// NewContext creates a hook context from the current environment.
// Clients should not use this function, but use their init functions to
// call Register to register a hook function instead, which enables
// gocharm to generate hook stubs automatically.
//
// Local state will be stored relative to the given localStateName.
func NewContext() (*Context, error) {
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
	ctxt := &Context{
		UUID:           os.Getenv(envUUID),
		Unit:           os.Getenv(envUnitName),
		CharmDir:       os.Getenv(envCharmDir),
		RelationName:   os.Getenv(envRelationName),
		RelationId:     os.Getenv(envRelationId),
		RemoteUnit:     os.Getenv(envRemoteUnit),
		HookName:       hookName,
		jujucContextId: os.Getenv(envJujuContextId),
		localState:     make(map[string]interface{}),
	}
	client, err := rpc.Dial("unix", os.Getenv(envSocketPath))
	if err != nil {
		return nil, errors.Newf("cannot dial uniter: %v", err)
	}
	ctxt.jujucClient = client
	return ctxt, nil
}

// Close closes the context's connection to the unit agent.
func (ctxt *Context) Close() error {
	return ctxt.jujucClient.Close()
}
