package hook_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/gocharm/hook"
)

type HookSuite struct {
	sockPath  string
	srvCtxt   *ServerContext
	server    *jujuc.Server
	err       chan error
	savedVars map[string]string
	savedArgs []string
	stateDir  string
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
	if os.Getenv("TEST_EXEC_HOOK_TOOLS") == "1" {
		// Run all tests using jujud as a hook tool. This requires a currently installed
		// jujud executable.
		*hook.ExecHookTools = true
		*hook.JujucSymlinks = false
	}
	c.Assert(s.savedVars, gc.HasLen, 0)
	s.savedVars = make(map[string]string)
	s.savedArgs = os.Args
	s.stateDir = c.MkDir()
	os.Args = nil
}

func (s *HookSuite) TearDownTest(c *gc.C) {
	s.resetEnv(c)
	if s.server != nil {
		s.server.Close()
		c.Assert(<-s.err, gc.IsNil)
		s.server = nil
	}
	os.Args = s.savedArgs
}

func (s *HookSuite) resetEnv(c *gc.C) {
	for key, val := range s.savedVars {
		os.Setenv(key, val)
		delete(s.savedVars, key)
	}
}

func (s *HookSuite) setenv(key, val string) {
	if _, ok := s.savedVars[key]; !ok {
		s.savedVars[key] = os.Getenv(key)
	}
	os.Setenv(key, val)
}

func (s *HookSuite) newContext(c *gc.C, args ...string) *hook.Context {
	r := hook.NewRegistry()
	registerDefaultRelations(r)

	os.Args = append([]string{"runhook"}, args...)
	ctxt, _, err := hook.NewContextFromEnvironment(r, s.stateDir)
	c.Assert(err, gc.IsNil)
	return ctxt
}

func (s *HookSuite) TestSimple(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	c.Check(ctxt.HookName, gc.Equals, "peer-relation-changed")
	c.Check(ctxt.UUID, gc.Equals, "fff.fff.fff")
	c.Check(ctxt.Unit, gc.Equals, hook.UnitId("local/55"))
	c.Check(ctxt.CharmDir, gc.Matches, ".*/charmdir")
	c.Check(ctxt.RelationName, gc.Equals, "peer0")
	c.Check(ctxt.RelationId, gc.Equals, hook.RelationId("peer0:0"))
	c.Check(ctxt.RemoteUnit, gc.Equals, hook.UnitId("peer0/0"))

	// should really check false but annoying to do
	// and too trivial to be worth it.
	c.Assert(ctxt.IsRelationHook(), gc.Equals, true)

	addr, err := ctxt.PrivateAddress()
	c.Check(err, gc.IsNil)
	c.Check(addr, gc.Equals, "192.168.0.99")

	addr, err = ctxt.PublicAddress()
	c.Check(err, gc.IsNil)
	c.Check(addr, gc.Equals, "gimli.minecraft.example.com")

	err = ctxt.SetStatus(hook.StatusMaintenance, "hello, world")
	c.Check(err, gc.IsNil)
	c.Check(s.srvCtxt.status, gc.DeepEquals, jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "hello, world",
	})

	err = ctxt.SetStatus(hook.StatusActive, "")
	c.Check(err, gc.IsNil)
	c.Check(s.srvCtxt.status, gc.DeepEquals, jujuc.StatusInfo{
		Status: "active",
	})
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
	start := func(hookf func(state *fooState)) {
		r := hook.NewRegistry()
		var state fooState
		r.RegisterHook("peer-relation-changed", func() error {
			hookf(&state)
			return nil
		})
		r.RegisterContext(nopContextSetter, &state)
		os.Args = []string{"exe", "peer-relation-changed"}
		ctxt, pstate, err := hook.NewContextFromEnvironment(r, s.stateDir)
		c.Assert(err, gc.IsNil)
		defer ctxt.Close()
		hcmd, err := hook.Main(r, ctxt, pstate)
		c.Assert(err, gc.IsNil)
		c.Assert(hcmd, gc.IsNil)
	}
	start(func(state *fooState) {
		c.Assert(state, gc.DeepEquals, &fooState{})
	})
	start(func(state *fooState) {
		c.Assert(state, gc.DeepEquals, &fooState{})
		state.Foo = 88
		state.Bar = "xxx"
	})
	start(func(state *fooState) {
		c.Assert(state, gc.DeepEquals, &fooState{
			Foo: 88,
			Bar: "xxx",
		})
		state.Foo = 10
	})
	start(func(state *fooState) {
		c.Assert(state, gc.DeepEquals, &fooState{
			Foo: 10,
			Bar: "xxx",
		})
	})
}

