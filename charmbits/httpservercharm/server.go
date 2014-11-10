// The httpservercharm package can be used in a charm to implement an
// http server. The port of the server is configured with the
// "server-port" charm configuration option, which should
// be added to config.yaml with integer type.
//
// This package is currently highly experimental.
package httpservercharm

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"

	serviceCommon "github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/hook"
)

// serverCommand implements the http server for the charm. It's invoked
// independently of hook context.
func serverCommand(newHandler func() http.Handler) {
	serverAddr := flag.String("http", "", "HTTP service address (e.g. :8080)")
	flag.Parse()
	log.Fatal(http.ListenAndServe(*serverAddr, newHandler()))
}

// serverState holds the persistent charm state for the server part of
// the charm.
type serverState struct {
	Installed     bool
	InstalledPort *int
}

type Server struct {
	state          *serverState
	ctxt           *hook.Context
	configuredPort *int
}

// ServerGetter returns a Server instance from a hook context.
type ServerGetter func(*hook.Context) (*Server, error)

// Register registers the handlers and commands necessary for
// starting an http server in a charm that will serve content
// using the handler created by calling newHandler.
func (srv *Server) Register(r *hook.Registry, newHandler func() http.Handler) {
	r.RegisterContext(srv.setContext)
	r.RegisterConfig("server-port", charm.Option{
		Type:        "int",
		Description: "Port for the HTTP server to listen on",
		Default:     8080,
	})
	r.RegisterHook("config-changed", srv.configChanged)
	r.RegisterHook("stop", srv.uninstall)
	r.RegisterCommand("server", func() {
		serverCommand(newHandler)
	})
}

func (srv *Server) setContext(ctxt *hook.Context) error {
	port0, err := ctxt.GetConfig("server-port")
	if err != nil {
		return errors.Wrapf(err, "cannot set server context")
	}
	if port0 != nil {
		port := int(port0.(float64))
		if 0 < port && port < 65535 {
			srv.configuredPort = &port
		} else {
			ctxt.Logf("ignoring invalid port %v", port0)
		}
	}
	if err := ctxt.LocalState("server", &srv.state); err != nil {
		return errors.Wrap(err)
	}
	srv.ctxt = ctxt
	return nil
}

// PrivateAddress returns the TCP address of the HTTP server.
// If the server is not running, it returns the empty string.
func (srv *Server) PrivateAddress() (string, error) {
	if !srv.state.Installed {
		return "", nil
	}
	addr, err := srv.ctxt.PrivateAddress()
	if err != nil {
		return "", errors.Wrap(err)
	}
	return fmt.Sprintf("%s:%d", addr, *srv.state.InstalledPort), nil
}

func (srv *Server) upstartService() *upstart.Service {
	exe := filepath.Join(srv.ctxt.CharmDir, "bin", "runhook")
	return &upstart.Service{
		Name: "concat-webserver-" + names.NewUnitTag(srv.ctxt.Unit).String(),
		Conf: serviceCommon.Conf{
			InitDir: "/etc/init",
			Desc:    "web server for concat charm",
			Cmd: fmt.Sprintf("%s %s -http ':%d'",
				exe,
				srv.ctxt.CommandName("server"),
				*srv.configuredPort,
			),
			// TODO save output somewhere - we need a better answer for that.
		},
	}
}

func (srv *Server) install() error {
	if srv.state.Installed || srv.configuredPort == nil {
		return nil
	}
	// Ask for the new port before trying anything else.
	if err := srv.ctxt.OpenPort("tcp", *srv.configuredPort); err != nil {
		return errors.Wrap(err)
	}
	if err := srv.upstartService().Install(); err != nil {
		return errors.Wrap(err)
	}
	srv.state.Installed = true
	srv.state.InstalledPort = srv.configuredPort
	return nil
}

func (srv *Server) uninstall() error {
	if !srv.state.Installed {
		return nil
	}
	if err := srv.ctxt.ClosePort("tcp", *srv.state.InstalledPort); err != nil {
		return errors.Wrap(err)
	}
	if err := srv.upstartService().StopAndRemove(); err != nil {
		return errors.Wrap(err)
	}
	srv.state.Installed = false
	srv.state.InstalledPort = nil
	return nil
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(errors.Newf("atoi without valid integer on %q", s))
	}
	return n
}

func (srv *Server) configChanged() error {
	switch {
	case !srv.state.Installed:
		if err := srv.install(); err != nil {
			return errors.Wrapf(err, "could not install server")
		}
	case srv.configuredPort == nil || *srv.configuredPort != *srv.state.InstalledPort:
		// The port has changed - reinstall server with new port configured.
		if err := srv.uninstall(); err != nil {
			return errors.Wrapf(err, "could not uninstall server")
		}
		if err := srv.install(); err != nil {
			return errors.Wrapf(err, "could not reinstall server")
		}
	}
	return nil
}
