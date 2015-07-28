package httpservice_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/gocharm/charmbits/httpservice"
	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/charmbits/simplerelation"
	"github.com/juju/gocharm/hook"
	"github.com/juju/gocharm/hook/hooktest"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (s *suite) SetUpTest(c *gc.C) {
	httpservice.ClearRegisteredRelations()
}

func nopRegisterField(r *hook.Registry, tag string) (getVal func() (interface{}, error)) {
	panic("should not be called")
}

var registerRelationTests = []struct {
	about         string
	setFieldValue interface{}
	expectPanic   string
	expectT       reflect.Type
	expectD       reflect.Type
}{{
	about:         "correctly specified function",
	setFieldValue: func(*string, int) error { return nil },
	expectT:       reflect.TypeOf(""),
	expectD:       reflect.TypeOf(0),
}, {
	about:         "non-function setFieldValue",
	setFieldValue: 0,
	expectPanic:   `setFieldValue function argument to RegisterRelationType is int not func`,
}, {
	about:         "too few return values",
	setFieldValue: func(*struct{}, int) {},
	expectPanic:   `setFieldValue function argument to RegisterRelationType has wrong return count, got 0, want 1`,
}, {
	about:         "too many return values",
	setFieldValue: func(*struct{}, int) (int, error) { panic("unreachable") },
	expectPanic:   `setFieldValue function argument to RegisterRelationType has wrong return count, got 2, want 1`,
}, {
	about:         "non-error return type",
	setFieldValue: func(*struct{}, int) int { panic("unreachable") },
	expectPanic:   `setFieldValue function argument to RegisterRelationType has wrong return type, got int, want error`,
}, {
	about:         "too few args",
	setFieldValue: func(*struct{}) error { panic("unreachable") },
	expectPanic:   `setFieldValue function argument to RegisterRelationType has wrong argument count; got 1, want 2`,
}, {
	about:         "too many args",
	setFieldValue: func(*struct{}, int, int) error { panic("unreachable") },
	expectPanic:   `setFieldValue function argument to RegisterRelationType has wrong argument count; got 3, want 2`,
}, {
	about:         "first arg not pointer",
	setFieldValue: func(struct{}, int) error { panic("unreachable") },
	expectPanic:   `setFieldValue function argument to RegisterRelationType has wrong first argument type; got struct {} want pointer`,
}}

func (s *suite) TestRegisterRelation(c *gc.C) {
	for i, test := range registerRelationTests {
		c.Logf("test %d: %s", i, test.about)
		httpservice.ClearRegisteredRelations()
		if test.expectPanic != "" {
			c.Assert(func() {
				httpservice.RegisterRelationType(nopRegisterField, test.setFieldValue)
			}, gc.PanicMatches, "cannot register relation: "+test.expectPanic)
			continue
		}
		httpservice.RegisterRelationType(nopRegisterField, test.setFieldValue)
		t, d, ok := httpservice.RegisteredRelationInfo(test.expectT)
		c.Assert(ok, gc.Equals, true)
		c.Assert(t, gc.Equals, test.expectT)
		c.Assert(d, gc.Equals, test.expectD)
	}
}

var registerTests = []struct {
	about            string
	registerField    func(r *hook.Registry, tag string) (getVal func() (interface{}, error))
	setFieldValue    interface{}
	serviceName      string
	httpRelationName string
	handler          interface{}
	expectRelations  map[string]charm.Relation
}{{
	about: "no panic, simple relation",
	registerField: func(r *hook.Registry, tag string) func() (interface{}, error) {
		if tag != "foo-tag" {
			panic("unexpected tag " + tag)
		}
		r.RegisterRelation(charm.Relation{
			Name:      "foo",
			Interface: "foo-interface",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		})
		return func() (interface{}, error) {
			panic("unreachable")
		}
	},
	setFieldValue: func(f *string, v string) error {
		*f = "relation val: " + v
		return nil
	},
	serviceName:      "fooservice",
	httpRelationName: "httpserver",
	handler: func(s string, r *struct {
		Foo string `httpservice:"foo-tag"`
	}) (httpservice.Handler, error) {
		return nil, fmt.Errorf("no handler yet")
	},
	expectRelations: map[string]charm.Relation{
		"foo": {
			Name:      "foo",
			Role:      charm.RoleRequirer,
			Interface: "foo-interface",
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		},
		"httpserver": {
			Name:      "httpserver",
			Role:      charm.RoleProvider,
			Interface: "http",
			Scope:     charm.ScopeGlobal,
		},
	},
},

// TODO test all error cases in newHandlerInfo
}

func (s *suite) TestRegister(c *gc.C) {
	for i, test := range registerTests {
		c.Logf("test %d: %s", i, test.about)
		httpservice.ClearRegisteredRelations()
		httpservice.RegisterRelationType(test.registerField, test.setFieldValue)
		r := hook.NewRegistry()
		var svc httpservice.Service
		svc.Register(r, test.serviceName, test.httpRelationName, test.handler)
		c.Assert(r.RegisteredRelations(), jc.DeepEquals, test.expectRelations)
	}
}