func (s *HookSuite) TestContextGetter(c *gc.C) {
	// TODO
}

func (s *HookSuite) TestRelationValuesFromRelationHook(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()
	c.Assert(ctxt.RelationIds, jc.DeepEquals, allRelationIds)
	c.Assert(ctxt.Relations, jc.DeepEquals, allRelationValues)
}

func (s *HookSuite) TestRelationValuesFromNonRelationHook(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "config-changed")
	defer ctxt.Close()
	c.Assert(ctxt.RelationIds, jc.DeepEquals, allRelationIds)
	c.Assert(ctxt.Relations, jc.DeepEquals, allRelationValues)
}

func (s *HookSuite) TestGetAllRelationUnit(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := hook.CtxtGetAllRelationUnit(ctxt, "peer1:1", "peer1/1")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, map[string]string{"private-address": "peer1-1.example.com"})
}

func (s *HookSuite) TestRelationIds(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := hook.CtxtRelationIds(ctxt, "peer0")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, []hook.RelationId{"peer0:0"})
}

func (s *HookSuite) TestRelationUnits(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	val, err := hook.CtxtRelationUnits(ctxt, "peer1:1")
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.DeepEquals, []hook.UnitId{"peer1/0", "peer1/1"})
}

func (s *HookSuite) TestGetFloatConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	var valInterface interface{}
	err := ctxt.GetConfig("spline-reticulation", &valInterface)
	c.Assert(err, gc.IsNil)
	c.Assert(valInterface, gc.Equals, 45.0)

	valFloat64, err := ctxt.GetConfigFloat64("spline-reticulation")
	c.Assert(err, gc.IsNil)
	c.Assert(valFloat64, gc.Equals, 45.0)

	valInt, err := ctxt.GetConfigInt("spline-reticulation")
	c.Assert(err, gc.IsNil)
	c.Assert(valInt, gc.Equals, 45)

	valBool, err := ctxt.GetConfigBool("spline-reticulation")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("spline-reticulation", "bool"))
	c.Assert(valBool, gc.Equals, false)

	valString, err := ctxt.GetConfigString("spline-reticulation")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("spline-reticulation", "string"))
	c.Assert(valString, gc.Equals, "")
}

func badConfigTypeErrorPat(key, type_ string) string {
	return fmt.Sprintf(`cannot get configuration option %q: cannot parse command output ".*": json: cannot unmarshal .* into Go value of type %s`, key, type_)
}

func (s *HookSuite) TestGetStringConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	var valInterface interface{}
	err := ctxt.GetConfig("title", &valInterface)
	c.Assert(err, gc.IsNil)
	c.Assert(valInterface, gc.Equals, "My Title")

	valFloat64, err := ctxt.GetConfigFloat64("title")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("title", "float64"))
	c.Assert(valFloat64, gc.Equals, 0.0)

	valInt, err := ctxt.GetConfigInt("title")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("title", "int"))
	c.Assert(valInt, gc.Equals, 0)

	valBool, err := ctxt.GetConfigBool("title")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("title", "bool"))
	c.Assert(valBool, gc.Equals, false)

	valString, err := ctxt.GetConfigString("title")
	c.Assert(err, gc.IsNil)
	c.Assert(valString, gc.Equals, "My Title")
}

