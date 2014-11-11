// The httpcharm package can be used in a charm
// that uses a relation with interface type "http".
//
// This package is currently experimental.
package httpcharm

import (
	"flag"
	"fmt"
	"log"
	"net"
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

// providerState holds the persistent charm state for the provider part of
// the charm.
type providerState struct {
	Installed     bool
	InstalledPort *int
}

// Provider represents the provider of an http relation.
type Provider struct {
	state          *providerState
	ctxt           *hook.Context
	configuredPort *int
	relationName   string
}

// Register registers the handlers and commands necessary for
// starting an http server in a charm that will serve content
// using the handler created by calling newHandler.
//
// The port of the server is configured with the
// "server-port" charm configuration option, which should
// be added to config.yaml with integer type.
//
// It provides an http relation with the given relation name.
func (p *Provider) Register(r *hook.Registry, relationName string, newHandler func() http.Handler) {
	p.relationName = relationName
	r.RegisterContext(p.setContext)
	r.RegisterRelation(charm.Relation{
		Name:      relationName,
		Interface: "http",
		Role:      charm.RoleProvider,
		Scope:     charm.ScopeGlobal,
	})
	r.RegisterConfig("server-port", charm.Option{
		Type:        "int",
		Description: "Port for the HTTP server to listen on",
		Default:     8080,
	})
	r.RegisterHook("relation-"+relationName+"-joined", p.relationJoined)
	r.RegisterHook("config-changed", p.configChanged)
	r.RegisterHook("stop", p.uninstall)
	r.RegisterCommand("server", func() {
		serverCommand(newHandler)
	})
}

func (p *Provider) setContext(ctxt *hook.Context) error {
	port0, err := ctxt.GetConfig("server-port")
	if err != nil {
		return errors.Wrapf(err, "cannot set server context")
	}
	if port0 != nil {
		port := int(port0.(float64))
		if 0 < port && port < 65535 {
			p.configuredPort = &port
		} else {
			ctxt.Logf("ignoring invalid port %v", port0)
		}
	}
	if err := ctxt.LocalState("server", &p.state); err != nil {
		return errors.Wrap(err)
	}
	p.ctxt = ctxt
	return nil
}

// PrivateAddress returns the TCP address of the HTTP server.
// If the server is not running, it returns the empty string.
func (p *Provider) PrivateAddress() (string, error) {
	if !p.state.Installed {
		return "", nil
	}
	addr, err := p.ctxt.PrivateAddress()
	if err != nil {
		return "", errors.Wrap(err)
	}
	return net.JoinHostPort(addr, strconv.Itoa(*p.state.InstalledPort)), nil
}

func (p *Provider) upstartService() *upstart.Service {
	exe := filepath.Join(p.ctxt.CharmDir, "bin", "runhook")
	return &upstart.Service{
		Name: "concat-webserver-" + names.NewUnitTag(p.ctxt.Unit).String(),
		Conf: serviceCommon.Conf{
			InitDir: "/etc/init",
			Desc:    "web server for concat charm",
			Cmd: fmt.Sprintf("%s %s -http ':%d'",
				exe,
				p.ctxt.CommandName("server"),
				*p.configuredPort,
			),
			// TODO save output somewhere - we need a better answer for that.
		},
	}
}

func (p *Provider) install() error {
	if p.state.Installed || p.configuredPort == nil {
		return nil
	}
	// Ask for the new port before trying anything else.
	if err := p.ctxt.OpenPort("tcp", *p.configuredPort); err != nil {
		return errors.Wrap(err)
	}
	if err := p.upstartService().Install(); err != nil {
		return errors.Wrap(err)
	}
	p.state.Installed = true
	p.state.InstalledPort = p.configuredPort
	return nil
}

func (p *Provider) uninstall() error {
	if !p.state.Installed {
		return nil
	}
	if err := p.ctxt.ClosePort("tcp", *p.state.InstalledPort); err != nil {
		return errors.Wrap(err)
	}
	if err := p.upstartService().StopAndRemove(); err != nil {
		return errors.Wrap(err)
	}
	p.state.Installed = false
	p.state.InstalledPort = nil
	return nil
}

func (p *Provider) configChanged() error {
	switch {
	case !p.state.Installed:
		if err := p.install(); err != nil {
			return errors.Wrapf(err, "could not install server")
		}
	case p.configuredPort == nil || *p.configuredPort != *p.state.InstalledPort:
		// The port has changed - reinstall server with new port configured.
		if err := p.uninstall(); err != nil {
			return errors.Wrapf(err, "could not uninstall server")
		}
		if err := p.install(); err != nil {
			return errors.Wrapf(err, "could not reinstall server")
		}
	default:
		return nil
	}
	addr, err := p.PrivateAddress()
	if err != nil {
		return errors.Wrap(err)
	}
	// Set the current address in all requirers.
	ids, err := p.ctxt.RelationIds(p.relationName)
	if err != nil {
		return errors.Wrap(err)
	}
	for _, id := range ids {
		if err := p.ctxt.SetRelationWithId(id, addr); err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

func (p *Provider) relationJoined() error {
	addr, err := p.PrivateAddress()
	if err != nil {
		return errors.Wrap(err)
	}
	if addr == "" {
		return nil
	}
	if err := p.setRelationAddress(p.ctxt.RelationId, addr); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (p *Provider) setRelationAddress(relId string, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.Wrapf(err, "cannot split host/port")
	}
	if err := p.ctxt.SetRelationWithId(relId, "port", port, "hostname", host); err != nil {
		return errors.Wrap(err)
	}
	return nil
}
