// The httpservice package provides a way to run a simple HTTP
// server service as a charm. It does not provide any way to
// interact with the running service - for more control, see
// the httprelation and service packages, which the httpservice
// package uses for its implementation.
package httpservice

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"gopkg.in/errgo.v1"

	"github.com/juju/gocharm/charmbits/httprelation"
	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
)

// Service represents an HTTP service. It provides an http
// relation runs a Go HTTP handler as a service.
type Service struct {
	ctxt           *hook.Context
	svc            service.Service
	http           httprelation.Provider
	state          localState
	handlerInfo    *handlerInfo
	relationValues map[string][]byte
}

type localState struct {
	ArgData []byte
	Started bool
}

var (
	registeredRelations      = make(map[reflect.Type]*registeredRelation)
	registeredRelationsMutex sync.Mutex
)

type registeredRelation struct {
	// t holds the type of the field that the server will see when
	// running. The first argument of setFieldValue is a pointer
	// to this type.
	t reflect.Type

	// d holds the type of the relation data that's transferred across
	// the RPC channel to the server. The second argument of setFieldValue
	// has this type.
	d reflect.Type

	// registerField is documented in RegisterRelationType.
	registerField func(r *hook.Registry, tag string) (getVal func() (interface{}, error))

	// setFieldValue is a function value as documented in RegisterRelationType.
	setFieldValue reflect.Value
}

var (
	ErrRestartNeeded      = errgo.New("handler needs to be restarted")
	ErrRelationIncomplete = errgo.New("relation data is incomplete")
)

// RegisterRelationType registers a relation type that can be passed to a
// handler. It may be called by a relation package to register a type
// that may be used in field of the R argument to handler function
// registered with Register.
//
// This function is usually called implicitly as a result of importing
// a relation package.
//
// The register function argument will be called when a charm registers
// a handler function that has an second argument with a struct type
// that has a field of type T. Its arguments are the current hook
// registry, the httpservice tag associated with the field (if any).
//
// The registerField function should return a function that will be called
// after any hook functions registered and which should return any data
// associated with the relation in a JSON-marshalable value. This will
// be unmarshaled and passed as the second setFieldValue argument
// when the server is started.
//
// If the data associated with the relation is incomplete, registerField
// may return an error with an ErrRelationIncomplete cause,
// in which case the handler will be stopped until the relation is complete.
// TODO provide a mechanism for making relations optional.
//
// The setFieldValue argument should be a function of the form
//
//	func(fv *T, val D) error
//
// where T is the struct-field type to be registered and D is the JSON-marshalable
// type returned from getVal (the data that will be sent to the
// server). The setFieldValue function will be called within the service whenever the
// value returned by getVal changes. The value of fv will point to the
// field to fill in from the relation data. Note that each time the
// relation changes while the service is still running, the fv value
// will be the same. If the change in the value requires the service to
// be restarted, setFieldValue should return an error with an
// ErrRestartNeeded cause. If any other error is returned, it will
// be logged as a warning and made available in the charm status.
// Even if setFieldValue returns an error, it should still set the value
// appropriately (for example by zeroing out the field).
func RegisterRelationType(
	registerField func(r *hook.Registry, tag string) (getVal func() (interface{}, error)),
	setFieldValue interface{},
) {
	registeredRelationsMutex.Lock()
	defer registeredRelationsMutex.Unlock()
	reg, err := newRegisteredRelation(registerField, reflect.ValueOf(setFieldValue))
	if err != nil {
		panic(errgo.Notef(err, "cannot register relation"))
	}
	if _, ok := registeredRelations[reg.t]; ok {
		panic("type " + reg.t.String() + " registered twice")
	}
	registeredRelations[reg.t] = reg
}

func newRegisteredRelation(
	registerField func(r *hook.Registry, tag string) (getVal func() (interface{}, error)),
	setf reflect.Value,
) (*registeredRelation, error) {
	sett := setf.Type()
	if sett.Kind() != reflect.Func {
		return nil, errgo.Newf("setFieldValue function argument to RegisterRelationType is %v not func", sett)
	}
	if sett.NumOut() != 1 {
		return nil, errgo.Newf("setFieldValue function argument to RegisterRelationType has wrong return count, got %d, want 1", sett.NumOut())
	}
	if sett.Out(0) != errorType {
		return nil, errgo.Newf("setFieldValue function argument to RegisterRelationType has wrong return type, got %s, want error", sett.Out(0))
	}
	if sett.NumIn() != 2 {
		return nil, errgo.Newf("setFieldValue function argument to RegisterRelationType has wrong argument count; got %d, want 2", sett.NumIn())
	}
	T := sett.In(0)
	if T.Kind() != reflect.Ptr {
		return nil, errgo.Newf("setFieldValue function argument to RegisterRelationType has wrong first argument type; got %s want pointer", T)
	}
	return &registeredRelation{
		t:             T.Elem(),
		d:             sett.In(1),
		registerField: registerField,
		setFieldValue: setf,
	}, nil
}

// Handler represents a handler that can also be
// closed to free any of its associated resources.
type Handler interface {
	http.Handler
	io.Closer
}