func (s *HookSuite) TestGetBoolConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	var valInterface interface{}
	err := ctxt.GetConfig("monsters", &valInterface)
	c.Assert(err, gc.IsNil)
	c.Assert(valInterface, gc.Equals, true)

	valFloat64, err := ctxt.GetConfigFloat64("monsters")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("monsters", "float64"))
	c.Assert(valFloat64, gc.Equals, 0.0)

	valInt, err := ctxt.GetConfigInt("monsters")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("monsters", "int"))
	c.Assert(valInt, gc.Equals, 0)

	valBool, err := ctxt.GetConfigBool("monsters")
	c.Assert(err, gc.IsNil)
	c.Assert(valBool, gc.Equals, true)

	valString, err := ctxt.GetConfigString("monsters")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("monsters", "string"))
	c.Assert(valString, gc.Equals, "")
}

func (s *HookSuite) TestGetIntConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	var valInterface interface{}
	err := ctxt.GetConfig("red-balloon-count", &valInterface)
	c.Assert(err, gc.IsNil)
	c.Assert(valInterface, gc.Equals, 99.0)

	valFloat64, err := ctxt.GetConfigFloat64("red-balloon-count")
	c.Assert(err, gc.IsNil)
	c.Assert(valFloat64, gc.Equals, 99.0)

	valInt, err := ctxt.GetConfigInt("red-balloon-count")
	c.Assert(err, gc.IsNil)
	c.Assert(valInt, gc.Equals, 99)

	valBool, err := ctxt.GetConfigBool("red-balloon-count")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("red-balloon-count", "bool"))
	c.Assert(valBool, gc.Equals, false)

	valString, err := ctxt.GetConfigString("red-balloon-count")
	c.Assert(err, gc.ErrorMatches, badConfigTypeErrorPat("red-balloon-count", "string"))
	c.Assert(valString, gc.Equals, "")
}

func (s *HookSuite) TestGetAllConfig(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "peer-relation-changed")
	defer ctxt.Close()

	var m map[string]interface{}
	err := ctxt.GetAllConfig(&m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, map[string]interface{}{
		"monsters":            true,
		"spline-reticulation": 45.0,
		"red-balloon-count":   99.0,
		"title":               "My Title",
		"username":            "admin001",
	})

	var cfg struct {
		Monsters *bool  `json:"monsters"`
		Title    string `json:"title"`
		Other    string
	}
	err = ctxt.GetAllConfig(&cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(*cfg.Monsters, gc.Equals, true)
	c.Assert(cfg.Title, gc.Equals, "My Title")
	c.Assert(cfg.Other, gc.Equals, "")
}

func (s *HookSuite) TestRegister(c *gc.C) {
	r := hook.NewRegistry()
	r.RegisterHook("install", func() error { return nil })
	c.Assert(r.RegisteredHooks(), gc.DeepEquals, []string{"install"})
}

var registerRelationTests = []struct {
	about       string
	rel         charm.Relation
	expect      charm.Relation
	expectPanic string
}{{
	about: "requirer limit default = 1, scope default to global",
	rel: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RoleRequirer,
	},
	expect: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RoleRequirer,
		Scope:     charm.ScopeGlobal,
		Limit:     1,
	},
}, {
	about: "provider limit default = 0, scope default to global",
	rel: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RoleProvider,
	},
	expect: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RoleProvider,
		Scope:     charm.ScopeGlobal,
	},
}, {
	about: "peer limit default = 1, scope default to global",
	rel: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RolePeer,
	},
	expect: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Limit:     1,
		Role:      charm.RolePeer,
		Scope:     charm.ScopeGlobal,
	},
}, {
	about: "no defaults",
	rel: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RoleProvider,
		Limit:     2,
		Scope:     charm.ScopeContainer,
		Optional:  true,
	},
	expect: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RoleProvider,
		Limit:     2,
		Scope:     charm.ScopeContainer,
		Optional:  true,
	},
}, {
	about: "no name",
	rel: charm.Relation{
		Interface: "bar",
		Role:      charm.RoleProvider,
		Limit:     2,
		Scope:     charm.ScopeContainer,
		Optional:  true,
	},
	expectPanic: "no relation name given in .*",
}, {
	about: "no interface name",
	rel: charm.Relation{
		Name:     "foo",
		Role:     charm.RoleProvider,
		Limit:    2,
		Scope:    charm.ScopeContainer,
		Optional: true,
	},
	expectPanic: "no interface name given in .*",
}, {
	about: "no role",
	rel: charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Limit:     2,
		Scope:     charm.ScopeContainer,
		Optional:  true,
	},
	expectPanic: "no role given in .*",
}}

