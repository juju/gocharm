package hook_test

import (
	"fmt"
	"io/ioutil"
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"launchpad.net/juju-utils/hook"
	"os"
	"path/filepath"
)

type HookSuite struct {
	sockPath  string
	ctxt      *hook.Context
	srvCtxt   *ServerContext
	server    *jujuc.Server
	err       chan error
	savedVars map[string]string
	savedArgs []string
}

var _ = gc.Suite(&HookSuite{})

// StartServer starts the jujuc server going with the
// given relation id and remote unit.
// The server context is stored in s.srvCtxt.
// The hook context is stored in s.ctxt.
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
	s.ctxt, err = hook.NewContext()
	c.Assert(err, gc.IsNil)
}

func (s *HookSuite) SetUpTest(c *gc.C) {
	c.Assert(s.savedVars, gc.HasLen, 0)
	s.savedVars = make(map[string]string)
	s.savedArgs = os.Args
	os.Args = []string{os.Args[0], "peer-relation-changed"}
}

func (s *HookSuite) TearDownTest(c *gc.C) {
	for key, val := range s.savedVars {
		os.Setenv(key, val)
		delete(s.savedVars, key)
	}
	if s.server != nil {
		err := s.ctxt.Close()
		c.Check(err, gc.IsNil)
		s.server.Close()
		c.Assert(<-s.err, gc.IsNil)
		s.server = nil
	}
	os.Args = s.savedArgs
}

func (s *HookSuite) setenv(key, val string) {
	if _, ok := s.savedVars[key]; !ok {
		s.savedVars[key] = os.Getenv(key)
	}
	os.Setenv(key, val)
}

func (s *HookSuite) TestSimple(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	c.Check(s.ctxt.HookName, gc.Equals, "peer-relation-changed")
	c.Check(s.ctxt.UUID, gc.Equals, "fff.fff.fff")
	c.Check(s.ctxt.Unit, gc.Equals, "local/55")
	c.Check(s.ctxt.CharmDir, gc.Matches, ".*/charmdir")
	c.Check(s.ctxt.RelationName, gc.Equals, "peer0")
	c.Check(s.ctxt.RelationId, gc.Equals, "peer0:0")
	c.Check(s.ctxt.RemoteUnit, gc.Equals, "peer0/0")

	// should really check false but annoying to do
	// and too trivial to be worth it.
	c.Assert(s.ctxt.IsRelationHook(), gc.Equals, true)

	addr, err := s.ctxt.PrivateAddress()
	c.Check(err, gc.IsNil)
	c.Check(addr, gc.Equals, "192.168.0.99")

	addr, err = s.ctxt.PublicAddress()
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

	type fooState struct {
		Foo int
		Bar string
	}
	var state fooState
	err := s.ctxt.LocalState("foo", &state)
	c.Assert(err, gc.IsNil)
	c.Assert(state, gc.Equals, fooState{})

	c.Assert(func() {
		s.ctxt.LocalState("foo", &state)
	}, gc.PanicMatches, "LocalState called twice for \"foo\"")

	state.Foo = 88
	state.Bar = "xxx"
	err = s.ctxt.SaveState()
	c.Assert(err, gc.IsNil)

	ctxt, err := hook.NewContext()
	c.Assert(err, gc.IsNil)
	defer ctxt.Close()

	var newState fooState
	err = ctxt.LocalState("foo", &newState)
	c.Assert(err, gc.IsNil)
	c.Assert(newState, gc.Equals, state)

	newState.Foo = 88
	err = ctxt.SaveState()
	c.Assert(err, gc.IsNil)

	data, err := ioutil.ReadFile(filepath.Join(s.ctxt.CharmDir, "localstate", "foo"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, `{"Foo":88,"Bar":"xxx"}`)
}

func (s *HookSuite) TestGetRelation(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetRelation("private-address")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, "peer0-0.example.com")
}

func (s *HookSuite) TestGetRelationUnit(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetRelationUnit("peer1:1", "peer1/1", "private-address")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, "peer1-1.example.com")
}

func (s *HookSuite) TestGetRelationUnitUnknown(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	val, err := s.ctxt.GetRelationUnit("unknown:99", "peer1/1", "private-address")
	c.Check(val, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `invalid value "unknown:99" for flag -r: unknown relation id`)
}

func (s *HookSuite) TestGetAllRelation(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetAllRelation()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string]string{"private-address": "peer0-0.example.com"})
}

func (s *HookSuite) TestGetAllRelationUnit(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetAllRelationUnit("peer1:1", "peer1/1")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string]string{"private-address": "peer1-1.example.com"})
}

func (s *HookSuite) TestRelationIds(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.RelationIds("peer0")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, []string{"peer0:0"})
}

func (s *HookSuite) TestRelationUnits(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.RelationUnits("peer1:1")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, []string{"peer1/0", "peer1/1"})
}

func (s *HookSuite) TestAllRelationUnits(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.AllRelationUnits("peer0")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string][]string{
		"peer0:0": {"peer0/0", "peer0/1"},
	})
}

func (s *HookSuite) TestGetConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetConfig("spline-reticulation")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, 45.0)

	val, err = s.ctxt.GetConfig("unknown")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, nil)
}

func (s *HookSuite) TestGetAllConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetAllConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	})
}

func (s *HookSuite) TestRegister(c *gc.C) {
	defer hook.ClearRegistry()
	hook.Register("install", func(ctxt *hook.Context) error {
		return nil
	})
	c.Assert(hook.RegisteredHooks(), gc.DeepEquals, []string{"install"})
}

func (s *HookSuite) TestMain(c *gc.C) {
	defer hook.ClearRegistry()
	s.StartServer(c, 0, "peer0/0")
	called := false
	hook.Register("peer-relation-changed", func(ctxt *hook.Context) error {
		called = true
		localState := "value"
		err := ctxt.LocalState("x", &localState)
		c.Check(err, gc.IsNil)
		val, err := ctxt.GetRelationUnit("peer1:1", "peer1/1", "private-address")
		c.Check(err, gc.IsNil)
		c.Check(val, gc.Equals, "peer1-1.example.com")
		return nil
	})
	err := hook.Main()
	c.Assert(err, gc.IsNil)

	// Check that the local state has been saved.
	ctxt, err := hook.NewContext()
	c.Assert(err, gc.IsNil)
	var localState string
	err = ctxt.LocalState("x", &localState)
	c.Assert(err, gc.IsNil)
	c.Assert(localState, gc.Equals, "value")
}

func (s *HookSuite) TestMainWithUnregisteredHook(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	err := hook.Main()
	c.Assert(err, gc.ErrorMatches, `hook "peer-relation-changed" not registered`)
}
