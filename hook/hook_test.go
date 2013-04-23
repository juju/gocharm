package hook_test

import (
	"fmt"
	. "launchpad.net/gocheck"
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

var _ = Suite(&HookSuite{})

// StartServer starts the jujuc server going with the
// given relation id and remote unit.
// The server context is stored in s.srvCtxt.
// The hook context is stored in s.ctxt.
func (s *HookSuite) StartServer(c *C, relid int, remote string) {
	// can't start server twice.
	c.Assert(s.server, IsNil)

	s.srvCtxt = GetHookServerContext(c, relid, remote)

	s.sockPath = filepath.Join(c.MkDir(), "test.sock")
	factory := func(ctxtId, cmdName string) (cmd.Command, error) {
		if ctxtId != "testcontext" {
			return nil, fmt.Errorf("incorrect context %q", ctxtId)
		}
		return jujuc.NewCommand(s.srvCtxt, cmdName)
	}
	srv, err := jujuc.NewServer(factory, s.sockPath)
	c.Assert(err, IsNil)
	c.Assert(srv, NotNil)
	s.server = srv
	s.err = make(chan error)
	go func() { s.err <- s.server.Run() }()

	s.setenv("JUJU_CONTEXT_ID", "testcontext")
	s.setenv("JUJU_AGENT_SOCKET", s.sockPath)
	s.setenv("JUJU_UNIT_NAME", "local/55")
	s.setenv("JUJU_ENV_UUID", "fff.fff.fff")
	s.setenv("CHARM_DIR", "/charm/dir")

	if r, found := s.srvCtxt.HookRelation(); found {
		remoteName, _ := s.srvCtxt.RemoteUnitName()
		s.setenv("JUJU_RELATION", r.Name())
		s.setenv("JUJU_RELATION_ID", r.FakeId())
		s.setenv("JUJU_REMOTE_UNIT", remoteName)
	}
	s.ctxt, err = hook.NewContext()
	c.Assert(err, IsNil)
}

func (s *HookSuite) SetUpTest(c *C) {
	c.Assert(s.savedVars, HasLen, 0)
	s.savedVars = make(map[string]string)
	s.savedArgs = os.Args
	os.Args = []string{"/foo/bar/peer-relation-changed"}
}

func (s *HookSuite) TearDownTest(c *C) {
	for key, val := range s.savedVars {
		os.Setenv(key, val)
		delete(s.savedVars, key)
	}
	if s.server != nil {
		err := s.ctxt.Close()
		c.Check(err, IsNil)
		s.server.Close()
		c.Assert(<-s.err, IsNil)
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

func (s *HookSuite) TestSimple(c *C) {
	s.StartServer(c, 0, "peer0/0")

	c.Check(s.ctxt.HookName, Equals, "peer-relation-changed")
	c.Check(s.ctxt.UUID, Equals, "fff.fff.fff")
	c.Check(s.ctxt.Unit, Equals, "local/55")
	c.Check(s.ctxt.CharmDir, Equals, "/charm/dir")
	c.Check(s.ctxt.RelationName, Equals, "peer0")
	c.Check(s.ctxt.RelationId, Equals, "peer0:0")
	c.Check(s.ctxt.RemoteUnit, Equals, "peer0/0")

	// should really check false but annoying to do
	// and too trivial to be worth it.
	c.Assert(s.ctxt.IsRelationHook(), Equals, true)

	addr, err := s.ctxt.PrivateAddress()
	c.Check(err, IsNil)
	c.Check(addr, Equals, "192.168.0.99")

	addr, err = s.ctxt.PublicAddress()
	c.Check(err, IsNil)
	c.Check(addr, Equals, "gimli.minecraft.example.com")
}

// TODO(rog) test methods that make changes!
// TestOpenPort
// TestClosePort
// TestSetRelation
// TestSetRelationWithId

func (s *HookSuite) TestGetRelation(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetRelation("private-address")
	c.Assert(err, IsNil)
	c.Assert(val, Equals, "peer0-0.example.com")
}

func (s *HookSuite) TestGetRelationUnit(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetRelationUnit("private-address", "peer1:1", "peer1/1")
	c.Assert(err, IsNil)
	c.Assert(val, Equals, "peer1-1.example.com")
}

func (s *HookSuite) TestGetRelationUnitUnknown(c *C) {
	s.StartServer(c, 0, "peer0/0")
	val, err := s.ctxt.GetRelationUnit("private-address", "unknown:99", "peer1/1")
	c.Check(val, Equals, "")
	c.Assert(err, ErrorMatches, `invalid value "unknown:99" for flag -r: unknown relation id`)
}

func (s *HookSuite) TestGetAllRelation(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetAllRelation()
	c.Assert(err, IsNil)
	c.Assert(val, DeepEquals, map[string]string{"private-address": "peer0-0.example.com"})
}

func (s *HookSuite) TestGetAllRelationUnit(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetAllRelationUnit("peer1:1", "peer1/1")
	c.Assert(err, IsNil)
	c.Assert(val, DeepEquals, map[string]string{"private-address": "peer1-1.example.com"})
}

func (s *HookSuite) TestRelationIds(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.RelationIds("peer0")
	c.Assert(err, IsNil)
	c.Assert(val, DeepEquals, []string{"peer0:0"})
}

func (s *HookSuite) TestRelationUnits(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.RelationUnits("peer1:1")
	c.Assert(err, IsNil)
	c.Assert(val, DeepEquals, []string{"peer1/0", "peer1/1"})
}

func (s *HookSuite) TestAllRelationUnits(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.AllRelationUnits("peer0")
	c.Assert(err, IsNil)
	c.Assert(val, DeepEquals, map[string][]string{
		"peer0:0": {"peer0/0", "peer0/1"},
	})
}

func (s *HookSuite) TestGetConfig(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetConfig("spline-reticulation")
	c.Assert(err, IsNil)
	c.Assert(val, Equals, 45.0)

	val, err = s.ctxt.GetConfig("unknown")
	c.Assert(err, IsNil)
	c.Assert(val, Equals, nil)
}

func (s *HookSuite) TestGetAllConfig(c *C) {
	s.StartServer(c, 0, "peer0/0")

	val, err := s.ctxt.GetAllConfig()
	c.Assert(err, IsNil)
	c.Assert(val, DeepEquals, map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	})
}

func (s *HookSuite) TestRegister(c *C) {
	defer hook.ClearRegistry()
	hook.Register("install", func(ctxt *hook.Context) error {
		return nil
	})
	c.Assert(hook.RegisteredHooks(), DeepEquals, []string{"install"})
}

func (s *HookSuite) TestMain(c *C) {
	defer hook.ClearRegistry()
	s.StartServer(c, 0, "peer0/0")
	called := false
	hook.Register("peer-relation-changed", func(ctxt *hook.Context) error {
		called = true
		val, err := ctxt.GetRelationUnit("private-address", "peer1:1", "peer1/1")
		c.Check(err, IsNil)
		c.Check(val, Equals, "peer1-1.example.com")
		return nil
	})
	err := hook.Main()
	c.Assert(err, IsNil)
}

func (s *HookSuite) TestMainWithUnregisteredHook(c *C) {
	s.StartServer(c, 0, "peer0/0")
	err := hook.Main()
	c.Assert(err, ErrorMatches, `hook "peer-relation-changed" not registered`)
}
