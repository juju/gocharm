package httpservice

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
)

// server represents a running HTTP server.
type server struct {
	tomb tomb.Tomb

	handlerInfo *handlerInfo
	handler     Handler
	stateDir    string

	mu    sync.Mutex
	state ServerState
	// relationState holds the struct value with fields
	// set by the setFieldValue argument to RegisterRelation
	// calls.
	relationState reflect.Value
	httpListener  *handlerListener
	httpsListener *handlerListener
}

// ServerState holds the state of a server - all the
// parameters that might affect it.
//
// It is an implementation detail and only exposed
// because net/rpc requires it.
type ServerState struct {
	HTTPPort  int
	HTTPSPort int
	CertPEM   string

	// Arg holds the value passed to Service.Start, marshaled as JSON.
	ArgData []byte

	// RelationValues holds an entry for each relation
	// defined in the second argument of the handler function
	// passed to Register.
	RelationValues map[string][]byte
}

// startServer starts the actual HTTP server running. It is called
// in service context, not hook context.
func startServer(ctxt *service.Context, args []string, h *handlerInfo) (hook.Command, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("need exactly one argument, got %q", args)
	}
	srv := &server{
		handlerInfo: h,
		stateDir:    args[0],
	}
	state, err := srv.loadState()
	if err != nil {
		return nil, fmt.Errorf("cannot load state: %v", err)
	}
	var fb Feedback
	srv.set(state, &fb)
	fb.show()
	// Fool net/rpc into thinking the type is exported, because
	// we don't want to export the srv type from this package.
	type Srv struct {
		*server
	}
	rpcCmd, err := ctxt.ServeLocalRPC(Srv{srv})
	if err != nil {
		return nil, errgo.Notef(err, "serve local RPC failed")
	}
	srv.tomb.Go(func() error {
		srv.tomb.Kill(rpcCmd.Wait())
		return nil
	})
	srv.tomb.Go(func() error {
		<-srv.tomb.Dying()
		// The command has been killed. Stop the RPC listener
		// and close any current listeners to cause the handlers
		// to terminate.
		rpcCmd.Kill()
		err := rpcCmd.Wait()
		if err != nil {
			err = errgo.Notef(err, "local RPC server")
		}

		srv.mu.Lock()
		defer srv.mu.Unlock()
		srv.closeResources()
		return err
	})
	return srv, nil
}

// Kill implements gocharm.Command.Kill.
func (srv *server) Kill() {
	srv.tomb.Kill(nil)
}

// Wait implements gocharm.Command.Wait.
func (srv *server) Wait() error {
	return srv.tomb.Wait()
}

// Feedback is an implementation detail, exposed only because net/rpc requires it.
type Feedback struct {
	Warnings []string
}

// show shows all the warnings.
func (fb *Feedback) show() {
	for _, w := range fb.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %v\n", w)
	}
}

// addError adds the given error to the list of warnings
// that will be reported as the result of an RPC call.
func (fb *Feedback) addError(err error) {
	fb.Warnings = append(fb.Warnings, err.Error())
}

// Set implements an RPC server method that is called
// to set the current server state. Instead of returning
// an error, any errors are recorded in the given response
// value
func (srv *server) Set(p *ServerState, response *Feedback) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.set(*p, response)
	response.show()
	srv.state = *p
	// Save the server state so that if it's started or the
	// instance is rebooted, we can carry on as before.
	if err := srv.saveState(srv.state); err != nil {
		return errgo.Notef(err, "cannot save server state")
	}
	return nil
}

func (srv *server) serveHTTP(port int, h http.Handler) (*handlerListener, error) {
	addr := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errgo.Newf("cannot listen on %s: %v", addr, err)
	}
	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	return newHandlerListener(server, listener), nil
}

func (srv *server) serveHTTPS(port int, certPEM string, h http.Handler) (*handlerListener, error) {
	certPEMBytes := []byte(certPEM)
	cert, err := tls.X509KeyPair(certPEMBytes, certPEMBytes)
	if err != nil {
		return nil, errgo.Newf("cannot parse certificate: %v", err)
	}
	config := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
	}
	addr := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errgo.Newf("cannot listen on %s: %v", addr, err)
	}
	tlsListener := tls.NewListener(
		tcpKeepAliveListener{listener.(*net.TCPListener)},
		config,
	)
	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	return newHandlerListener(server, tlsListener), nil
}

// set sets the current state of the server, and starts
// or restarts the HTTP listener when appropriate.
func (srv *server) set(state ServerState, fb *Feedback) {
	restartNeeded := false
	noStart := false
	if err := srv.setRelations(state); err != nil {
		switch cause := errgo.Cause(err); cause {
		case ErrRestartNeeded:
			restartNeeded = true
		case ErrRelationIncomplete:
			restartNeeded = true
			fb.addError(err)
			noStart = true
		default:
			fb.addError(err)
			noStart = true
		}
	}
	restartNeeded = restartNeeded || srv.needsRestart(state)
	log.Printf("srv set state %#v; current state %#v", state, srv.state)
	if restartNeeded {
		srv.closeResources()
	}
	if noStart {
		return
	}
	httpOK := state.HTTPPort != 0
	httpsOK := state.HTTPSPort != 0 && state.CertPEM != ""
	h, err := srv.handlerInfo.handler(state.ArgData, srv.relationState)
	if err != nil {
		fb.addError(errgo.Notef(err, "cannot get handler"))
		return
	}
	if srv.httpListener == nil && httpOK {
		log.Printf("starting HTTP server on port %v", state.HTTPPort)
		srv.httpListener, err = srv.serveHTTP(state.HTTPPort, h)
		if err != nil {
			fb.addError(errgo.Notef(err, "cannot start HTTP server"))
		}
	}
	if srv.httpsListener == nil && httpsOK {
		log.Printf("starting HTTPS server on port %v", state.HTTPSPort)
		srv.httpsListener, err = srv.serveHTTPS(state.HTTPSPort, state.CertPEM, h)
		if err != nil {
			fb.addError(errgo.Notef(err, "cannot start HTTPS server"))
		}
	}
	srv.handler = h
	srv.state = state
	return
}

