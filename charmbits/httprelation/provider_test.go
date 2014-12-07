package httprelation_test

import (
	"encoding/json"
	"sort"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/gocharm/charmbits/httprelation"
	"github.com/juju/gocharm/hook"
	"github.com/juju/gocharm/hook/hooktest"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&providerSuite{})

type providerSuite struct {
}

func (s *providerSuite) TestRegister(c *gc.C) {
	s.testRegister(c, false)
	s.testRegister(c, true)
}

func (s *providerSuite) testRegister(c *gc.C, withHTTPS bool) {
	r := hook.NewRegistry()
	var p httprelation.Provider
	p.Register(r, "foo", withHTTPS)

	c.Assert(r.RegisteredRelations(), jc.DeepEquals, map[string]charm.Relation{
		"foo": {
			Name:      "foo",
			Role:      charm.RoleProvider,
			Interface: "http",
			Optional:  false,
			Scope:     charm.ScopeGlobal,
		},
	})
	expectedConfig := map[string]charm.Option{
		"http-port": charm.Option{
			Type:        "int",
			Description: "Port for the HTTP server to listen on",
			Default:     80,
		},
	}
	if withHTTPS {
		expectedConfig["https-certificate"] = charm.Option{
			Type:        "string",
			Description: "Certificate and key for https server in PEM format. If this is not set, no https server will be run",
		}
		expectedConfig["https-port"] = charm.Option{
			Type:        "int",
			Description: "Port for the HTTP server to listen on",
			Default:     443,
		}
	}
	c.Assert(r.RegisteredConfig(), jc.DeepEquals, expectedConfig)

	hooks := r.RegisteredHooks()
	sort.Strings(hooks)
	c.Assert(hooks, jc.DeepEquals, []string{"config-changed", "foo-relation-joined"})
}

func (s *providerSuite) TestPortsReturnZeroAtInstallTime(c *gc.C) {
	r := hook.NewRegistry()
	var p httprelation.Provider
	p.Register(r, "foo", true)

	ctxt := &context{
		state: make(hooktest.MemState),
	}
	called := false
	ctxt.runHook(c, "install", "", "", func(p *httprelation.Provider, r *hook.Registry) {
		r.RegisterHook("*", func() error {
			c.Assert(p.HTTPPort(), gc.Equals, 0)
			c.Assert(p.HTTPSPort(), gc.Equals, 0)
			called = true
			return nil
		})
	})
	c.Assert(called, jc.IsTrue)
}

func (s *providerSuite) TestOpensPortWhenConfigChanged(c *gc.C) {
	ctxt := &context{
		state: make(hooktest.MemState),
	}
	ctxt.runHook(c, "install", "", "", nil)
	ctxt.runHook(c, "start", "", "", nil)
	ctxt.config = map[string]interface{}{
		"http-port": 1234,
	}
	rec := ctxt.runHook(c, "config-changed", "", "", func(p *httprelation.Provider, r *hook.Registry) {
		r.RegisterHook("*", func() error {
			c.Check(p.HTTPPort(), gc.Equals, 1234)
			return nil
		})
	})
	c.Assert(rec, jc.DeepEquals, [][]string{
		{"open-port", "1234/tcp"},
	})
}

type context struct {
	withHTTPS   bool
	relations   map[hook.RelationId]map[hook.UnitId]map[string]string
	relationIds map[string][]hook.RelationId
	config      map[string]interface{}
	state       hook.PersistentState
}

func (ctxt *context) runHook(c *gc.C, hookName string, relId hook.RelationId, relUnit hook.UnitId, register func(*httprelation.Provider, *hook.Registry)) [][]string {
	var p httprelation.Provider

	r := hook.NewRegistry()
	p.Register(r, "foo", ctxt.withHTTPS)
	if register != nil {
		register(&p, r)
	}
	hook.RegisterMainHooks(r)
	runner := &hooktest.Runner{
		RunFunc: func(cmd string, args ...string) ([]byte, error) {
			c.Logf("RunFunc %s %q", cmd, args)
			if cmd != "config-get" {
				return nil, nil
			}
			var val interface{}
			if len(args) < 4 {
				val = ctxt.config
			} else {
				key := args[3]
				val = ctxt.config[key]
			}
			data, err := json.Marshal(val)
			c.Assert(err, gc.IsNil)
			c.Logf("returning %s", data)
			return data, nil
		},
		Logger: c,
	}
	hctxt := &hook.Context{
		UUID:        hooktest.UUID,
		Unit:        "wordpress/0",
		CharmDir:    "/nowhere",
		HookName:    hookName,
		Runner:      runner,
		Relations:   ctxt.relations,
		RelationIds: ctxt.relationIds,
	}
	if relId != "" {
		hctxt.RelationId = relId
		hctxt.RemoteUnit = relUnit
	loop:
		for name, ids := range ctxt.relationIds {
			for _, id := range ids {
				if id == hctxt.RelationId {
					hctxt.RelationName = name
					break loop
				}
			}
		}
		c.Assert(hctxt.RelationName, gc.Not(gc.Equals), "")
	}
	err := hook.Main(r, hctxt, ctxt.state)
	c.Assert(err, gc.IsNil)
	return runner.Record
}
