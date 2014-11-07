package hook_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/juju/worker/uniter/context/jujuc"
	"launchpad.net/errgo/errors"
	gc "launchpad.net/gocheck"

	"gopkg.in/juju-utils.v0/hook"
)

type HookSuite struct {
	sockPath          string
	srvCtxt           *ServerContext
	server            *jujuc.Server
	err               chan error
	savedVars         map[string]string
	savedArgs         []string
	savedHookStateDir string
}

var _ = gc.Suite(&HookSuite{})

// StartServer starts the jujuc server going with the
// given relation id and remote unit and sets
// up the environment for hook.NewContext.
// The server context is stored in s.srvCtxt.
func (s *HookSuite) StartServer(c *gc.C, relid int, remote string) {
	// can't start server twice.
	c.Assert(s.server, gc.IsNil)

	s.srvCtxt = GetHookServerContext(c, relid, remote)

	s.sockPath = filepath.Join(c.MkDir(), "test.sock")
	factory := func(ctxtId, cmdName string) (cmd.Command, error) {
		if ctxtId != "testcontext" {
			return nil, fmt.Errorf("incorrect context %q", ctxtId)
		}
		return jujuc.NewCommand(s.srvCtxt, cmdName)
	}
	srv, err := jujuc.NewServer(factory, s.sockPath)
	c.Assert(err, gc.IsNil)
	c.Assert(srv, gc.NotNil)
	s.server = srv
	s.err = make(chan error)
	go func() { s.err <- s.server.Run() }()

	s.setenv("JUJU_CONTEXT_ID", "testcontext")
	s.setenv("JUJU_AGENT_SOCKET", s.sockPath)
	s.setenv("JUJU_UNIT_NAME", "local/55")
	s.setenv("JUJU_ENV_UUID", "fff.fff.fff")

	charmDir := filepath.Join(c.MkDir(), "charmdir")
	err = os.Mkdir(charmDir, 0777)
	c.Assert(err, gc.IsNil)
	s.setenv("CHARM_DIR", charmDir)

	if r, found := s.srvCtxt.HookRelation(); found {
		remoteName, _ := s.srvCtxt.RemoteUnitName()
		s.setenv("JUJU_RELATION", r.Name())
		s.setenv("JUJU_RELATION_ID", r.FakeId())
		s.setenv("JUJU_REMOTE_UNIT", remoteName)
	}
}

func (s *HookSuite) SetUpTest(c *gc.C) {
	c.Assert(s.savedVars, gc.HasLen, 0)
	s.savedVars = make(map[string]string)
	s.savedArgs = os.Args
	os.Args = nil
	s.savedHookStateDir = *hook.HookStateDir
	*hook.HookStateDir = c.MkDir()
}

func (s *HookSuite) TearDownTest(c *gc.C) {
	for key, val := range s.savedVars {
		os.Setenv(key, val)
		delete(s.savedVars, key)
	}
	if s.server != nil {
		s.server.Close()
		c.Assert(<-s.err, gc.IsNil)
		s.server = nil
	}
	os.Args = s.savedArgs
	*hook.HookStateDir = s.savedHookStateDir
}

func (s *HookSuite) setenv(key, val string) {
	if _, ok := s.savedVars[key]; !ok {
		s.savedVars[key] = os.Getenv(key)
	}
	os.Setenv(key, val)
}

func (s *HookSuite) newContext(c *gc.C, args ...string) *hook.Context {
	os.Args = append([]string{"runhook"}, args...)
	ctxt, err := hook.NewContext()
	c.Assert(err, gc.IsNil)
	return ctxt
}

