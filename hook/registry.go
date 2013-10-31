package hook

import (
	"fmt"
	"launchpad.net/errgo/errors"
	"net/rpc"
	"os"
)

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
func Main() (err error) {
	ctxt, err := NewContext()
	if err != nil {
		return errors.Wrap(err)
	}
	defer ctxt.Close()
	f := hooks[ctxt.HookName]
	if f == nil {
		return errors.Newf("hook %q not registered", ctxt.HookName)
	}
	defer func() {
		if saveErr := ctxt.SaveState(); saveErr != nil {
			if err == nil {
				err = saveErr
			} else {
				ctxt.Logf("cannot save local state: %v", saveErr)
			}
		}
	}()
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
		localState:     make(map[string]localState),
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