type handlerListener struct {
	tomb tomb.Tomb
	lis  net.Listener
}

func newHandlerListener(server *http.Server, lis net.Listener) *handlerListener {
	hl := &handlerListener{
		lis: lis,
	}
	hl.tomb.Go(func() error {
		if err := server.Serve(lis); err != nil {
			return errgo.Notef(err, "listener on %s died", lis.Addr())
		}
		return nil
	})
	return hl
}

func (hl *handlerListener) Kill() {
	hl.lis.Close()
}

func (hl *handlerListener) Wait() error {
	hl.tomb.Wait()
	return nil
}

// needsRestart reports whether the server
// needs a restart when the state is changed to the given
// state.
func (srv *server) needsRestart(state ServerState) bool {
	// Note that when the mongodb addresses change, that
	// doesn't necessarily imply we'll need a restart,
	// as mongodb informs all clients of the new addresses
	// as a matter of course. We'll store the addresses however,
	// and they'll be used when reconnecting.
	return state.HTTPPort == 0 ||
		state.HTTPPort != srv.state.HTTPPort ||
		state.HTTPSPort != srv.state.HTTPSPort ||
		state.CertPEM != srv.state.CertPEM ||
		!bytes.Equal(state.ArgData, srv.state.ArgData)
}

// setRelations sets the relation fields from the information in the
// given state. If it returns ErrRestartNeeded, the handler
// must be restarted.
func (srv *server) setRelations(state ServerState) error {
	h := srv.handlerInfo
	if h.relationType == nil {
		return nil
	}
	if !srv.relationState.IsValid() {
		srv.relationState = reflect.New(h.relationType).Elem()
	}

	// First check that we have all our required relations
	incomplete := false
	for i := 0; i < h.relationType.NumField(); i++ {
		f := h.relationType.Field(i)
		data := state.RelationValues[f.Name]
		// If there's no data, it indicates that the relation is incomplete.
		if len(data) == 0 {
			incomplete = true
		}
	}
	if incomplete {
		return ErrRelationIncomplete
	}

	// For each field, unmarshal the relation data into the
	// expected type and call the appropriate setFieldFunc value.
	restartNeeded := false
	for i := 0; i < h.relationType.NumField(); i++ {
		f := h.relationType.Field(i)
		// TODO we could potentially do all these in parallel
		// so that any network connections they might be
		// making would be concurrent.
		if err := srv.setFieldValue(f, state); err != nil {
			if errgo.Cause(err) == ErrRestartNeeded {
				restartNeeded = true
			} else {
				return errgo.Notef(err, "cannot set field %s", f.Name)
			}
		}
	}
	if restartNeeded {
		return ErrRestartNeeded
	}
	return nil
}

// setFieldValue sets the value of the given struct field from the
// relation data for that field held in state.RelationValues.
func (srv *server) setFieldValue(f reflect.StructField, state ServerState) error {
	registeredRelationsMutex.Lock()
	reg := registeredRelations[f.Type]
	registeredRelationsMutex.Unlock()
	if reg == nil {
		// This shouldn't be able to happen because we should have
		// checked that all fields have registered types during Register.
		panic(errgo.Newf("no registered relation for type %s in field %s", f.Type, f.Name))
	}
	args := []reflect.Value{
		srv.relationState.Field(0).Addr(),
		reflect.New(reg.d),
	}

	// Unmarshal the data into the newly created  instance of type D.
	data := state.RelationValues[f.Name]
	if err := json.Unmarshal(data, args[1].Interface()); err != nil {
		return errgo.Notef(err, "cannot unmarshal relation data (type %v, data %q)", args[1].Type(), data)
	}
	args[1] = args[1].Elem()
	err, _ := reg.setFieldValue.Call(args)[0].Interface().(error)
	if err != nil {
		return errgo.Mask(err, errgo.Is(ErrRestartNeeded))
	}
	return nil
}

// closeResources closes any current listeners. Called
// with srv.mu held.
func (srv *server) closeResources() {
	if srv.handler != nil {
		srv.handler.Close()
		srv.handler = nil
	}
	if srv.httpListener != nil {
		srv.httpListener.Kill()
		srv.httpListener.Wait()
		srv.httpListener = nil
	}
	if srv.httpsListener != nil {
		srv.httpsListener.Kill()
		srv.httpsListener.Wait()
		srv.httpsListener = nil
	}
}

func (srv *server) saveState(state ServerState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return errgo.Mask(err)
	}
	path := srv.statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errgo.Mask(err)
	}
	if err := ioutil.WriteFile(path, data, 0600); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (srv *server) loadState() (ServerState, error) {
	data, err := ioutil.ReadFile(srv.statePath())
	if os.IsNotExist(err) {
		return ServerState{}, nil
	}
	if err != nil {
		return ServerState{}, errgo.Mask(err)
	}
	var state ServerState
	if err := json.Unmarshal(data, &state); err != nil {
		return ServerState{}, errgo.Notef(err, "cannot unmarshal state %q", data)
	}
	return state, nil
}

func (srv *server) statePath() string {
	return filepath.Join(srv.stateDir, "serverstate.json")
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
