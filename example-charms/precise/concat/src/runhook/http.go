package runhook

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

// ConcatServer defines the operations on the service's local
// RPC server that is used for communication between the
// charm and the running service.
//
// Note that this type must be exported because of
// the rules about net/rpc types.
type ConcatServer struct {
	mu      sync.Mutex
	port    int
	val     string
	started bool
}

// StartParams holds the parameters for a ConcatServer.Start call.
type StartParams struct {
	Port int
}

// Start starts the service running with the given start parameters.
// This should only be called once, just after the service is run.
func (svc *ConcatServer) Start(p *StartParams, _ *struct{}) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.started {
		return fmt.Errorf("server already started")
	}
	go http.ListenAndServe(":"+strconv.Itoa(p.Port), http.HandlerFunc(svc.serveHTTP))
	svc.started = true
	return nil
}

// SetValParams holds the parameters for a ConcatServer.SetVal call.
type SetValParams struct {
	Val string
}

func (svc *ConcatServer) SetVal(p *SetValParams, _ *struct{}) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.val = p.Val
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
		fmt.Fprintf(w, "%s", svc.val)
	default:
		http.Error(w, "unsupported method", http.StatusBadRequest)
	}
}
