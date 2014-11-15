// The service packge provides a way for a charm to start and stop
// a service that runs independently of the charm hooks.
package service

import (
	"fmt"
	"net/rpc"
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
}

// Register registers the service with the given registry. If
// serviceName is non-empty, it specifies the name of the service,
// otherwise the service will be named after the charm's unit.
//
// The serviceRPC parameter defines any methods that can be invoked
// locally on the service, defined as for the rpc package (see
// rpc.Server.Register). When the service has been started, these
// methods may be invoked using the Service.Call method. Parameters and
// return values will be marshaled as JSON.
func (svc *Service) Register(r *hook.Registry, serviceName string, serviceRPC interface{}) {
	svc.serviceName = serviceName
	r.RegisterContext(svc.setContext, &svc.state)
	// TODO restart the service when the charm is upgraded.
	// We could also perhaps provide some way to do zero-downtime
	// upgrades.
	//r.RegisterHook("upgrade-charm", svc.upgradeCharm)
	r.RegisterCommand(func(args []string) {
		runServer(serviceRPC, args)
	})
}

func (svc *Service) setContext(ctxt *hook.Context) error {
	svc.ctxt = ctxt
	return nil
}

// Start starts the service if it is not already started.
func (svc *Service) Start() error {
	svc.ctxt.Logf("starting service")
	usvc := svc.upstartService()
	if !svc.state.Installed {
		svc.ctxt.Logf("installing service")
		if err := usvc.Install(); err != nil {
			return errors.Wrap(err)
		}
		svc.state.Installed = true
		return nil
	} else {
		svc.ctxt.Logf("service already installed")
	}
	if err := usvc.Start(); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

// Stop stops the service running.
func (svc *Service) Stop() error {
	if err := svc.upstartService().Stop(); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

// Started reports whether the service has been started.
func (svc *Service) Started() bool {
	return svc.upstartService().Running()
}

// StopAndRemove stops and removes the service completely.
func (svc *Service) StopAndRemove() error {
	if !svc.state.Installed {
		return nil
	}
	if err := svc.upstartService().StopAndRemove(); err != nil {
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
			c, err := rpc.Dial("unix", svc.socketPath())
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

func (svc *Service) socketPath() string {
	return "@" + filepath.Join(svc.ctxt.StateDir(), "service")
}

func (svc *Service) upstartService() *upstart.Service {
	exe := filepath.Join(svc.ctxt.CharmDir, "bin", "runhook")
	serviceName := svc.serviceName
	if serviceName == "" {
		serviceName = svc.ctxt.Unit.Tag().String()
	}
	return &upstart.Service{
		Name: serviceName,
		Conf: serviceCommon.Conf{
			InitDir: "/etc/init",
			Desc:    fmt.Sprintf("service for juju unit %q", svc.ctxt.Unit),
			Cmd: fmt.Sprintf("%s %s %q",
				exe,
				svc.ctxt.CommandName(),
				svc.socketPath(),
			),
			// TODO save output somewhere - we need a better answer for that.
		},
	}
}
