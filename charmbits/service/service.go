// The service packge provides a way for a charm to start and stop
// a service that runs independently of the charm hooks.
package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path/filepath"
	"time"

	serviceCommon "github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/utils"
	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/hook"
)

// Service represents a long running service that runs
// outside of the usual charm hook context.
type Service struct {
	ctxt        *hook.Context
	serviceName string
	state       localState
	rpcClient   *rpc.Client
}

type localState struct {
	Installed bool
	Args      []string
}

// Register registers the service with the given registry. If
// serviceName is non-empty, it specifies the name of the service,
// otherwise the service will be named after the charm's unit.
//
// When the service is started, the start function will be called
// with the context for the running service and any arguments
// that were passed to the Service.Start method.
// When the start function returns, the service will exit.
func (svc *Service) Register(r *hook.Registry, serviceName string, start func(ctxt *Context, args []string)) {
	svc.serviceName = serviceName
	r.RegisterContext(svc.setContext, &svc.state)
	// TODO Perhaps provide some way to do zero-downtime
	// upgrades?
	r.RegisterHook("upgrade-charm", svc.upgradeCharm)
	r.RegisterCommand(func(args []string) {
		runServer(start, args)
	})
}

func (svc *Service) setContext(ctxt *hook.Context) error {
	svc.ctxt = ctxt
	return nil
}

func (svc *Service) upgradeCharm() error {
	if err := svc.Stop(); err != nil {
		return errors.Wrapf(err, "cannot stop service")
	}
	if err := svc.Start(svc.state.Args...); err != nil {
		return errors.Wrapf(err, "cannot restart service")
	}
	return nil
}

// Start starts the service if it is not already started,
// passing it the given arguments.
// If the arguments are different from the last
// time it was started, it will be stopped and then
// started again with the new arguments.
func (svc *Service) Start(args ...string) error {
	// Create the state directory in preparation for the log output.
	if err := os.MkdirAll(svc.ctxt.StateDir(), 0700); err != nil {
		return errors.Wrapf(err, "cannot create state directory")
	}
	svc.ctxt.Logf("starting service")
	usvc := svc.upstartService(args)
	// Note: Install will restart the service if the configuration
	// file has changed.
	if err := usvc.Install(); err != nil {
		return errors.Wrapf(err, "cannot install service")
	}
	// If the service was already installed but not started,
	// Install will not do anything, so ensure that the service
	// is actually started.
	if err := usvc.Start(); err != nil {
		return errors.Wrapf(err, "cannot start service")
	}
	svc.state.Installed = true
	svc.state.Args = args
	return nil
}

// Stop stops the service running.
func (svc *Service) Stop() error {
	if err := svc.upstartService(nil).Stop(); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

// Started reports whether the service has been started.
func (svc *Service) Started() bool {
	return svc.upstartService(nil).Running()
}

// StopAndRemove stops and removes the service completely.
func (svc *Service) StopAndRemove() error {
	if !svc.state.Installed {
		return nil
	}
	if err := svc.upstartService(nil).StopAndRemove(); err != nil {
		return errors.Wrap(err)
	}
	svc.state.Installed = false
	return nil
}

var shortAttempt = utils.AttemptStrategy{
	Total: 250 * time.Millisecond,
	Delay: 5 * time.Millisecond,
}

// Call invokes a method on the service. See rpc.Client.Call for
// the full semantics.
func (svc *Service) Call(method string, args interface{}, reply interface{}) error {
	if svc.rpcClient == nil {
		if !svc.state.Installed {
			return errors.New("service is not started")
		}
		// The service may be notionally started not be actually
		// running yet, so try for a short while if it fails.
		for a := shortAttempt.Start(); a.Next(); {
			c, err := dialRPC(svc.socketPath())
			if err == nil {
				svc.rpcClient = c
				break
			}
			if !a.HasNext() {
				return errors.Wrap(err)
			}
		}
	}
	return svc.rpcClient.Call(method, args, reply)
}

func dialRPC(path string) (*rpc.Client, error) {
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return rpc.NewClientWithCodec(jsonrpc.NewClientCodec(c)), nil
}

func (svc *Service) socketPath() string {
	return "@" + filepath.Join(svc.ctxt.StateDir(), "service")
}

func (svc *Service) upstartService(args []string) *upstart.Service {
	exe := filepath.Join(svc.ctxt.CharmDir, "bin", "runhook")
	serviceName := svc.serviceName
	if serviceName == "" {
		serviceName = svc.ctxt.Unit.Tag().String()
	}
	// Marshal all arguments as JSON to avoid upstart quoting hassles.
	p := serviceParams{
		SocketPath: svc.socketPath(),
		Args:       args,
	}
	pdata, err := json.Marshal(p)
	if err != nil {
		panic(errors.Wrapf(err, "cannot marshal parameters"))
	}
	cmd := exe + " " +
		svc.ctxt.CommandName() + " " +
		base64.StdEncoding.EncodeToString(pdata)
	return &upstart.Service{
		Name: serviceName,
		Conf: serviceCommon.Conf{
			InitDir: "/etc/init",
			Desc:    fmt.Sprintf("service for juju unit %q", svc.ctxt.Unit),
			Cmd:     cmd,
			Out:     filepath.Join(svc.ctxt.StateDir(), "servicelog.out"),
		},
	}
}