func (s *HookSuite) TestSimple(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	os.Args = []string{"runhook", "peer-relation-changed"}
	ctxt, err := hook.NewContext()
	c.Assert(err, gc.IsNil)
	defer ctxt.Close()

	c.Check(ctxt.HookName, gc.Equals, "peer-relation-changed")
	c.Check(ctxt.UUID, gc.Equals, "fff.fff.fff")
	c.Check(ctxt.Unit, gc.Equals, "local/55")
	c.Check(ctxt.CharmDir, gc.Matches, ".*/charmdir")
	c.Check(ctxt.RelationName, gc.Equals, "peer0")
	c.Check(ctxt.RelationId, gc.Equals, "peer0:0")
	c.Check(ctxt.RemoteUnit, gc.Equals, "peer0/0")

	// should really check false but annoying to do
	// and too trivial to be worth it.
	c.Assert(ctxt.IsRelationHook(), gc.Equals, true)

	addr, err := ctxt.PrivateAddress()
	c.Check(err, gc.IsNil)
	c.Check(addr, gc.Equals, "192.168.0.99")

	addr, err = ctxt.PublicAddress()
	c.Check(err, gc.IsNil)
	c.Check(addr, gc.Equals, "gimli.minecraft.example.com")
}

// TODO(rog) test methods that make changes!
// TestOpenPort
// TestClosePort
// TestSetRelation
// TestSetRelationWithId

func (s *HookSuite) TestLocalState(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	type fooState struct {
		Foo int
		Bar string
	}
	var state *fooState
	err := ctxt.LocalState("foo", &state)
	c.Assert(err, gc.IsNil)
	c.Assert(state, gc.DeepEquals, &fooState{})

	var state1 *fooState
	err = ctxt.LocalState("foo", &state1)
	c.Assert(err, gc.IsNil)
	c.Assert(state1, gc.Equals, state)

	state.Foo = 88
	state.Bar = "xxx"
	err = ctxt.SaveState()
	c.Assert(err, gc.IsNil)

	ctxt = s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	var newState *fooState
	err = ctxt.LocalState("foo", &newState)
	c.Assert(err, gc.IsNil)
	c.Assert(newState, gc.DeepEquals, state)

	newState.Foo = 88
	err = ctxt.SaveState()
	c.Assert(err, gc.IsNil)

	data, err := ioutil.ReadFile(filepath.Join(ctxt.StateDir(), "foo.json"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, `{"Foo":88,"Bar":"xxx"}`)
}

func (s *HookSuite) TestContextGetter(c *gc.C) {
	// TODO
}

func (s *HookSuite) TestGetRelation(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.GetRelation("private-address")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, "peer0-0.example.com")
}

func (s *HookSuite) TestGetRelationUnit(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.GetRelationUnit("peer1:1", "peer1/1", "private-address")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, "peer1-1.example.com")
}

func (s *HookSuite) TestGetRelationUnitUnknown(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()
	val, err := ctxt.GetRelationUnit("unknown:99", "peer1/1", "private-address")
	c.Check(val, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `invalid value "unknown:99" for flag -r: unknown relation id`)
}

func (s *HookSuite) TestGetAllRelation(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.GetAllRelation()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string]string{"private-address": "peer0-0.example.com"})
}

func (s *HookSuite) TestGetAllRelationUnit(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.GetAllRelationUnit("peer1:1", "peer1/1")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string]string{"private-address": "peer1-1.example.com"})
}

func (s *HookSuite) TestRelationIds(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.RelationIds("peer0")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, []string{"peer0:0"})
}

func (s *HookSuite) TestRelationUnits(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.RelationUnits("peer1:1")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, []string{"peer1/0", "peer1/1"})
}

func (s *HookSuite) TestAllRelationUnits(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.AllRelationUnits("peer0")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string][]string{
		"peer0:0": {"peer0/0", "peer0/1"},
	})
}

func (s *HookSuite) TestGetConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.GetConfig("spline-reticulation")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, 45.0)

	val, err = ctxt.GetConfig("unknown")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, nil)
}

func (s *HookSuite) TestGetAllConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := ctxt.GetAllConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	})
}

func (s *HookSuite) TestRegister(c *gc.C) {
	r := hook.NewRegistry()
	r.Register("install", func(ctxt *hook.Context) error {
		return nil
	})
	c.Assert(r.RegisteredHooks(), gc.DeepEquals, []string{"install"})
}

