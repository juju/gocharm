// The httprelation package can be used in a charm
// that uses a relation with interface type "http".
package httprelation

import (
	"strconv"

	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/charmbits/simplerelation"
	"github.com/juju/gocharm/hook"
)

// providerState holds the persistent charm state for the provider part of
// the charm.
type providerState struct {
	OpenedPort int
}

// Provider represents the provider of an http relation.
type Provider struct {
	prov  simplerelation.Provider
	state providerState
	ctxt  *hook.Context
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
func (p *Provider) Register(r *hook.Registry, relationName string) {
	p.prov.Register(r.Clone("http"), relationName, "http")
	r.RegisterConfig("server-port", charm.Option{
		Type:        "int",
		Description: "Port for the HTTP server to listen on",
		Default:     8080,
	})
	r.RegisterHook("config-changed", p.configChanged)
	r.RegisterContext(p.setContext, &p.state)
}

func (p *Provider) setContext(ctxt *hook.Context) error {
	p.ctxt = ctxt
	return nil
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
	if port == 0 {
		return p.prov.SetValues(map[string]string{
			"hostname": "",
			"port":     "",
		})
	}
	if err := p.ctxt.OpenPort("tcp", port); err != nil {
		return errors.Wrap(err)
	}
	p.state.OpenedPort = port
	addr, err := p.ctxt.PrivateAddress()
	if err != nil {
		return errors.Wrap(err)
	}
	if err := p.prov.SetValues(map[string]string{
		"hostname": addr,
		"port":     strconv.Itoa(p.state.OpenedPort),
	}); err != nil {
		return errors.Wrap(err)
	}
	return nil
}
