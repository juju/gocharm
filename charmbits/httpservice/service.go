// The httpservice package provides a way to run a simple HTTP
// server service as a charm. It does not provide any way to
// interact with the running service - for more control, see
// the httprelation and service packages, which the httpservice
// package uses for its implementation.
package httpservice

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"

	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/charmbits/httprelation"
	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
)

// Service represents an HTTP service. It provides an http
// relation runs a Go HTTP handler as a service.
type Service struct {
	ctxt    *hook.Context
	svc     service.Service
	http    httprelation.Provider
	state   localState
	handler *handler
}

type localState struct {
	Port     int
	Started  bool
	StartArg string
}

// Register registers the service with the given registry.
// If serviceName is non-empty, it specifies the name of the service,
// otherwise the service will be named after the charm's unit.
// The relationName parameter specifies the name of the
// http relation.
//
// The handler value must be a function of the form:
//
//	func(T) (http.Handler, error)
//
// for some type T that can be marshaled as JSON.
// When the service is started, this function will be called
// with the arguments provided to the Start method.
func (svc *Service) Register(r *hook.Registry, serviceName, relationName string, handler interface{}) {
	h, err := newHandler(handler)
	if err != nil {
		panic(errors.Wrapf(err, "cannot register handler function"))
	}
	svc.handler = h
	svc.svc.Register(r.Clone("service"), serviceName, svc.startServer)
	svc.http.Register(r.Clone("http"), relationName)
	r.RegisterContext(svc.setContext, &svc.state)
	r.RegisterHook("*", svc.changed)
}

func (svc *Service) setContext(ctxt *hook.Context) error {
	svc.ctxt = ctxt
	return nil
}

func (svc *Service) changed() error {
	port := svc.http.Port()
	if port == svc.state.Port {
		return nil
	}
	if port == 0 {
		return svc.svc.Stop()
	}
	if err := svc.start(svc.state.StartArg); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

// Start starts the service with the given argument.
// The type of arg must be the same as the type T in
// the handler function provided to Register.
//
// If the argument values change, the service will be
// be restarted, otherwise the service will be left
// unchanged if it is already started.
func (svc *Service) Start(arg interface{}) error {
	argStr, err := svc.handler.marshal(arg)
	if err != nil {
		return errors.Wrap(err)
	}
	if err := svc.start(argStr); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (svc *Service) start(argStr string) error {
	port := svc.http.Port()
	if port == 0 {
		svc.state.StartArg = argStr
		svc.state.Started = true
		return nil
	}
	if err := svc.svc.Start(strconv.Itoa(port), argStr); err != nil {
		return errors.Wrap(err)
	}
	svc.state.StartArg = argStr
	return nil
}

// Stop stops the service.
func (svc *Service) Stop() error {
	if err := svc.svc.Stop(); err != nil {
		return errors.Wrap(err)
	}
	svc.state.Started = false
	return nil
}

// Restart restarts the service.
func (svc *Service) Restart() error {
	return svc.svc.Restart()
}

func (svc *Service) startServer(ctxt *service.Context, args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "got %d arguments, expected 2\n", len(args))
		os.Exit(1)
	}
	port, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid port %q", args[0])
	}
	h, err := svc.handler.get(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get handler: %v", err)
		os.Exit(2)
	}
	addr := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot listen on %s: %v", addr, err)
	}
	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	server.Serve(listener)
}

type handler struct {
	argType reflect.Type
	fv      reflect.Value
}

var (
	httpHandlerType = reflect.TypeOf((*http.Handler)(nil)).Elem()
	errorType       = reflect.TypeOf((*error)(nil)).Elem()
)

func newHandler(f interface{}) (*handler, error) {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		return nil, errors.Newf("bad handler; got %T, expected function", f)
	}
	if n := ft.NumIn(); n != 1 {
		return nil, errors.Newf("bad handler; got %d arguments, expected 1", n)
	}
	if n := ft.NumOut(); n != 2 {
		return nil, errors.Newf("bad handler; got %d return values, expected 2", n)
	}
	if ft.Out(0) != httpHandlerType || ft.Out(1) != errorType {
		return nil, errors.Newf("bad handler; got return values (%s, %s), expected (http.Handler, error)", ft.Out(0), ft.Out(1))
	}
	return &handler{
		argType: ft.In(0),
		fv:      fv,
	}, nil
}

func (h *handler) get(arg string) (http.Handler, error) {
	argv := reflect.New(h.argType)
	err := json.Unmarshal([]byte(arg), argv.Interface())
	if err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal into %s", argv.Type())
	}
	r := h.fv.Call([]reflect.Value{argv.Elem()})
	if err := r[1].Interface(); err != nil {
		return nil, err.(error)
	}
	return r[0].Interface().(http.Handler), nil
}

func (h *handler) marshal(arg interface{}) (string, error) {
	argv := reflect.ValueOf(arg)
	if argv.Type() != h.argType {
		return "", errors.Newf("unexpected argument type; got %s, expected %s", argv.Type(), h.argType)
	}
	data, err := json.Marshal(argv.Interface())
	if err != nil {
		return "", errors.Wrapf(err, "cannot marshal %#v", arg)
	}
	return string(data), nil
}