func (s *HookSuite) TestRegisterRelation(c *gc.C) {
	all := hook.NewRegistry()
	allExpect := make(map[string]charm.Relation)
	for i, test := range registerRelationTests {
		c.Logf("%d: %s", i, test.about)
		r := hook.NewRegistry()
		if test.expectPanic != "" {
			c.Assert(func() { r.RegisterRelation(test.rel) }, gc.PanicMatches, test.expectPanic)
		} else {
			r.RegisterRelation(test.rel)
			c.Assert(r.RegisteredRelations(), jc.DeepEquals, map[string]charm.Relation{
				test.rel.Name: test.expect,
			})
			rel := test.rel
			rel.Name = fmt.Sprintf("%s%d", test.rel.Name, i)
			all.RegisterRelation(rel)
			expect := test.expect
			expect.Name = rel.Name
			allExpect[expect.Name] = expect
		}
	}
	c.Assert(all.RegisteredRelations(), jc.DeepEquals, allExpect)
}

func (s *HookSuite) TestRegisterSameNameDifferentRelation(c *gc.C) {
	r := hook.NewRegistry()
	rel := charm.Relation{
		Name:      "foo",
		Interface: "bar",
		Role:      charm.RoleRequirer,
	}
	r.RegisterRelation(rel)

	// Check that it's OK to register again with the same relation.
	r.RegisterRelation(rel)

	// Check that it panics when something changes.
	rel.Interface = "baz"
	c.Assert(func() {
		r.RegisterRelation(rel)
	}, gc.PanicMatches, `relation "foo" is already registered with different details .*`)
}

func (s *HookSuite) TestRegisterConfig(c *gc.C) {
	r := hook.NewRegistry()
	opt0 := charm.Option{
		Type:        "string",
		Description: "d",
		Default:     134,
	}
	r.RegisterConfig("opt0", opt0)
	// Check that it's OK to register again with the same option.
	r.RegisterConfig("opt0", opt0)

	opt1 := opt0
	opt1.Default = 135
	c.Assert(func() {
		r.RegisterConfig("opt0", opt1)
	}, gc.PanicMatches, `configuration option "opt0" is already registered with different details .*`)

	r.RegisterConfig("opt1", opt1)

	c.Assert(r.RegisteredConfig(), jc.DeepEquals, map[string]charm.Option{
		"opt0": opt0,
		"opt1": opt1,
	})
}

func (s *HookSuite) TestMain(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	r0 := hook.NewRegistry()
	var localState0 string
	var ctxt0 *hook.Context
	r0.RegisterContext(func(ctxt *hook.Context) error {
		ctxt0 = ctxt
		return nil
	}, &localState0)
	called0 := false
	r0.RegisterHook("peer-relation-changed", func() error {
		called0 = true
		localState0 = "value"
		val, err := ctxt0.GetConfigString("title")
		c.Check(err, gc.IsNil)
		c.Check(val, gc.Equals, "My Title")
		return nil
	})
	os.Args = []string{"exe", "peer-relation-changed"}

	ctxt, state, err := hook.NewContextFromEnvironment(r0, s.stateDir)
	c.Assert(err, gc.IsNil)
	defer ctxt.Close()

	hcmd, err := hook.Main(r0, ctxt, state)
	c.Assert(err, gc.IsNil)
	c.Assert(hcmd, gc.IsNil)
	c.Assert(called0, gc.Equals, true)

	// Check that when the same hook is invoked again, the
	// state is retrieved correctly.
	r1 := hook.NewRegistry()
	var localState1 string
	r1.RegisterContext(nopContextSetter, &localState1)
	called1 := false
	r1.RegisterHook("peer-relation-changed", func() error {
		c.Assert(localState1, gc.Equals, "value")
		called1 = true
		return nil
	})
	err = s.runMain(c, r1)
	c.Assert(err, gc.IsNil)
	c.Assert(called1, gc.Equals, true)
}