// Register registers the service with the given registry.
// If serviceName is non-empty, it specifies the name of the service,
// otherwise the service will be named after the charm's unit.
// The httpRelationName parameter specifies the name of the
// http relation.
//
// The handler value must be a function in one of
// the following forms:
//
//	func(T) (Handler, error)
//	func(T, *R) (Handler, error)
//
// for some type T that can be marshaled as JSON
// and some struct type R that contains exported fields
// with types registered with RegisterRelationType.
// Currently, anonymous fields are not supported.
//
// When the service is started, this function will be called
// with the argument provided to the Start method,
// and any R argument filled in with the values for any
// current relations.
//
// Note that the handler function will not be called with
// any hook context available, as it is run by the OS-provided
// service runner (e.g. upstart).
//
// When a new handler is required, the old one will be closed
// before the new one is started, but outstanding
// HTTP requests will not be waited for (this may change).
func (svc *Service) Register(r *hook.Registry, serviceName, httpRelationName string, handler interface{}) {
	h, err := svc.newHandlerInfo(handler, r)
	if err != nil {
		panic(errgo.Notef(err, "cannot register handler function"))
	}
	svc.handlerInfo = h
	svc.svc.Register(r.Clone("service"), serviceName, func(ctxt *service.Context, args []string) (hook.Command, error) {
		return startServer(ctxt, args, svc.handlerInfo)
	})
	svc.http.Register(r.Clone("http"), httpRelationName, true)
	r.RegisterContext(svc.setContext, &svc.state)
	r.RegisterHook("*", svc.changed)
}

func (svc *Service) setContext(ctxt *hook.Context) error {
	svc.ctxt = ctxt
	return nil
}

// HTTPPort returns the currently configured HTTP port
// for the service.
func (svc *Service) HTTPPort() int {
	return svc.http.HTTPPort()
}

// HTTPSPort returns the currently configured HTTPS port
// for the service.
func (svc *Service) HTTPSPort() int {
	return svc.http.HTTPSPort()
}

// Start starts the service with the given argument.
// The type of arg must be the same as the type T in
// the handler function provided to Register.
//
// If the value changes, a new handler will be started
// with the given argument value.
func (svc *Service) Start(arg interface{}) error {
	argData, err := svc.handlerInfo.marshal(arg)
	if err != nil {
		return errgo.Mask(err)
	}
	svc.state.ArgData = argData
	svc.state.Started = true
	return svc.changed()
}

// changed is called after any hook has been invoked.
// It gets the current value of all settings and starts,
// stops or notifies the server appropriately.
func (svc *Service) changed() error {
	svc.ctxt.Logf("httpservice: changed, hook %s", svc.ctxt.HookName)
	httpPort := svc.http.HTTPPort()
	httpsPort := svc.http.HTTPSPort()
	cert, err := svc.http.TLSCertPEM()
	if err != nil && errgo.Cause(err) != httprelation.ErrHTTPSNotConfigured {
		svc.ctxt.Logf("bad TLS cert")
		// TODO set charm status instead?
		return errgo.Mask(err)
	}
	if !svc.state.Started || httpPort == 0 && (httpsPort == 0 || cert == "") {
		svc.ctxt.Logf("httpservice: stopping service")
		return svc.svc.Stop()
	}
	if !svc.svc.Started() {
		svc.ctxt.Logf("httpservice: starting service")
		if err := svc.svc.Start(svc.ctxt.StateDir()); err != nil {
			return errgo.Notef(err, "cannot start service")
		}
	} else {
		svc.ctxt.Logf("httpservice: no need to start service")
	}
	state := &ServerState{
		HTTPPort:       httpPort,
		HTTPSPort:      httpsPort,
		CertPEM:        cert,
		ArgData:        svc.state.ArgData,
		RelationValues: svc.relationValues,
	}
	svc.ctxt.Logf("calling Srv.Set %#v", state)
	var resp Feedback
	if err := svc.svc.Call("Srv.Set", state, &resp); err != nil {
		return errgo.Notef(err, "cannot set state in server")
	}
	if len(resp.Warnings) == 0 {
		return nil
	}
	for _, w := range resp.Warnings {
		svc.ctxt.Logf("warning: %s", w)
	}
	// TODO set status to reflect warnings?
	return nil
}

// PublicHTTPURL returns a URL that can be used to access
// the HTTP service, not including the trailing slash.
// TODO https?
func (svc *Service) PublicHTTPURL() (string, error) {
	addr, err := svc.ctxt.PublicAddress()
	if err != nil {
		return "", errgo.Notef(err, "cannot get public address")
	}
	port := svc.http.HTTPPort()
	if port == 0 {
		return "", errgo.New("port not currently set")
	}
	url := "http://" + addr
	if port != 80 {
		url += ":" + strconv.Itoa(port)
	}
	return url, nil
}

var ErrHTTPSNotConfigured = errgo.New("HTTPS not configured")

