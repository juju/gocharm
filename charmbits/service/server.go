package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"

	"gopkg.in/errgo.v1"
)

type serviceParams struct {
	SocketPath string
	Args       []string
}

// runServer runs the server side of the service. It is invoked
// (indirectly) by upstart.
func runServer(start func(ctxt *Context, args []string), args []string) {
	log.Printf("server started %q", args)
	if len(args) != 1 {
		fatalf("expected exactly one argument, found %q", args)
	}
	pdata, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		fatalf("cannot base64 decode argument: %v", err)
	}
	var p serviceParams
	if err := json.Unmarshal(pdata, &p); err != nil {
		fatalf("cannot json unmarshal argument %q: %v", pdata, err)
	}
	ctxt := &Context{
		socketPath: p.SocketPath,
	}
	start(ctxt, p.Args)
}

func fatalf(f string, a ...interface{}) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf(f, a...))
	os.Exit(2)
}

// Context holds the context provided to a running service.
type Context struct {
	socketPath string
}

// ServeLocalRPC starts a local RPC server serving methods on the given
// receiver value, using the net/rpc package (see rpc.Server.Register).
//
// The methods may be invoked using the Service.Call method. Parameters
// and return values will be marshaled as JSON.
//
// ServeLocalRPC blocks indefinitely.
func (ctxt *Context) ServeLocalRPC(rcvr interface{}) error {
	srv := rpc.NewServer()
	srv.Register(rcvr)
	listener, err := listen(ctxt.socketPath)
	if err != nil {
		return errgo.Notef(err, "cannot listen on local socket")
	}
	for {
		log.Printf("accepting local service on %s", ctxt.socketPath)
		conn, err := listener.Accept()
		if err != nil {
			return errgo.Notef(err, "local socket accept failed")
		}
		log.Printf("accepted dial request")
		go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}

func listen(socketPath string) (net.Listener, error) {
	// In case the unix socket is present, delete it.
	os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, errgo.Notef(err, "cannot listen on unix socket")
	}
	return listener, err
}
