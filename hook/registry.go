package hook

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"
)

// ContextSetter is the type of a function that can
// set the context for a hook. Usually this is
// a method that sets a context variable inside a struct.
type ContextSetter func(ctxt *Context) error

// Registry allows the registration of hook functions.
type Registry struct {
	name string

	// hasContext and hasCommand record whether the
	// RegisterContext and/or RegisterCommand have
	// been called for this context.
	hasContext bool
	hasCommand bool

	// clones stores an entry for each cloned name.
	clones map[string]bool

	*sharedRegistry
}

// sharedRegistry holds registry values that
// are shared across all clones of a Registry.
type sharedRegistry struct {
	hooks     map[string][]hookFunc
	commands  map[string]func([]string) (Command, error)
	relations map[string]charm.Relation
	config    map[string]charm.Option
	contexts  []ContextSetter
	state     []localState
}

type hookFunc struct {
	registryName string
	run          func() error
}

// localState holds a registered persistent local state value.
type localState struct {
	registryName string
	val          interface{}
}

// NewRegistry returns a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		name:   "root",
		clones: make(map[string]bool),
		sharedRegistry: &sharedRegistry{
			hooks:     make(map[string][]hookFunc),
			commands:  make(map[string]func([]string) (Command, error)),
			relations: make(map[string]charm.Relation),
			config:    make(map[string]charm.Option),
		},
	}
}

// Clone returns a sub-registry of r with the given name. This
// will use a separate name space for local state and for commands.
// This should be used when passing a registry to an external
// package Register function.
//
// This method may not be called more than once on the same registry
// with the same name. The name must be filesystem safe, and must not
// contain a '.' character or be blank. If either of these two
// conditions occur, Clone will panic.
func (r *Registry) Clone(name string) *Registry {
	if name == "" || strings.Contains(name, ".") || strings.ContainsRune(name, filepath.Separator) {
		panic(errgo.Newf("invalid registry name %q", name))
	}
	if r.clones[name] {
		panic(errgo.Newf("registry name %q registered twice", name))
	}
	r.clones[name] = true
	return &Registry{
		name:           r.name + "." + name,
		clones:         make(map[string]bool),
		sharedRegistry: r.sharedRegistry,
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
	if name != "*" && !validHookName(name) {
		panic(fmt.Errorf("invalid hook name %q", name))
	}
	r.hooks[name] = append(r.hooks[name], hookFunc{
		run:          f,
		registryName: r.name,
	})
}

// RegisterContext registers a function that will be called
// to set up a context before any hook function execution.
//
// If state is non-nil, it should hold a pointer to a value
// that will be used to hold persistent state associated with the
// function. When a hook runs, before the setter function is
// called, any previously saved state is loaded into the value.
// When all hooks have completed, the state is saved, making
// it persistent. The data is saved using JSON.Marshal.
//
// This function may not be called more than once for a given Registry;
// it will panic if it is.
func (r *Registry) RegisterContext(setter ContextSetter, state interface{}) {
	if r.hasContext {
		// TODO if this proves to be a problem, we could save
		// some of the stack from the original invocation, so
		// that we can produce a more useful error message here.
		panic("RegisterContext called more than once")
	}
	r.hasContext = true
	r.contexts = append(r.contexts, func(ctxt *Context) error {
		return setter(ctxt.withRegistryName(r.name))
	})
	if state == nil {
		return
	}
	if reflect.ValueOf(state).Kind() != reflect.Ptr {
		panic(errgo.Newf("state value is not pointer but type %T", state))
	}
	r.state = append(r.state, localState{
		registryName: r.name,
		val:          state,
	})
}

// Command is implemented by running commands
// that implement long-lived services.
type Command interface {
	// Kill requests that the command be stopped.
	// It may be called concurrently with Wait.
	Kill()

	// Wait waits for the command to complete and
	// returns any error encountered when running.
	Wait() error
}

// RegisterCommand registers the given function to be called
// when the hook is invoked with a first argument of "cmd".
// It will panic if it is called more than once in the same Registry.
// The function will be called with any extra arguments passed on
// the command line, without the command name itself.
//
// The function may return a nil Command if it completes immediately
//  or return a Command representing a long-running service.
//
// Note that the function will not be called in hook context,
// so it will not have any of the usual hook context to use.
func (r *Registry) RegisterCommand(f func(args []string) (Command, error)) {
	if r.hasCommand {
		panic(errgo.Newf("command registered twice on registry %s", r.name))
	}
	r.hasCommand = true
	r.commands[r.name] = f
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
			panic(errgo.Newf("relation %q is already registered with different details (%#v)", rel.Name, old))
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
	if !ok {
		r.config[name] = opt
		return
	}
	if old != opt {
		panic(errgo.Newf("configuration option %q is already registered with different details (%#v)", name, old))
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

var relationHookPattern = regexp.MustCompile("^(?:(" + names.RelationSnippet + ")-)?(relation-[a-z]+)$")

var hookNames = map[hooks.Kind]bool{
	hooks.Install:            true,
	hooks.Start:              true,
	hooks.ConfigChanged:      true,
	hooks.UpgradeCharm:       true,
	hooks.Stop:               true,
	hooks.ActionRequested:    true,
	hooks.Action:             true,
	hooks.CollectMetrics:     true,
	hooks.MeterStatusChanged: true,
	hooks.RelationJoined:     true,
	hooks.RelationChanged:    true,
	hooks.RelationDeparted:   true,
	hooks.RelationBroken:     true,
}

func validHookName(s string) bool {
	if m := relationHookPattern.FindStringSubmatch(s); m != nil {
		if m[1] == "" {
			// The user has specified a relation hook name with
			// no relation.
			return false
		}
		s = m[2]
	}
	return hookNames[hooks.Kind(s)]
}
