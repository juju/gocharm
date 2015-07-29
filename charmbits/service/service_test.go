package service_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
	"github.com/juju/gocharm/hook/hooktest"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (*suite) TestService(c *gc.C) {
	var svc service.Service

	startCount := 0
	configChangedCount := 0
	startService := func(ctxt *service.Context, args []string) (hook.Command, error) {
		startCount++
		c.Check(args, jc.DeepEquals, []string{"arg1", "arg2"})
		return ctxt.ServeLocalRPC(TestRPCServer{})
	}
	r := &hooktest.Runner{
		HookStateDir: c.MkDir(),
		RegisterHooks: func(r *hook.Registry) {
			svc.Register(r.Clone("svc"), "servicename", startService)
			var ctxt *hook.Context
			r.RegisterContext(func(hctxt *hook.Context) error {
				ctxt = hctxt
				return nil
			}, nil)
			r.RegisterHook("start", func() error {
				return svc.Start("arg1", "arg2")
			})
			r.RegisterHook("stop", func() error {
				return svc.Stop()
			})
			r.RegisterHook("config-changed", func() error {
				var resp TestCallResponse
				err := svc.Call("TestRPCServer.TestCall", &TestCallArg{"test"}, &resp)
				c.Check(err, gc.IsNil)
				c.Check(resp.Resp, gc.Equals, "test reply")
				configChangedCount++
				return nil
			})
		},
		Logger: c,
	}
	notify := make(chan hooktest.ServiceEvent, 10)
	service.NewService = hooktest.NewServiceFunc(r, notify)

	err := r.RunHook("install", "", "")
	c.Assert(err, gc.IsNil)

	// Check that the start hook installs and starts the service.
	err = r.RunHook("start", "", "")
	c.Assert(err, gc.IsNil)

	e := expectEvent(c, notify, hooktest.ServiceEventInstall)
	c.Assert(e.Params.Name, gc.Equals, "servicename")

	e = expectEvent(c, notify, hooktest.ServiceEventStart)
	c.Assert(e.Params.Name, gc.Equals, "servicename")

	c.Assert(startCount, gc.Equals, 1)

	// Run the config-changed hook and check that the
	// RPC server works OK.
	err = r.RunHook("config-changed", "", "")
	c.Assert(err, gc.IsNil)

	c.Assert(configChangedCount, gc.Equals, 1)

	// Check that the upgrade-charm hook restarts the service.
	err = r.RunHook("upgrade-charm", "", "")
	c.Assert(err, gc.IsNil)

	e = expectEvent(c, notify, hooktest.ServiceEventStop)
	c.Assert(e.Params.Name, gc.Equals, "servicename")

	e = expectEvent(c, notify, hooktest.ServiceEventStart)
	c.Assert(e.Params.Name, gc.Equals, "servicename")

	c.Assert(startCount, gc.Equals, 2)

	// Run the config-changed hook and check that the
	// RPC server still works OK.
	err = r.RunHook("config-changed", "", "")
	c.Assert(err, gc.IsNil)

	c.Assert(configChangedCount, gc.Equals, 2)

	err = r.RunHook("stop", "", "")
	c.Assert(err, gc.IsNil)

	e = expectEvent(c, notify, hooktest.ServiceEventStop)
	c.Assert(e.Params.Name, gc.Equals, "servicename")
}

type TestRPCServer struct{}

type TestCallArg struct {
	Arg string
}

type TestCallResponse struct {
	Resp string
}

func (TestRPCServer) TestCall(arg *TestCallArg, resp *TestCallResponse) error {
	resp.Resp = arg.Arg + " reply"
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