func (s *HookSuite) TestWildcardHook(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	r := hook.NewRegistry()
	var called []string
	register := func(name string) {
		r.RegisterHook(name, func() error {
			called = append(called, name)
			return nil
		})
	}
	register("config-changed")
	register("peer0-relation-changed")
	register("*")
	os.Args = []string{"exe", "config-changed"}
	err := s.runMain(c, r)
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.DeepEquals, []string{"config-changed", "*"})

	called = nil
	os.Args = []string{"exe", "peer0-relation-changed"}
	err = s.runMain(c, r)
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.DeepEquals, []string{"peer0-relation-changed", "*"})
}

func (s *HookSuite) TestMainFailsWhenCannotSaveState(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	r := hook.NewRegistry()
	var state int
	r.RegisterContext(nopContextSetter, &state)
	r.RegisterHook("peer-relation-changed", func() error { return nil })
	os.Args = []string{"exe", "peer-relation-changed"}
	ctxt, _, err := hook.NewContextFromEnvironment(r, s.stateDir)
	c.Assert(err, gc.IsNil)
	defer ctxt.Close()
	hcmd, err := hook.Main(r, ctxt, errorState{})
	c.Assert(err, gc.ErrorMatches, "cannot save local state: cannot save state for root: save error")
	c.Assert(hcmd, gc.IsNil)
}

type errorState struct{}

func (errorState) Load(name string) ([]byte, error) {
	return nil, nil
}

func (errorState) Save(name string, data []byte) error {
	return errgo.New("save error")
}

func (s *HookSuite) TestNewContextFromEnvironmentUsage(c *gc.C) {
	r := hook.NewRegistry()
	r.RegisterCommand(nopCommand)
	r.Clone("client").RegisterCommand(nopCommand)
	r.RegisterHook("peer-relation-changed", func() error { return nil })
	r.RegisterHook("config-changed", func() error { return nil })
	os.Args = []string{"exe"}
	_, _, err := hook.NewContextFromEnvironment(r, s.stateDir)
	c.Assert(err, gc.ErrorMatches, `usage: runhook cmd-root \[arg\.\.\.]
	\| runhook cmd-root\.client \[arg\.\.\.]
	\| runhook config-changed
	\| runhook peer-relation-changed`)
}

func nopCommand([]string) (hook.Command, error) {
	return nil, nil
}

func (s *HookSuite) TestRegisterCommandTwice(c *gc.C) {
	r := hook.NewRegistry()
	r.RegisterCommand(nopCommand)
	c.Assert(func() {
		r.RegisterCommand(nopCommand)
	}, gc.PanicMatches, `command registered twice on registry root`)

	r1 := r.Clone("foo")
	r1.RegisterCommand(nopCommand)
	c.Assert(func() {
		r1.RegisterCommand(nopCommand)
	}, gc.PanicMatches, `command registered twice on registry root\.foo`)
}

func (s *HookSuite) TestRegisterContextTwice(c *gc.C) {
	r := hook.NewRegistry()
	var i int
	r.RegisterContext(nopContextSetter, &i)
	c.Assert(func() {
		r.RegisterContext(nopContextSetter, &i)
	}, gc.PanicMatches, `RegisterContext called more than once`)

	r1 := r.Clone("foo")
	r1.RegisterContext(nopContextSetter, &i)
	c.Assert(func() {
		r1.RegisterContext(nopContextSetter, &i)
	}, gc.PanicMatches, `RegisterContext called more than once`)
}

func (s *HookSuite) TestRunUnimplementedCommand(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	ctxt := s.newContext(c, "config-changed")
	defer ctxt.Close()
	_, err := ctxt.Runner.Run("unimplemented-command")
	c.Assert(errgo.Cause(err), gc.Equals, hook.ErrUnimplemented)
}