func (s *HookSuite) TestMain(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	called := false
	r := hook.NewRegistry()
	r.Register("peer-relation-changed", func(ctxt *hook.Context) error {
		called = true
		var localState *string
		err := ctxt.LocalState("x", &localState)
		c.Check(err, gc.IsNil)
		*localState = "value"
		val, err := ctxt.GetRelationUnit("peer1:1", "peer1/1", "private-address")
		c.Check(err, gc.IsNil)
		c.Check(val, gc.Equals, "peer1-1.example.com")
		return nil
	})
	os.Args = []string{"exe", "peer-relation-changed"}
	err := hook.Main(r)
	c.Assert(err, gc.IsNil)

	// Check that the local state has been saved.
	ctxt, err := hook.NewContext()
	c.Assert(err, gc.IsNil)
	defer ctxt.Close()
	var localState *string
	err = ctxt.LocalState("x", &localState)
	c.Assert(err, gc.IsNil)
	c.Assert(*localState, gc.Equals, "value")
}

func (s *HookSuite) TestMainFailsWhenCannotSaveState(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	r := hook.NewRegistry()
	r.Register("peer-relation-changed", func(ctxt *hook.Context) error {
		var x *int
		if err := ctxt.LocalState("x", &x); err != nil {
			return errors.Wrap(err)
		}
		err := os.Chmod(*hook.HookStateDir, 0)
		c.Check(err, gc.IsNil)
		return nil
	})
	os.Args = []string{"exe", "peer-relation-changed"}
	err := hook.Main(r)
	c.Logf("err info: %v", errors.Info(err))
	c.Assert(err, gc.ErrorMatches, "cannot save local state: .*")
}

func (s *HookSuite) TestMainCleansUpLocalStateOnStopHook(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	r := hook.NewRegistry()
	r1 := r.NewRegistry("sub")
	r1.Register("peer-relation-changed", func(ctxt *hook.Context) error {
		var x *int
		return ctxt.LocalState("x", &x)
	})
	// Create some local state.
	os.Args = []string{"exe", "peer-relation-changed"}
	err := hook.Main(r)
	c.Assert(err, gc.IsNil)

	// Check that the state directory exists.
	ctxt, err := hook.NewContext()
	c.Assert(err, gc.IsNil)
	defer ctxt.Close()
	_, err = os.Stat(ctxt.StateDir())
	c.Assert(err, gc.IsNil)

	// Run the stop hook and check that the state directory has been removed.
	os.Args = []string{"exe", "stop"}
	err = hook.Main(r)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(ctxt.StateDir())
	c.Assert(os.IsNotExist(err), gc.Equals, true)

	// Check that the top level localstate directory still exists, but
	// is now empty.
	infos, err := ioutil.ReadDir(*hook.HookStateDir)
	c.Assert(err, gc.IsNil)
	c.Assert(infos, gc.HasLen, 0)
}

func (s *HookSuite) TestMainWithEmptyRegistry(c *gc.C) {
	err := hook.Main(hook.NewRegistry())
	c.Assert(err, gc.ErrorMatches, "no registered hooks or commands")
}

func (s *HookSuite) TestMainUsage(c *gc.C) {
	r := hook.NewRegistry()
	r.RegisterCommand("server", func() {})
	r.RegisterCommand("client", func() {})
	r.Register("peer-relation-changed", func(*hook.Context) error { return nil })
	r.Register("config-changed", func(*hook.Context) error { return nil })
	os.Args = []string{"exe"}
	err := hook.Main(r)
	c.Assert(err, gc.ErrorMatches, `usage: runhook cmd-server \[arg\.\.\.]
	| runhook cmd-client \[arg\.\.\.]
	| runhook config-changed
	| runhook peer-relation-changed`)
}