type testType struct {
	relValues map[hook.UnitId]map[string]string
}

type testArg struct {
	Arg string
}

func (s *suite) TestServer(c *gc.C) {
	// First register the relation type. This would usually be
	// done at init time.
	registerField := func(r *hook.Registry, tag string) func() (interface{}, error) {
		c.Logf("registering field with tag %q", tag)
		var rel simplerelation.Requirer
		rel.Register(r.Clone("rel"), "foorelation", "foo-interface")

		return func() (interface{}, error) {
			c.Logf("getting relation data (len vals %d)", len(rel.Values()))
			if vals := rel.Values(); len(vals) > 0 {
				return vals, nil
			}
			c.Logf("returning ErrRelationIncomplete because there are no relations")
			// No relations - no handler.
			return nil, httpservice.ErrRelationIncomplete
		}
	}
	setFieldValue := func(fieldVal *testType, vals map[hook.UnitId]map[string]string) error {
		c.Logf("setFieldVal %v", vals)
		if fieldVal.relValues != nil {
			return httpservice.ErrRestartNeeded
		}
		fieldVal.relValues = vals
		return nil
	}
	httpservice.RegisterRelationType(registerField, setFieldValue)

	closeNotify := make(chan struct{}, 1)
	// Now create a test runner to actually test the logic.
	runner := &hooktest.Runner{
		HookStateDir: c.MkDir(),
		RegisterHooks: func(r *hook.Registry) {
			var svc httpservice.Service
			type relations struct {
				Test testType
			}
			svc.Register(r.Clone("svc"), "httpservicename", "http", func(arg testArg, rel *relations) (httpservice.Handler, error) {
				c.Logf("starting test handler")
				return &testHandler{
					closeNotify: closeNotify,
					arg:         arg.Arg,
					relValues:   rel.Test.relValues,
				}, nil
			})
			r.RegisterHook("start", func() error {
				return svc.Start(testArg{
					Arg: "start arg",
				})
			})
			r.RegisterHook("stop", func() error {
				return svc.Stop()
			})
		},
		Logger: c,
	}

	notify := make(chan hooktest.ServiceEvent, 10)
	service.NewService = hooktest.NewServiceFunc(runner, notify)

	err := runner.RunHook("install", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)

	err = runner.RunHook("start", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)

	httpPort := jujutesting.FindTCPPort()
	runner.Config = map[string]interface{}{
		"http-port": httpPort,
	}
	err = runner.RunHook("config-changed", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.DeepEquals, [][]string{
		{"open-port", fmt.Sprintf("%d/tcp", httpPort)},
	})
	runner.Record = nil

	e := expectEvent(c, notify, hooktest.ServiceEventInstall)
	c.Assert(e.Params.Name, gc.Equals, "httpservicename")
	e = expectEvent(c, notify, hooktest.ServiceEventStart)
	c.Assert(e.Params.Name, gc.Equals, "httpservicename")

	runner.Relations = map[hook.RelationId]map[hook.UnitId]map[string]string{
		"rel0": {
			"fooservice/0": {
				"foo": "bar",
			},
		},
	}
	runner.RelationIds = map[string][]hook.RelationId{
		"foorelation": {"rel0"},
	}
	err = runner.RunHook("foorelation-relation-joined", "rel0", "fooservice/0")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/", httpPort))
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)
	var tresp testResponse
	err = json.Unmarshal(data, &tresp)
	c.Assert(err, gc.IsNil, gc.Commentf("data: %q", data))
	c.Assert(tresp, jc.DeepEquals, testResponse{
		Arg: "start arg",
		RelValues: map[hook.UnitId]map[string]string{
			"fooservice/0": {
				"foo": "bar",
			},
		},
	})

	err = runner.RunHook("stop", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)

	select {
	case <-closeNotify:
	case <-time.After(time.Second):
		c.Fatalf("timed out waiting for close notification")
	}
}

type testHandler struct {
	closeNotify chan struct{}
	arg         string
	relValues   map[hook.UnitId]map[string]string
}

type testResponse struct {
	Arg       string
	RelValues map[hook.UnitId]map[string]string
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := json.NewEncoder(w).Encode(testResponse{
		Arg:       h.arg,
		RelValues: h.relValues,
	}); err != nil {
		panic(err)
	}
}

func (h *testHandler) Close() error {
	h.closeNotify <- struct{}{}
	return nil
}

func expectEvent(c *gc.C, eventc <-chan hooktest.ServiceEvent, kind hooktest.ServiceEventKind) hooktest.ServiceEvent {
	select {
	case e := <-eventc:
		c.Assert(e.Kind, gc.Equals, kind, gc.Commentf("got event %#v", e))
		return e
	case <-time.After(5 * time.Second):
		c.Fatalf("no event received; expected %v", kind)
		panic("unreachable")
	}
}