func (s *HookSuite) TestCommandCall(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	r := hook.NewRegistry()
	callArgs := make(map[string][]string)
	cmdFunc := func(name string) func([]string) (hook.Command, error) {
		return func(args []string) (hook.Command, error) {
			callArgs[name] = append([]string{}, args...)
			return nil, nil
		}
	}

	r.RegisterCommand(cmdFunc("main"))
	r1 := r.Clone("sub")
	r1.RegisterCommand(cmdFunc("main-sub"))

	var cmdNames []string
	registerSimpleHook(r, "install", func(ctxt *hook.Context) error {
		cmdNames = append(cmdNames, ctxt.CommandName())
		return nil
	})
	registerSimpleHook(r1, "install", func(ctxt *hook.Context) error {
		cmdNames = append(cmdNames, ctxt.CommandName())
		return nil
	})
	// invoke install hook, so we can find out the
	// reported command names.
	os.Args = []string{"exe", "install"}
	ctxt := &hook.Context{
		HookName: "install",
		Runner:   nopRunner{},
	}
	hcmd, err := hook.Main(r, ctxt, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(hcmd, gc.IsNil)
	c.Assert(cmdNames, gc.HasLen, 2)

	for i, name := range cmdNames {
		os.Args = []string{"exe", name, fmt.Sprint(i), "arg1", "arg2"}
		ctxt, state, err := hook.NewContextFromEnvironment(r, s.stateDir)
		c.Assert(err, gc.IsNil)
		hcmd, err := hook.Main(r, ctxt, state)
		c.Assert(err, gc.IsNil)
		c.Assert(hcmd, gc.IsNil)
		defer ctxt.Close()
	}

	c.Assert(callArgs, gc.DeepEquals, map[string][]string{
		"main":     {"0", "arg1", "arg2"},
		"main-sub": {"1", "arg1", "arg2"},
	})
}

type fakeCommand struct {
}

func (fakeCommand) Kill() {}

func (fakeCommand) Wait() error {
	return nil
}

func (s *HookSuite) TestLongRunningCommand(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")

	r := hook.NewRegistry()
	r.RegisterCommand(func(args []string) (hook.Command, error) {
		return fakeCommand{}, nil
	})
	var cmdName string
	registerSimpleHook(r, "install", func(ctxt *hook.Context) error {
		cmdName = ctxt.CommandName()
		return nil
	})

	// invoke install hook, so we can find out the
	// reported command name.
	os.Args = []string{"exe", "install"}
	ctxt := &hook.Context{
		HookName: "install",
		Runner:   nopRunner{},
	}
	hcmd, err := hook.Main(r, ctxt, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(hcmd, gc.IsNil)
	c.Assert(cmdName, gc.Not(gc.Equals), "")

	os.Args = []string{"exe", cmdName}
	ctxt, state, err := hook.NewContextFromEnvironment(r, s.stateDir)
	c.Assert(err, gc.IsNil)
	hcmd, err = hook.Main(r, ctxt, state)
	c.Assert(err, gc.IsNil)
	c.Assert(hcmd, gc.Equals, fakeCommand{})
}

var validHookNameTests = map[string]bool{
	"config-changed":        true,
	"changed-config":        false,
	"install":               true,
	"relation-changed":      false,
	"foo-relation-changed":  true,
	"relation-foo-changed":  false,
	"foo0-relation-changed": true,
	"-relation-changed":     false,
	"foo-xrelation-changed": false,
	"foo-relation-changedx": false,
	"foo-relation-departed": true,
}

func (s *HookSuite) TestValidHookName(c *gc.C) {
	for name, ok := range validHookNameTests {
		c.Check(hook.ValidHookName(name), gc.Equals, ok, gc.Commentf("hook %s", name))
	}
}

type localState struct {
	CallCount int
	Name      string
}

var latestState = make(map[string]localState)

func localStateHookFunc(state *localState, name string) func() error {
	return func() error {
		if state.Name != name && state.Name != "" {
			panic(errgo.Newf("unexpected name in state: %q; expected %q", state.Name, name))
		}
		state.Name = name
		state.CallCount++
		latestState[name] = *state
		return nil
	}
}

func nopContextSetter(ctxt *hook.Context) error {
	return nil
}

func registerLevel1(name string, r *hook.Registry) {
	var state localState
	r.RegisterContext(nopContextSetter, &state)
	r.RegisterHook("config-changed", localStateHookFunc(&state, name))
	r.RegisterHook("peer-relation-changed", localStateHookFunc(&state, name))
}

func registerLevel0(r *hook.Registry) {
	registerLevel1("level1-0", r.Clone("level1-0"))
	registerLevel1("level1-1", r.Clone("level1-1"))

	var state localState
	r.RegisterContext(nopContextSetter, &state)

	r.RegisterHook("config-changed", localStateHookFunc(&state, "level0"))
	r.RegisterHook("other-relation-changed", localStateHookFunc(&state, "level0"))
}

type memState map[string][]byte

func (s memState) Save(name string, data []byte) error {
	s[name] = data
	return nil
}

func (s memState) Load(name string) ([]byte, error) {
	return s[name], nil
}

// runMain runs hook.Main with a context created from the
// environment.
func (s *HookSuite) runMain(c *gc.C, r *hook.Registry) error {
	ctxt, state, err := hook.NewContextFromEnvironment(r, s.stateDir)
	c.Assert(err, gc.IsNil)
	defer ctxt.Close()
	hcmd, err := hook.Main(r, ctxt, state)
	if err != nil {
		return err
	}
	c.Assert(hcmd, gc.IsNil)
	return nil
}

func (s *HookSuite) TestHierarchicalLocalState(c *gc.C) {
	latestState = make(map[string]localState)

	s.StartServer(c, 0, "peer0/0")

	r := hook.NewRegistry()
	registerLevel0(r)
	os.Args = []string{"", "config-changed"}
	for i := 0; i < 3; i++ {
		err := s.runMain(c, r)
		c.Assert(err, gc.IsNil)
	}
	os.Args = []string{"", "peer-relation-changed"}
	for i := 0; i < 2; i++ {
		err := s.runMain(c, r)
		c.Assert(err, gc.IsNil)
	}
	os.Args = []string{"", "other-relation-changed"}
	err := s.runMain(c, r)
	c.Assert(err, gc.IsNil)

	c.Assert(latestState, jc.DeepEquals, map[string]localState{
		"level1-0": {
			CallCount: 5,
			Name:      "level1-0",
		},
		"level1-1": {
			CallCount: 5,
			Name:      "level1-1",
		},
		"level0": {
			CallCount: 4,
			Name:      "level0",
		},
	})
}

func (s *HookSuite) TestHookNotFound(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	r := hook.NewRegistry()
	r.RegisterHook("install", func() error { return nil })
	r.RegisterHook("stop", func() error { return nil })
	os.Args = []string{"exe", "peer-relation-changed"}
	err := s.runMain(c, r)
	c.Assert(err, gc.ErrorMatches, `usage: runhook install
	\| runhook stop`)
}

func (s *HookSuite) BenchmarkHook(c *gc.C) {
	s.StartServer(c, 0, "peer0/0")
	for i := 0; i < 200; i++ {
		s.srvCtxt.rels[0].units[fmt.Sprintf("peer0/%d", i)] = Settings{
			"private-address": fmt.Sprintf("peer0-%d.example.com", i),
			"foo":             "hello there",
		}
	}
	for i := 0; i < c.N; i++ {
		r := hook.NewRegistry()
		r.RegisterHook("install", func() error { return nil })
		registerDefaultRelations(r)
		registerSimpleHook(r, "peer0-relation-joined", func(ctxt *hook.Context) error {
			return nil
		})
		r.RegisterHook("stop", func() error { return nil })
		r.RegisterHook("*", func() error { return nil })
		os.Args = []string{"exe", "peer0-relation-joined"}
		ctxt, state, err := hook.NewContextFromEnvironment(r, s.stateDir)
		c.Assert(err, gc.IsNil)
		hcmd, err := hook.Main(r, ctxt, state)
		c.Assert(hcmd, gc.IsNil)
		c.Assert(err, gc.IsNil)
		ctxt.Close()
	}
}

func registerSimpleHook(r *hook.Registry, hookName string, hookFunc func(ctxt *hook.Context) error) {
	var ctxt *hook.Context
	r.RegisterContext(func(c *hook.Context) error {
		ctxt = c
		return nil
	}, nil)
	r.RegisterHook(hookName, func() error {
		return hookFunc(ctxt)
	})
}

type nopRunner struct{}

func (nopRunner) Run(cmd string, args ...string) (stdout []byte, err error) {
	return nil, nil
}

func (nopRunner) Close() error {
	return nil
}
