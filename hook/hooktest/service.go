package hooktest

import (
	"sync"

	"gopkg.in/errgo.v1"

	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
)

// NewServiceFunc returns a function that can be used to create
// new services and can be assigned to service.NewService
// to mock the OS-level service creator.
// When the service is started, the given Runner
// will be used to run it (via hook.Main).
//
// If the notify channel is not nil, it will be used to send
// events about services created with the function.
// It should be buffered with a size of at least 2.
func NewServiceFunc(r *Runner, notify chan ServiceEvent) func(service.OSServiceParams) service.OSService {
	services := &osServices{
		notify:   notify,
		runner:   r,
		services: make(map[string]*osService),
	}
	return func(p service.OSServiceParams) service.OSService {
		services.mu.Lock()
		defer services.mu.Unlock()
		if svc := services.services[p.Name]; svc != nil {
			return svc
		}
		svc := &osService{
			params:   p,
			services: services,
		}
		services.services[p.Name] = svc
		return svc
	}
}

type osServices struct {
	notify chan ServiceEvent
	runner *Runner

	// mu guards the following fields and services.
	mu       sync.Mutex
	services map[string]*osService
}

// osService provides a mock implementation of the
// service.OSService interface.
type osService struct {
	params service.OSServiceParams

	services *osServices

	// The following fields are guarded by services.mu.

	// installed holds whether the services has been installed.
	installed bool

	// When the service is running, cmd holds
	// the running command.
	cmd hook.Command
}

// ServiceEvent represents an event on a service
// created by the function returned from NewServiceFunc.
type ServiceEvent struct {
	// Kind holds the kind of the service event.
	Kind ServiceEventKind

	// Params holds the parameters used to create the service,
	Params service.OSServiceParams

	// Service holds the service itself.
	Service service.OSService

	// Error holds an error returned from the service's
	// command. It is valid only for ServiceEventError
	// events.
	Error error
}

// ServiceEventKind represents the kind of a ServiceEvent.
type ServiceEventKind int

// NOTE if you change any of the constants below, be
// sure to run go generate to keep the ServiceEventKind.String
// method up to date.

//go:generate stringer -type=ServiceEventKind

const (
	_ ServiceEventKind = iota

	// ServiceEventInstall happens when a service is installed.
	ServiceEventInstall

	// ServiceEventStart happens when a service is started.
	ServiceEventStart

	// ServiceEventError happens when a service's command
	// terminates with an error.
	ServiceEventError

	// ServiceEventStop happens when a service is stopped.
	ServiceEventStop

	// ServiceEventRemove happens when a service is removed.
	// The service will always be stopped first.
	ServiceEventRemove
)

func (svc *osService) notify(kind ServiceEventKind, err error) {
	if svc.services.notify == nil {
		return
	}
	svc.services.notify <- ServiceEvent{
		Kind:    kind,
		Params:  svc.params,
		Service: svc,
		Error:   err,
	}
}

// Install implements service.OSService.Install.
func (svc *osService) Install() error {
	svc.install()
	svc.Start()
	return nil
}

func (svc *osService) install() {
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	if svc.installed {
		return
	}
	svc.installed = true
	svc.notify(ServiceEventInstall, nil)
}

// StopAndRemove implements service.OSService.StopAndRemove.
func (svc *osService) StopAndRemove() error {
	svc.Stop()
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	svc.installed = false
	svc.notify(ServiceEventRemove, nil)
	return nil
}

// Running implements service.OSService.Running.
func (svc *osService) Running() bool {
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	return svc.cmd != nil
}

// Stop implements service.OSService.Stop.
func (svc *osService) Stop() error {
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	if svc.cmd == nil {
		return nil
	}
	svc.cmd.Kill()
	err := svc.cmd.Wait()
	if err != nil {
		svc.notify(ServiceEventError, err)
	}
	svc.notify(ServiceEventStop, nil)
	svc.cmd = nil
	return nil
}

// Start implements service.OSService.Start.
func (svc *osService) Start() error {
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	if !svc.installed {
		return errgo.Newf("service not installed")
	}
	if svc.cmd != nil {
		return nil
	}
	cmd, err := svc.services.runner.RunCommand(svc.params.Args[0], svc.params.Args[1:])
	if err != nil {
		svc.notify(ServiceEventError, errgo.Notef(err, "command start"))
		return nil
	}
	svc.notify(ServiceEventStart, nil)
	if cmd == nil {
		svc.notify(ServiceEventStop, nil)
		return nil
	}
	svc.cmd = cmd
	go func() {
		err := cmd.Wait()
		if err != nil {
			svc.notify(ServiceEventError, errgo.Notef(err, "command wait"))
		}
		svc.services.mu.Lock()
		defer svc.services.mu.Unlock()
		if svc.cmd != cmd {
			// The service has been stopped independently
			// already. We need do nothing more.
			return
		}
		svc.notify(ServiceEventStop, nil)
		svc.cmd = nil
	}()
	return nil
}
