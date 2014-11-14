package hook

import (
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"
)

type hookFunc struct {
	localStateName string
	run            func() error
}

// ContextSetter is the type of a function that can
// set the context for a hook. Usually this is
// a method that sets a context variable inside a struct.
type ContextSetter func(ctxt *Context) error

// Registry allows the registration of hook functions.
type Registry struct {
	localStateName string
	hooks          map[string][]hookFunc
	commands       map[string]func()
	relations      map[string]charm.Relation
	config         map[string]charm.Option
	contexts       *[]ContextSetter
}

// NewRegistry returns a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks:     make(map[string][]hookFunc),
		commands:  make(map[string]func()),
		relations: make(map[string]charm.Relation),
		config:    make(map[string]charm.Option),
		contexts:  new([]ContextSetter),
	}
}

// RegisterHook registers the given function to be called when the
// charm hook with the given name is invoked.
//
// If the name is "*", the function will always be invoked, after
// any functions registered specifically for the current hook.
//
// If more than one function is registered for a given hook,
// each function will be called in order of registration until
// one returns an error.
func (r *Registry) RegisterHook(name string, f func() error) {
	// TODO(rog) implement validHookName
	//if !validHookName(name) {
	//	panic(fmt.Errorf("invalid hook name %q", name))
	//}
	r.hooks[name] = append(r.hooks[name], hookFunc{
		run:            f,
		localStateName: r.localStateName,
	})
}

// RegisterContext registers a function that will be called
// to set up a context before any hook function execution.
func (r *Registry) RegisterContext(setter ContextSetter) {
	*r.contexts = append(*r.contexts, func(ctxt *Context) error {
		return setter(ctxt.withLocalStateName(r.localStateName))
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

// RegisterRelation registers a relation to be included in the charm's
// metadata.yaml. If a relation is registered twice with the same
// name, all of the details must also match.
// If the relation's scope is empty, charm.ScopeGlobal
// is assumed. If rel.Limit is zero, it is assumed to be 1
// if the role is charm.RolePeer or charm.RoleRequirer.
func (r *Registry) RegisterRelation(rel charm.Relation) {
	if rel.Name == "" {
		panic(fmt.Errorf("no relation name given in %#v", rel))
	}
	if rel.Interface == "" {
		panic(fmt.Errorf("no interface name given in relation %#v", rel))
	}
	if rel.Role == "" {
		panic(fmt.Errorf("no role given in relation %#v", rel))
	}
	if rel.Limit == 0 && (rel.Role == charm.RolePeer || rel.Role == charm.RoleRequirer) {
		rel.Limit = 1
	}
	if rel.Scope == "" {
		rel.Scope = charm.ScopeGlobal
	}
	old, ok := r.relations[rel.Name]
	if ok {
		if old != rel {
			panic(errors.Newf("relation %q is already registered with different details (%#v)", rel.Name, old))
		}
		return
	}
	r.relations[rel.Name] = rel
}

// RegisterConfig registers a configuration option to be included in
// the charm's config.yaml. If an option is registered twice with the
// same name, all of the details must also match.
func (r *Registry) RegisterConfig(name string, opt charm.Option) {
	old, ok := r.config[name]
	if ok {
		if old != opt {
			panic(errors.Newf("configuration option %q is already registered with different details (%#v)", name, old))
		}
		return
	}
	r.config[name] = opt
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
		relations:      r.relations,
		config:         r.config,
		contexts:       r.contexts,
	}
}

// RegisteredHooks returns the names of all currently
// registered hooks, excluding wildcard ("*") hooks.
func (r *Registry) RegisteredHooks() []string {
	var names []string
	for name := range r.hooks {
		if name != "*" {
			names = append(names, name)
		}
	}
	return names
}

// RegisteredRelations returns relations that have been
// registered with RegisterRelation, keyed by relation name.
func (r *Registry) RegisteredRelations() map[string]charm.Relation {
	return r.relations
}

// RegisteredConfig returns the configuration options
// that have been registered with RegisterConfig.
func (r *Registry) RegisteredConfig() map[string]charm.Option {
	return r.config
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
	sort.Strings(allowed[0:len(r.commands)])
	sort.Strings(allowed[len(r.commands):])
	return errors.Newf("usage: runhook %s", strings.Join(allowed, "\n\t| runhook "))
}

// Main creates a new context from the environment and invokes the
// appropriate command or hook functions from the given
// registry or sub-registries of it.
//
// This function is designed to be called by gocharm
// generated code only.
func Main(r *Registry) (err error) {
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

	// Notify everyone about the context.
	for _, ctxtf := range *r.contexts {
		if err := ctxtf(ctxt); err != nil {
			return errors.Wrapf(err, "cannot set context")
		}
	}
	defer func() {
		// All the hooks have now run; save the state.
		if saveErr := ctxt.saveState(); saveErr != nil {
			if err == nil {
				err = errors.Wrapf(saveErr, "cannot save local state")
			} else {
				ctxt.Logf("cannot save local state: %v", saveErr)
			}
		}
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
	var ctxt *Context
	r.RegisterContext(func(hctxt *Context) error {
		ctxt = hctxt
		return nil
	})
	// Ensure that we have a stop hook and that
	// it is called after every other hook, including
	// wildcard hooks.
	r.RegisterHook("stop", nop)
	r.RegisterHook("*", func() error {
		// We've shut down, so clean up all our local state.
		if err := os.RemoveAll(ctxt.StateDir()); err != nil {
			return errors.Wrapf(err, "cannot remove local state")
		}
		return nil
	})
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
