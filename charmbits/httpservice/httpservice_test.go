package httpservice_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync/atomic"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
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
	// relValues holds the currently set value of the field.
	relValues map[hook.UnitId]map[string]string

	// isSet holds whether the field has been set.
	isSet bool

	// restarted holds whether setFieldValue has returned ErrRestartNeeded.
	restarted bool

	// finalized holds whether the value has been finalized by passing
	// a zero second argument to setFieldValue.
	finalized bool
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
	// fieldValEvents records the events that have taken place on
	// the field value.
	var fieldValEvents []string
	addFieldValEvent := func(e string) {
		c.Logf("fieldValEvent %s", e)
		fieldValEvents = append(fieldValEvents, e)
	}
	var prevFieldVal *testType
	setFieldValue := func(fieldVal *testType, vals *map[hook.UnitId]map[string]string) error {
		if fieldVal != prevFieldVal {
			// We're using a new field value.
			prevFieldVal = fieldVal
			addFieldValEvent("new")
		}
		switch {
		case vals == nil:
			if fieldVal.finalized {
				return errgo.New("field finalized twice")
			}
			fieldVal.finalized = true
			fieldVal.relValues = nil
			addFieldValEvent("finalize")
			return nil
		case fieldVal.restarted:
			addFieldValEvent("error")
			return errgo.New("setFieldValue returned ErrRestartNeeded but saw a non-zero value")
		case fieldVal.isSet:
			fieldVal.restarted = true
			fieldVal.relValues = nil
			addFieldValEvent("restart")
			return httpservice.ErrRestartNeeded
		default:
			fieldVal.relValues = *vals
			fieldVal.isSet = true
			addFieldValEvent(fmt.Sprintf("set %v", *vals))
			return nil
		}
	}
	httpservice.RegisterRelationType(registerField, setFieldValue)

	var startCount, closeCount int64
	// Now create a test runner to actually test the logic.
	runner := &hooktest.Runner{
		HookStateDir: c.MkDir(),
		RegisterHooks: func(r *hook.Registry) {
			var svc httpservice.Service
			type relations struct {
				Test testType
			}
			svc.Register(r.Clone("svc"), "httpservicename", "http", func(arg testArg, rel *relations) (httpservice.Handler, error) {
				if !rel.Test.isSet {
					c.Errorf("relation not set when server started")
				}
				c.Logf("starting test handler")
				atomic.AddInt64(&startCount, 1)
				return &testHandler{
					closeCount: &closeCount,
					arg:        arg.Arg,
					relValues:  rel.Test.relValues,
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

	// Now go through the usual hook sequence to check
	// that it actually responds correctly.

	err := runner.RunHook("install", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)

	err = runner.RunHook("start", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)

	// Configure the HTTP port, which will allow the
	// OS service to be started but the relation will not
	// be ready yet, so the actual handler will not be
	// created.
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

	// Provide relation values, allowing the relation data to be successfully created.
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
	assertCount(c, &startCount, 1)
	assertCount(c, &closeCount, 0)

	// assertServerValue checks that the server really is started by getting a value
	// from it and checking that it looks correct.
	assertServerValue := func(val map[hook.UnitId]map[string]string) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/", httpPort))
		c.Assert(err, gc.IsNil)
		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		c.Assert(err, gc.IsNil)
		var tresp testResponse
		err = json.Unmarshal(data, &tresp)
		c.Assert(err, gc.IsNil, gc.Commentf("data: %q", data))
		c.Assert(tresp, jc.DeepEquals, testResponse{
			Arg:       "start arg",
			RelValues: val,
		})
	}
	assertServerValue(map[hook.UnitId]map[string]string{
		"fooservice/0": {
			"foo": "bar",
		},
	})

	// Change the relation data, which should cause the server
	// to be restarted with the new relation data.
	runner.Relations = map[hook.RelationId]map[hook.UnitId]map[string]string{
		"rel0": {
			"fooservice/0": {
				"foo": "arble",
			},
		},
	}
	err = runner.RunHook("foorelation-relation-changed", "rel0", "fooservice/0")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)
	assertCount(c, &startCount, 2)
	assertCount(c, &closeCount, 1)
	assertServerValue(map[hook.UnitId]map[string]string{
		"fooservice/0": {
			"foo": "arble",
		},
	})

	// Stop the service, which should close the handler and the field value.
	err = runner.RunHook("stop", "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(runner.Record, gc.HasLen, 0)

	assertCount(c, &startCount, 2)
	assertCount(c, &closeCount, 2)

	c.Assert(fieldValEvents, jc.DeepEquals, []string{
		// config-changed (relation-incomplete)
		"new",
		"finalize",
		// foorelation-relation-joined
		"new",
		"set map[fooservice/0:map[foo:bar]]",
		// foorelation-relation-changed
		"restart",
		"finalize",
		"new",
		"set map[fooservice/0:map[foo:arble]]",
		"finalize",
	})
}

func (s *suite) TestServerStopInStarHook(c *gc.C) {
	var startCount, closeCount int64
	// Now create a test runner to actually test the logic.
	runner := &hooktest.Runner{
		HookStateDir: c.MkDir(),
		RegisterHooks: func(r *hook.Registry) {
			var svc httpservice.Service
			svc.Register(r.Clone("svc"), "httpservicename", "http", func(arg testArg) (httpservice.Handler, error) {
				atomic.AddInt64(&startCount, 1)
				return &testHandler{
					closeCount: &closeCount,
					arg:        arg.Arg,
				}, nil
			})
			r.RegisterHook("start", func() error {
				return svc.Start(testArg{
					Arg: "start arg",
				})
			})
			stopped := false
			r.RegisterHook("stop", func() error {
				// Don't actually stop it here but get the "*" hook to
				// stop it later in this hook execution. This
				// mirrors a bug found in actual code.
				stopped = true
				return nil
			})
			r.RegisterHook("*", func() error {
				if stopped {
					return svc.Stop()
				}
				return nil
			})
		},
		Logger: c,
	}

	service.NewService = hooktest.NewServiceFunc(runner, nil)

	// Now go through the usual hook sequence to check
	// that it actually responds correctly.

	err := runner.RunHook("install", "", "")
	c.Assert(err, gc.IsNil)

	err = runner.RunHook("start", "", "")
	c.Assert(err, gc.IsNil)

	// Configure the HTTP port, which will allow the
	// OS service to be started but the relation will not
	// be ready yet, so the actual handler will not be
	// created.
	httpPort := jujutesting.FindTCPPort()
	runner.Config = map[string]interface{}{
		"http-port": httpPort,
	}
	err = runner.RunHook("config-changed", "", "")
	c.Assert(err, gc.IsNil)

	assertCount(c, &startCount, 1)
	assertCount(c, &closeCount, 0)

	// Stop the service, which should close the handler and the field value.
	err = runner.RunHook("stop", "", "")
	c.Assert(err, gc.IsNil)

	assertCount(c, &startCount, 1)
	assertCount(c, &closeCount, 1)
}

func assertCount(c *gc.C, count *int64, expect int64) {
	c.Assert(atomic.LoadInt64(count), gc.Equals, expect)
}

type testHandler struct {
	closeCount  *int64
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
	atomic.AddInt64(h.closeCount, 1)
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
