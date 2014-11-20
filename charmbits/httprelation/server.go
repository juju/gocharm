// The httprelation package can be used in a charm
// that uses a relation with interface type "http".
package httprelation

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"

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
	OpenedPort int
}

// Provider represents the provider of an http relation.
type Provider struct {
	state        providerState
	ctxt         *hook.Context
	relationName string
	oldState     providerState
	changed      PortChanger
}

// PortChanger is the type of a value that can be informed
// about an HTTP server port change.
// If the port is zero, there is no valid port and
// the server should not be run.
type PortChanger interface {
	HTTPServerPortChanged(port int) error
}

// Register registers everything necessary on r for running the provider
// side of an http relation with the given relation name.
//
// It takes care of opening and closing the configured port, but does
// not actually start the HTTP handler server. If changed is not nil, it
// notifies that the port has changed by calling
// changed.HTTPServerPortChanged.
//
// The port of the server is configured with the "server-port" charm
// configuration option.
func (p *Provider) Register(r *hook.Registry, relationName string, changed PortChanger) {
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
	r.RegisterHook(relationName+"-relation-joined", p.relationJoined)
	r.RegisterHook("config-changed", p.configChanged)
	r.RegisterContext(p.setContext, &p.state)
	p.relationName = relationName
	p.changed = changed
}

func (p *Provider) setContext(ctxt *hook.Context) error {
	p.ctxt = ctxt
	p.oldState = p.state
	return nil
}

var ErrAddressNotConfigured = fmt.Errorf("private address not configured")

// PrivateAddress returns the TCP address of the HTTP server.
// If the server is not running, it returns ErrAddressNotConfigured.
func (p *Provider) PrivateAddress() (string, error) {
	if p.state.OpenedPort == 0 {
		return "", ErrAddressNotConfigured
	}
	addr, err := p.ctxt.PrivateAddress()
	if err != nil {
		return "", errors.Wrap(err)
	}
	return net.JoinHostPort(addr, strconv.Itoa(p.state.OpenedPort)), nil
}

// Port returns the configured port of the server.
// If the port has not been set, it returns 0.
func (p *Provider) Port() int {
	return p.state.OpenedPort
}

func (p *Provider) configChanged() error {
	var port int
	if port0, err := p.ctxt.GetConfig("server-port"); err != nil {
		return errors.Wrapf(err, "cannot set server context")
	} else {
		if port = int(port0.(float64)); port <= 0 || port >= 65535 {
			p.ctxt.Logf("ignoring invalid port %v", port0)
			// TODO status-set appropriately if/when status-set is implemented
			return nil
		}
	}
	if port == p.state.OpenedPort {
		return nil
	}
	if p.state.OpenedPort != 0 {
		// Could check actually opened ports here to be
		// more resilient against previous errors.
		if err := p.ctxt.ClosePort("tcp", p.state.OpenedPort); err != nil {
			return errors.Wrap(err)
		}
		p.state.OpenedPort = 0
	}
	if err := p.ctxt.OpenPort("tcp", port); err != nil {
		return errors.Wrap(err)
	}
	p.state.OpenedPort = port
	if p.changed != nil {
		if err := p.changed.HTTPServerPortChanged(port); err != nil {
			return errors.Wrap(err)
		}
	}
	addr, err := p.PrivateAddress()
	if err != nil {
		return errors.Wrap(err)
	}
	// Set the current address in all requirers.
	for _, id := range p.ctxt.RelationIds[p.relationName] {
		if err := p.setRelationAddress(id, addr); err != nil {
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

func (p *Provider) setRelationAddress(relId hook.RelationId, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.Wrapf(err, "cannot split host/port")
	}
	if err := p.ctxt.SetRelationWithId(relId, "port", port, "hostname", host); err != nil {
		return errors.Wrap(err)
	}
	return nil
}