func (s *HookSuite) TestCommandCall(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	r := hook.NewRegistry()
	callArgs := make(map[string][]string)
	cmdFunc := func(name string) func() {
		return func() {
			callArgs[name] = append([]string{}, os.Args...)
		}
	}

	r.RegisterCommand("main", cmdFunc("main"))
	r.RegisterCommand("other", cmdFunc("other"))
	r1 := r.NewRegistry("sub")
	r1.RegisterCommand("main", cmdFunc("main-sub"))

	var cmdNames []string
	r.Register("install", func(ctxt *hook.Context) error {
		cmdNames = append(cmdNames,
			ctxt.CommandName("main"),
			ctxt.CommandName("other"),
		)
		return nil
	})
	r1.Register("install", func(ctxt *hook.Context) error {
		cmdNames = append(cmdNames,
			ctxt.CommandName("main"),
		)
		return nil
	})
	// invoke install hook, so we can find out the
	// reported command names.,
	os.Args = []string{"exe", "install"}
	err := hook.Main(r)
	c.Assert(err, gc.IsNil)
	c.Assert(cmdNames, gc.HasLen, 3)

	for _, name := range cmdNames {
		os.Args = []string{"exe", name, "arg1", "arg2"}
		err := hook.Main(r)
		c.Assert(err, gc.IsNil)
	}

	c.Assert(callArgs, gc.DeepEquals, map[string][]string{
		"main":     {"exe", "arg1", "arg2"},
		"other":    {"exe", "arg1", "arg2"},
		"main-sub": {"exe", "arg1", "arg2"},
	})
}

type localState struct {
	CallCount int
	Name      string
}

var latestState = make(map[string]localState)

func localStateHookFunc(name string) func(*hook.Context) error {
	return func(ctxt *hook.Context) error {
		var state *localState
		if err := ctxt.LocalState(name, &state); err != nil {
			return errors.Wrap(err)
		}
		if state.Name != name && state.Name != "" {
			panic(errors.Newf("unexpected name in state: %q; expected %q", state.Name, name))
		}
		state.Name = name
		state.CallCount++
		latestState[name] = *state
		return nil
	}
}

func registerLevel1(name string, r *hook.Registry) {
	r.Register("config-changed", localStateHookFunc(name+"-0"))
	r.Register("peer-relation-changed", localStateHookFunc(name+"-0"))
}

func registerLevel0(r *hook.Registry) {
	registerLevel1("level1-0", r.NewRegistry("level1-0"))
	registerLevel1("level1-1", r.NewRegistry("level1-1"))
	r.Register("config-changed", localStateHookFunc("level0-config"))
	r.Register("other-relation-changed", localStateHookFunc("level0-other"))
}

func (s *HookSuite) TestHierarchicalLocalState(c *gc.C) {
	latestState = make(map[string]localState)

	s.StartServer(c, 0, "peer0/0")

	r := hook.NewRegistry()
	registerLevel0(r)
	os.Args = []string{"", "config-changed"}
	for i := 0; i < 3; i++ {
		err := hook.Main(r)
		c.Assert(err, gc.IsNil)
	}
	os.Args = []string{"", "peer-relation-changed"}
	for i := 0; i < 2; i++ {
		err := hook.Main(r)
		c.Assert(err, gc.IsNil)
	}
	os.Args = []string{"", "other-relation-changed"}
	err := hook.Main(r)
	c.Assert(err, gc.IsNil)

	c.Assert(latestState, gc.DeepEquals, map[string]localState{
		"level1-0-0": {
			CallCount: 5,
			Name:      "level1-0-0",
		},
		"level1-1-0": {
			CallCount: 5,
			Name:      "level1-1-0",
		},
		"level0-config": {
			CallCount: 3,
			Name:      "level0-config",
		},
		"level0-other": {
			CallCount: 1,
			Name:      "level0-other",
		},
	})
}

func (s *HookSuite) TestMainWithUnregisteredHook(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	r := hook.NewRegistry()
	r.Register("install", func(*hook.Context) error { return nil })
	os.Args = []string{"exe", "peer-relation-changed"}
	err := hook.Main(r)
	c.Assert(err, gc.ErrorMatches, `usage: runhook install
	| runhook stop`)
}
