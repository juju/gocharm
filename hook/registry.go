package hook

import (
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"
)

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
}

var hooks = make(map[string]func(ctxt *Context) error)

// Register registers a hook to be called when Main
// is called in that hook context.
func Register(name string, f func(ctxt *Context) error) {
	// TODO(rog) implement validName
	//if !validName(name) {
	//	panic(fmt.Errorf("invalid hook name %q", name))
	//}
	if hooks[name] != nil {
		panic(fmt.Errorf("hook %q already registered", name))
	}
	hooks[name] = f
}

// RegisteredHooks returns the names of all currently
// registered hooks.
func RegisteredHooks() []string {
	var names []string
	for name := range hooks {
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

// Main creates a new context and invokes the appropriate
// registered hook function.
func Main() error {
	ctxt, err := NewContext()
	if err != nil {
		return err
	}
	defer ctxt.Close()
	f := hooks[ctxt.HookName]
	if f == nil {
		return fmt.Errorf("hook %q not registered", ctxt.HookName)
	}
	return f(ctxt)
}

// NewContext creates a hook context from the current environment.
// Clients should not use this function, but use their init functions to
// call Register to register a hook function instead, which enables
// gocharm to generate hook stubs automatically.
func NewContext() (*Context, error) {
	vars := mustEnvVars
	if os.Getenv(envRelationName) != "" {
		vars = append(vars, relationEnvVars...)
	}
	for _, v := range vars {
		if os.Getenv(v) == "" {
			return nil, fmt.Errorf("required environment variable %q not set", v)
		}
	}
	_, hookName := filepath.Split(os.Args[0])
	ctxt := &Context{
		UUID:           os.Getenv(envUUID),
		Unit:           os.Getenv(envUnitName),
		CharmDir:       os.Getenv(envCharmDir),
		RelationName:   os.Getenv(envRelationName),
		RelationId:     os.Getenv(envRelationId),
		RemoteUnit:     os.Getenv(envRemoteUnit),
		HookName:       hookName,
		jujucContextId: os.Getenv(envJujuContextId),
	}
	client, err := rpc.Dial("unix", os.Getenv(envSocketPath))
	if err != nil {
		return nil, fmt.Errorf("cannot dial uniter: %v", err)
	}
	ctxt.jujucClient = client
	return ctxt, nil
}

// Close closes the context's connection to the unit agent.
func (ctxt *Context) Close() error {
	return ctxt.jujucClient.Close()
}