// PublicHTTPSURL returns an http URL that can be
// used to access the HTTPS service, not including
// the trailing slash. It returns ErrHTTPSNotConfigured
// if there is no current https service.
func (svc *Service) PublicHTTPSURL() (string, error) {
	_, err := svc.http.TLSCertPEM()
	if errgo.Cause(err) == httprelation.ErrHTTPSNotConfigured {
		return "", ErrHTTPSNotConfigured
	}
	if err != nil {
		return "", errgo.Mask(err)
	}
	port := svc.http.HTTPSPort()
	if port == 0 {
		return "", ErrHTTPSNotConfigured
	}
	addr, err := svc.ctxt.PublicAddress()
	if err != nil {
		return "", errgo.Notef(err, "cannot get public address")
	}
	url := "https://" + addr
	if port != 443 {
		url += ":" + strconv.Itoa(port)
	}
	return url, nil
}

// Stop stops the service.
func (svc *Service) Stop() error {
	svc.state.ArgData = nil
	svc.state.Started = false
	return nil
}

// Restart restarts the service.
func (svc *Service) Restart() error {
	return svc.svc.Restart()
}

type handlerInfo struct {
	argType      reflect.Type
	relationType reflect.Type
	fv           reflect.Value
}

var (
	handlerType = reflect.TypeOf((*Handler)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// newHandler makes a new handler value from the given function,
// which should be in one of the forms expected by Service.Register.
// It also registers any relations required by the handler.
func (svc *Service) newHandlerInfo(f interface{}, registry *hook.Registry) (*handlerInfo, error) {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		return nil, errgo.Newf("bad handler: got %T, expected function", f)
	}
	if n := ft.NumIn(); n != 1 && n != 2 {
		return nil, errgo.Newf("bad handler:ag got %d arguments, expected 1 or 2", n)
	}
	if n := ft.NumOut(); n != 2 {
		return nil, errgo.Newf("bad handler: got %d return values, expected 2", n)
	}
	if ft.Out(0) != handlerType || ft.Out(1) != errorType {
		return nil, errgo.Newf("bad handler: got return values (%s, %s), expected (httpservice.Handler, error)", ft.Out(0), ft.Out(1))
	}
	h := &handlerInfo{
		argType: ft.In(0),
		fv:      fv,
	}
	if ft.NumIn() == 1 {
		// No relations, nothing more to do.
		return h, nil
	}
	svc.relationValues = make(map[string][]byte)
	rt := ft.In(1)
	if rt.Kind() != reflect.Ptr || rt.Elem().Kind() != reflect.Struct {
		return nil, errgo.Newf("bad handler: second argument is %v not a pointer to struct", h.relationType)
	}
	h.relationType = rt.Elem()
	getters := make(map[string]func() (interface{}, error))
	for i := 0; i < h.relationType.NumField(); i++ {
		// TODO anonymous fields?
		f := h.relationType.Field(i)
		if f.PkgPath != "" {
			continue
		}
		reg := registeredRelations[f.Type]
		if reg == nil {
			return nil, errgo.Newf("bad handler: field %s of type %s is not a registered relation type: missing import?", f.Name, f.Type)
		}
		getters[f.Name] = reg.registerField(registry.Clone("rel-"+f.Name), f.Tag.Get("httpservice"))
	}
	registry.RegisterHook("*", func() error {
		for name, getter := range getters {
			val, err := getter()
			if err != nil {
				if errgo.Cause(err) == ErrRelationIncomplete {
					delete(svc.relationValues, name)
					continue
				}
				return errgo.Notef(err, "cannot get value for field %s", name)
			}
			data, err := json.Marshal(val)
			if err != nil {
				return errgo.Notef(err, "cannot marshal value for field %s", name)
			}
			if len(data) == 0 {
				return errgo.Notef(err, "field %s marshaled to no data", name)
			}
			svc.relationValues[name] = data
		}
		svc.ctxt.Logf("after running getters, data: %#v", svc.relationValues)
		return nil
	})
	return h, nil
}

// handler returns the actual HTTP handler to serve by
// calling the registered handler function.
// The given argument holds JSON-marshaled data
// that will be unmarshaled into the first argument
// of the getter function. The other must be addressable
// and have the same type as h.relationType.
//
// This function, unlike the other methods on handlerInfo
// called in server context, not hook context.
func (h *handlerInfo) handler(arg []byte, val reflect.Value) (Handler, error) {
	argv := reflect.New(h.argType)
	err := json.Unmarshal(arg, argv.Interface())
	if err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal into %s", argv.Type())
	}
	r := h.fv.Call([]reflect.Value{argv.Elem(), val.Addr()})
	if err := r[1].Interface(); err != nil {
		return nil, err.(error)
	}
	return r[0].Interface().(Handler), nil
}

// marshal marshals the client-specific data in arg.
// It fails if the data is not the same type expected
// by the registered handler function.
func (h *handlerInfo) marshal(arg interface{}) ([]byte, error) {
	argv := reflect.ValueOf(arg)
	if argv.Type() != h.argType {
		return nil, errgo.Newf("unexpected argument type; got %s, expected %s", argv.Type(), h.argType)
	}
	data, err := json.Marshal(argv.Interface())
	if err != nil {
		return nil, errgo.Notef(err, "cannot marshal %#v", arg)
	}
	return data, nil
}
