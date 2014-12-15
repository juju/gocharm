// The httprelation package can be used in a charm
// that uses a relation with interface type "http".
package httprelation

import (
	"strconv"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/gocharm/charmbits/simplerelation"
	"github.com/juju/gocharm/hook"
)

// providerState holds the persistent charm state for the provider part of
// the charm.
type providerState struct {
	OpenedHTTPPort  int
	OpenedHTTPSPort int
}

// Provider represents the provider of an http relation.
type Provider struct {
	prov       simplerelation.Provider
	state      providerState
	ctxt       *hook.Context
	allowHTTPS bool
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
func (p *Provider) Register(r *hook.Registry, relationName string, allowHTTPS bool) {
	p.allowHTTPS = allowHTTPS
	// TODO provide https relation?
	p.prov.Register(r.Clone("http"), relationName, "http")
	r.RegisterConfig("http-port", charm.Option{
		Type:        "int",
		Description: "Port for the HTTP server to listen on",
		Default:     80,
	})
	if p.allowHTTPS {
		r.RegisterConfig("https-certificate", charm.Option{
			Type:        "string",
			Description: "Certificate and key for https server in PEM format. If this is not set, no https server will be run",
		})
		r.RegisterConfig("https-port", charm.Option{
			Type:        "int",
			Description: "Port for the HTTP server to listen on",
			Default:     443,
		})
	}
	r.RegisterHook("install", p.configChanged)
	r.RegisterHook("config-changed", p.configChanged)
	r.RegisterContext(p.setContext, &p.state)
}

func (p *Provider) setContext(ctxt *hook.Context) error {
	p.ctxt = ctxt
	return nil
}

// HTTPPort returns the configured port of the HTTP server.
// If the port has not been set, it returns 0.
func (p *Provider) HTTPPort() int {
	return p.state.OpenedHTTPPort
}

// HTTPPort returns the configured port of the HTTP server.
// If the port has not been set, it returns 0.
func (p *Provider) HTTPSPort() int {
	return p.state.OpenedHTTPSPort
}

// HTTPSPort returns the configured port of the HTTPS server.
// If the port has not been set, or there is no cert provided, it returns 0.
func (p *Provider) configChanged() error {
	if err := p.configurePort(&p.state.OpenedHTTPPort, "http-port"); err != nil {
		return errgo.Mask(err)
	}
	if p.allowHTTPS {
		// If the TLSCert is invalid, ignore it.
		if _, err := p.TLSCertPEM(); err == nil {
			if err := p.configurePort(&p.state.OpenedHTTPSPort, "https-port"); err != nil {
				return errgo.Mask(err)
			}
		}
	}
	if p.state.OpenedHTTPPort == 0 {
		return p.prov.SetValues(map[string]string{
			"hostname": "",
			"port":     "",
		})
	}
	addr, err := p.ctxt.PrivateAddress()
	if err != nil {
		return errgo.Mask(err)
	}
	if err := p.prov.SetValues(map[string]string{
		"hostname": addr,
		"port":     strconv.Itoa(p.state.OpenedHTTPPort),
	}); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

var ErrHTTPSNotConfigured = errgo.New("HTTPS not configured")

// TLSCertPEM returns the currently configured server certificate
// in PEM format. It returns ErrHTTPSNotConfigured if there is no currently
// configured certificate.
func (p *Provider) TLSCertPEM() (string, error) {
	if !p.allowHTTPS {
		return "", ErrHTTPSNotConfigured
	}
	certPEM, err := p.ctxt.GetConfigString("https-certificate")
	if err != nil {
		return "", errgo.Mask(err)
	}
	return certPEM, nil
}

func (p *Provider) configurePort(openedPort *int, configKey string) error {
	port, err := p.ctxt.GetConfigInt(configKey)
	if err != nil {
		return errgo.Notef(err, "cannot get %s", configKey)
	}
	if port <= 0 || port >= 65535 {
		p.ctxt.Logf("ignoring invalid %s %v", configKey, port)
		// TODO status-set appropriately if/when status-set is implemented
		return nil
	}
	if port == *openedPort {
		return nil
	}
	if *openedPort != 0 {
		// Could check actually opened ports here to be
		// more resilient against previous errors.
		if err := p.ctxt.ClosePort("tcp", *openedPort); err != nil {
			return errgo.Mask(err)
		}
		*openedPort = 0
	}
	if err := p.ctxt.OpenPort("tcp", port); err != nil {
		return errgo.Mask(err)
	}
	*openedPort = port
	return nil
}
