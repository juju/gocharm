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
		notify:    notify,
		runner:    r,
		installed: make(map[string]*installedOSService),
	}
	return func(p service.OSServiceParams) service.OSService {
		return &osService{
			params:   p,
			services: services,
		}
	}
}

type osServices struct {
	notify chan ServiceEvent
	runner *Runner

	// mu guards the following fields and services.
	mu        sync.Mutex
	installed map[string]*installedOSService
}

// osService provides a mock implementation of the
// service.OSService interface.
type osService struct {
	params service.OSServiceParams

	services *osServices
}

type installedOSService struct {
	services *osServices

	// params holds the parameters that the service
	// was installed with.
	params service.OSServiceParams

	// The following fields are guarded by services.mu.

	// When the service is running, cmd holds
	// the running command.
	cmd hook.Command
}

func (isvc *installedOSService) notify(kind ServiceEventKind, err error) {
	if isvc.services.notify == nil {
		return
	}
	if err != nil {
		isvc.logf("hooktest: service event %v (err %v)", kind, err)
	} else {
		isvc.logf("hooktest: service event %v", kind)
	}
	isvc.services.notify <- ServiceEvent{
		Kind:   kind,
		Params: isvc.params,
		Error:  err,
	}
}

func (isvc *installedOSService) logf(f string, a ...interface{}) {
	isvc.services.runner.Logger.Logf(f, a...)
}

// ServiceEvent represents an event on a service
// created by the function returned from NewServiceFunc.
type ServiceEvent struct {
	// Kind holds the kind of the service event.
	Kind ServiceEventKind

	// Params holds the parameters used to create the service,
	Params service.OSServiceParams

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

// installedService returns the installed service corresponding
// to svc, or nil if it is not installed.
// Must be called with svc.services.mu held.
func (svc *osService) installedService() *installedOSService {
	return svc.services.installed[svc.params.Name]
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
	if isvc := svc.services.installed[svc.params.Name]; isvc != nil {
		return
	}
	isvc := &installedOSService{
		services: svc.services,
		params:   svc.params,
	}
	isvc.logf("hooktest: install service %s; exe: %s; args: %q", svc.params.Name, svc.params.Exe, svc.params.Args)
	isvc.services.installed[svc.params.Name] = isvc
	isvc.notify(ServiceEventInstall, nil)
}

// StopAndRemove implements service.OSService.StopAndRemove.
func (svc *osService) StopAndRemove() error {
	svc.Stop()
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	if isvc := svc.installedService(); isvc != nil {
		delete(svc.services.installed, svc.params.Name)
		isvc.notify(ServiceEventRemove, nil)
	}
	return nil
}

// Running implements service.OSService.Running.
func (svc *osService) Running() bool {
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	isvc := svc.installedService()
	return isvc != nil && isvc.cmd != nil
}

// Stop implements service.OSService.Stop.
func (svc *osService) Stop() error {
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	isvc := svc.installedService()
	if isvc == nil || isvc.cmd == nil {
		return nil
	}
	isvc.cmd.Kill()
	err := isvc.cmd.Wait()
	if err != nil {
		isvc.notify(ServiceEventError, err)
	}
	isvc.notify(ServiceEventStop, nil)
	isvc.cmd = nil
	return nil
}

// Start implements service.OSService.Start.
func (svc *osService) Start() error {
	svc.services.mu.Lock()
	defer svc.services.mu.Unlock()
	isvc := svc.installedService()
	if isvc == nil {
		return errgo.Newf("service not installed")
	}
	if isvc.cmd != nil {
		return nil
	}
	cmd, err := isvc.services.runner.RunCommand(svc.params.Args[0], svc.params.Args[1:])
	if err != nil {
		isvc.notify(ServiceEventError, errgo.Notef(err, "command start"))
		return nil
	}
	isvc.notify(ServiceEventStart, nil)
	if cmd == nil {
		isvc.notify(ServiceEventStop, nil)
		return nil
	}
	isvc.cmd = cmd
	go func() {
		err := cmd.Wait()
		if err != nil {
			isvc.notify(ServiceEventError, errgo.Notef(err, "command wait"))
		}
		isvc.services.mu.Lock()
		defer isvc.services.mu.Unlock()
		if isvc.cmd != cmd {
			// The service has been stopped independently
			// already. We need do nothing more.
			return
		}
		isvc.notify(ServiceEventStop, nil)
		isvc.cmd = nil
	}()
	return nil
}
