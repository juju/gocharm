package concat

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/charmbits/service"
)

// startServer starts the server running. The
// actual HTTP server will not start until the
// initial value and port have been set.
func startServer(ctxt *service.Context, args []string) {
	log.Printf("concat server started %q", args)
	if len(args) != 1 {
		fatalf("need exactly one argument, got %q", args)
		os.Exit(2)
	}
	srv := &ConcatServer{
		stateDir: args[0],
	}
	state, err := srv.loadState()
	if err != nil {
		fatalf("cannot load state: %v", err)
	}
	if err := srv.set(state); err != nil {
		fatalf("cannot set initial state: %v", err)
	}
	ctxt.ServeLocalRPC(srv)
}

func fatalf(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf(f, a...))
	os.Exit(1)
}

// ConcatServer defines the operations on the service's local
// RPC server that is used for communication between the
// charm and the running service.
//
// Note that this type must be exported because of
// the rules about net/rpc types.
type ConcatServer struct {
	mu       sync.Mutex
	stateDir string
	state    ServerState
	listener net.Listener
}

// ServerState holds the state of a server.
type ServerState struct {
	Val  string
	Port int
}

// SetVal sets the current value served by the HTTP server
// and starts the server running if it is not already started.
func (svc *ConcatServer) Set(p *ServerState, _ *struct{}) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if err := svc.set(*p); err != nil {
		return errors.Wrapf(err, "canot set server state")
	}
	if err := svc.saveState(svc.state); err != nil {
		return errors.Wrapf(err, "cannot save server state")
	}
	return nil
}

// set sets the current state of the server, and starts
// or restarts the HTTP listener when appropriate.
func (svc *ConcatServer) set(state ServerState) error {
	log.Printf("concat set state %#v; current state %#v", state, svc.state)
	if state.Port == 0 || state.Port != svc.state.Port {
		if svc.listener != nil {
			log.Printf("closing listener")
			svc.listener.Close()
			svc.listener = nil
		}
	}
	if svc.listener == nil && state.Port != 0 {
		log.Printf("listening on %d", state.Port)
		addr := ":" + strconv.Itoa(state.Port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return errors.Wrap(err)
		}
		svc.listener = listener
		go func() {
			server := &http.Server{
				Addr:    addr,
				Handler: http.HandlerFunc(svc.serveHTTP),
			}
			server.Serve(svc.listener)
		}()
	}
	svc.state = state
	return nil
}

// serveHTTP implements the HTTP handler to serve the current value of
// the unit.
//
// This is unexported so that we don't trigger a log message from the
// net/rpc package.
func (svc *ConcatServer) serveHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/val" {
		http.NotFound(w, req)
		return
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	switch req.Method {
	case "GET":
		fmt.Fprintf(w, "%s", svc.state.Val)
	default:
		http.Error(w, "unsupported method", http.StatusBadRequest)
	}
}

func (svc *ConcatServer) saveState(state ServerState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return errors.Wrap(err)
	}
	path := svc.statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.Wrap(err)
	}
	if err := ioutil.WriteFile(path, data, 0600); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (svc *ConcatServer) loadState() (ServerState, error) {
	data, err := ioutil.ReadFile(svc.statePath())
	if os.IsNotExist(err) {
		return ServerState{}, nil
	}
	if err != nil {
		return ServerState{}, errors.Wrap(err)
	}
	var state ServerState
	if err := json.Unmarshal(data, &state); err != nil {
		return ServerState{}, errors.Wrapf(err, "cannot unmarshal state %q", data)
	}
	return state, nil
}

func (svc *ConcatServer) statePath() string {
	return filepath.Join(svc.stateDir, "serverstate.json")
}
