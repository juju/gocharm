// The httpserver package can be used in a charm to implement an
// http server. The port of the server is configured with the
// "server-port" charm configuration option, which should
// be added to config.yaml with integer type.
package httpserver

import (
	"flag"
	"fmt"
	"launchpad.net/errgo/errors"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-utils/hook"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
)

// serverCommand implements the http server for the charm. It's invoked
// independently of hook context.
func serverCommand(handler http.Handler) {
	serverAddr := flag.String("http", "", "HTTP service address (e.g. :8080)")
	flag.Parse()
	log.Fatal(http.ListenAndServe(*serverAddr, handler))
}

// serverState holds the persistent charm state for the server part of
// the charm
type serverState struct {
	Installed     bool
	InstalledPort *int
}

type server struct {
	serverState
	ctxt           *hook.Context
	configuredPort *int
}

// Register registers the handlers and commands necessary for
// starting an http server in a charm.
func Register(r *hook.Registry, handler http.Handler) {
	serverRegister(r, "config-changed", (*server).configChanged)
	serverRegister(r, "stop", (*server).uninstall)
	r.RegisterCommand("server", func() {
		serverCommand(handler)
	})
}

func serverRegister(r *hook.Registry, hookName string, f func(*server) error) {
	r.Register(hookName, func(ctxt *hook.Context) error {
		srv, err := getServer(ctxt)
		if err != nil {
			return errors.Wrap(err)
		}
		return f(srv)
	})
}

// getServer returns the charm's HTTP server state.
//
// Although it returns a new instance of the server type, its
// serverState fields are persistently stored using the hook package's
// local-state mechanism.
func getServer(ctxt *hook.Context) (*server, error) {
	srv := &server{
		ctxt: ctxt,
	}
	port0, err := ctxt.GetConfig("server-port")
	if err != nil {
		return nil, errors.Wrap(err)
	}
	if port0 != nil {
		port := int(port0.(float64))
		if 0 < port && port < 65535 {
			srv.configuredPort = &port
		} else {
			ctxt.Logf("ignoring invalid port %v", port0)
		}
	}
	if err := ctxt.LocalState("server", &srv.serverState); err != nil {
		return nil, errors.Wrap(err)
	}
	return srv, nil
}

func (srv *server) upstartService() *upstart.Service {
	return &upstart.Service{
		Name:    "concat-webserver-" + names.UnitTag(srv.ctxt.Unit),
		InitDir: "/etc/init",
	}
}

func (srv *server) install() error {
	if srv.Installed || srv.configuredPort == nil {
		return nil
	}
	// Ask for the new port before trying anything else.
	if err := srv.ctxt.OpenPort("tcp", *srv.configuredPort); err != nil {
		return errors.Wrap(err)
	}
	exe := filepath.Join(srv.ctxt.CharmDir, "bin", "runhook")
	conf := &upstart.Conf{
		Service: *srv.upstartService(),
		Desc:    "web server for concat charm",
		Cmd: fmt.Sprintf("%s %s -http ':%d'",
			exe,
			srv.ctxt.CommandName("server"),
			*srv.configuredPort,
		),
		// TODO save output somewhere - we need a better answer for that.
	}
	if err := conf.Install(); err != nil {
		return errors.Wrap(err)
	}
	srv.Installed = true
	srv.InstalledPort = srv.configuredPort
	return nil
}

func (srv *server) uninstall() error {
	if !srv.Installed {
		return nil
	}
	if err := srv.ctxt.ClosePort("tcp", *srv.InstalledPort); err != nil {
		return errors.Wrap(err)
	}
	if err := srv.upstartService().StopAndRemove(); err != nil {
		return errors.Wrap(err)
	}
	srv.Installed = false
	srv.InstalledPort = nil
	return nil
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(errors.Newf("atoi without valid integer on %q", s))
	}
	return n
}

func (srv *server) configChanged() error {
	switch {
	case !srv.Installed:
		if err := srv.install(); err != nil {
			return errors.Wrapf(err, "could not install server")
		}
	case srv.configuredPort == nil || *srv.configuredPort != *srv.InstalledPort:
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
