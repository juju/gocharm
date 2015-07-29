package service

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"

	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/gocharm/hook"
)

type serviceParams struct {
	SocketPath string
	Args       []string
}

// runServer runs the server side of the service. It is invoked
// (indirectly) by upstart.
func runServer(start func(ctxt *Context, args []string) (hook.Command, error), args []string) (hook.Command, error) {
	log.Printf("server started %q", args)
	if len(args) != 1 {
		return nil, errgo.Newf("expected exactly one argument, found %q", args)
	}
	pdata, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		return nil, errgo.Notef(err, "cannot base64 decode argument")
	}
	var p serviceParams
	if err := json.Unmarshal(pdata, &p); err != nil {
		return nil, errgo.Notef(err, "cannot json unmarshal argument %q", pdata)
	}
	ctxt := &Context{
		socketPath: p.SocketPath,
	}
	return start(ctxt, p.Args)
}

// Context holds the context provided to a running service.
type Context struct {
	socketPath string
}

type rpcCommand struct {
	tomb     tomb.Tomb
	listener net.Listener
}

func (c *rpcCommand) Kill() {
	c.listener.Close()
}

func (c *rpcCommand) Wait() error {
	return c.tomb.Wait()
}

func (c *rpcCommand) run(srv *rpc.Server) {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			log.Printf("local socket accept failed: %v", err)
			return
		}
		log.Printf("local RPC accepted dial request")
		// TODO shut down existing RPC requests cleanly
		go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}

// ServeLocalRPC starts a local RPC server serving methods on the given
// receiver value, using the net/rpc package (see rpc.Server.Register).
//
// The methods may be invoked using the Service.Call method. Parameters
// and return values will be marshaled as JSON.
//
// ServeLocalRPC returns the Command representing the running
// service.
func (ctxt *Context) ServeLocalRPC(rcvr interface{}) (hook.Command, error) {
	srv := rpc.NewServer()
	srv.Register(rcvr)
	listener, err := listen(ctxt.socketPath)
	if err != nil {
		return nil, errgo.Notef(err, "cannot listen on local socket")
	}
	cmd := &rpcCommand{
		listener: listener,
	}
	log.Printf("accepting local service on %s", ctxt.socketPath)
	cmd.tomb.Go(func() error {
		cmd.run(srv)
		return nil
	})
	return cmd, nil
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
