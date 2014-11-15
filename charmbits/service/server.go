package service

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"

	"launchpad.net/errgo/errors"
)

func runServer(serviceRPC interface{}, args []string) {
	os.Args = append([]string{"runhook"}, args...)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: service <socketpath>\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	listener, err := listen(flag.Arg(0))
	if err != nil {
		fatalf("cannot listen: %v", err)
	}
	srv := rpc.NewServer()
	srv.Register(serviceRPC)
	srv.Accept(listener)
}

func listen(socketPath string) (net.Listener, error) {
	// In case the unix socket is present, delete it.
	os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot listen on unix socket")
	}
	return listener, err
}

func fatalf(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, f, a...)
	os.Exit(1)
}
