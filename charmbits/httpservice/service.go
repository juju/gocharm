// The httpservice package provides a way to run a simple HTTP
// server service as a charm. It does not provide any way to
// interact with the running service - for more control, see
// the httprelation and service packages, which the httpservice
// package uses for its implementation.
package httpservice

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"

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
	HTTPPort  int
	HTTPSPort int
	TLSCert   string
	Started   bool
	StartArg  string
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
	svc.http.Register(r.Clone("http"), relationName, true)
	r.RegisterContext(svc.setContext, &svc.state)
	r.RegisterHook("*", svc.changed)
}

func (svc *Service) setContext(ctxt *hook.Context) error {
	svc.ctxt = ctxt
	return nil
}

func (svc *Service) changed() error {
	httpPort := svc.http.HTTPPort()
	httpsPort := svc.http.HTTPSPort()
	if httpPort == svc.state.HTTPPort && httpsPort == svc.state.HTTPSPort {
		return nil
	}
	if httpPort == 0 && httpsPort == 0 {
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

// PublicHTTPURL returns a URL that can be used to access
// the HTTP service, not including the trailing slash.
// TODO https?
func (svc *Service) PublicHTTPURL() (string, error) {
	addr, err := svc.ctxt.PublicAddress()
	if err != nil {
		return "", errors.Wrapf(err, "cannot get public address")
	}
	port := svc.http.HTTPPort()
	if port == 0 {
		return "", errors.New("port not currently set")
	}
	url := "http://" + addr
	if port != 80 {
		url += ":" + strconv.Itoa(port)
	}
	return url, nil
}

var ErrHTTPSNotConfigured = errors.New("HTTPS not configured")

// PublicHTTPSURL returns an http URL that can be
// used to access the HTTPS service, not including
// the trailing slash. It returns ErrHTTPSNotConfigured
// if there is no current https service.
func (svc *Service) PublicHTTPSURL() (string, error) {
	_, err := svc.http.TLSCertPEM()
	if errors.Cause(err) == httprelation.ErrHTTPSNotConfigured {
		return "", ErrHTTPSNotConfigured
	}
	if err != nil {
		return "", errors.Wrap(err)
	}
	port := svc.http.HTTPSPort()
	if port == 0 {
		return "", ErrHTTPSNotConfigured
	}
	addr, err := svc.ctxt.PublicAddress()
	if err != nil {
		return "", errors.Wrapf(err, "cannot get public address")
	}
	url := "https://" + addr
	if port != 443 {
		url += ":" + strconv.Itoa(port)
	}
	return url, nil
}

func (svc *Service) start(argStr string) error {
	httpPort := svc.http.HTTPPort()
	httpsPort := svc.http.HTTPSPort()
	if httpPort == 0 && httpsPort == 0 {
		svc.state.StartArg = argStr
		svc.state.Started = true
		return nil
	}
	cert, err := svc.http.TLSCertPEM()
	if err != nil && errors.Cause(err) != httprelation.ErrHTTPSNotConfigured {
		return errors.Wrap(err)
	}
	if err := svc.svc.Start(strconv.Itoa(httpPort), strconv.Itoa(httpsPort), cert, argStr); err != nil {
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

type server struct {
	handler *handler
}

func (svc *Service) startServer(ctxt *service.Context, args []string) {
	srv := server{
		handler: svc.handler,
	}
	if err := srv.start(ctxt, args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func (srv *server) start(ctxt *service.Context, args []string) error {
	if len(args) != 4 {
		return errors.Newf("got %d arguments, expected 2", len(args))
	}
	httpPort, err := strconv.Atoi(args[0])
	if err != nil {
		return errors.Newf("invalid port %q", args[0])
	}
	httpsPort, err := strconv.Atoi(args[1])
	if err != nil {
		return errors.Newf("invalid port %q", args[1])
	}
	certPEM := args[2]
	h, err := srv.handler.get(args[3])
	if err != nil {
		return errors.Newf("cannot get handler: %v", err)
	}
	done := make(chan error, 2)
	if httpPort != 0 {
		go func() {
			done <- srv.serveHTTP(httpPort, h)
		}()
	}
	if httpsPort != 0 && certPEM != "" {
		go func() {
			done <- srv.serveHTTPS(httpsPort, certPEM, h)
		}()
	}
	return errors.Wrap(<-done)
}

func (*server) serveHTTP(port int, h http.Handler) error {
	addr := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.Newf("cannot listen on %s: %v", addr, err)
	}
	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	return server.Serve(listener)
}

func (*server) serveHTTPS(port int, certPEM string, h http.Handler) error {
	certPEMBytes := []byte(certPEM)
	cert, err := tls.X509KeyPair(certPEMBytes, certPEMBytes)
	if err != nil {
		return errors.Newf("cannot parse certificate: %v", err)
	}
	config := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
	}
	addr := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.Newf("cannot listen on %s: %v", addr, err)
	}
	tlsListener := tls.NewListener(
		tcpKeepAliveListener{listener.(*net.TCPListener)},
		config,
	)
	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	return server.Serve(tlsListener)
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

// tcpKeepAliveListener is stolen from net/http

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
