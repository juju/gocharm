package identity_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/testservices/openstackservice"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
)

func registerLocalTests() {
	Suite(&localLiveSuite{})
}

// localLiveSuite runs tests from LiveTests using a fake
// nova server that runs within the test process itself.
type localLiveSuite struct {
	LiveTests
	// The following attributes are for using testing doubles.
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
}

func (s *localLiveSuite) SetUpSuite(c *C) {
	c.Logf("Using identity and nova service test doubles")

	// Set up the HTTP server.
	s.Server = httptest.NewServer(nil)
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux

	// Set up an Openstack service.
	s.cred = &identity.Credentials{
		URL:        s.Server.URL,
		User:       "fred",
		Secrets:    "secret",
		Region:     "zone1.some region",
		TenantName: "tenant",
	}
	openstack := openstackservice.New(s.cred, identity.AuthUserPass)
	openstack.SetupHTTP(s.Mux)

	s.LiveTests.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
}

// Additional tests to be run against the service double only go here.

func (s *localLiveSuite) TestProductStreamsEndpoint(c *C) {
	err := s.client.Authenticate()
	c.Assert(err, IsNil)
	serviceURL, err := s.client.MakeServiceURL("product-streams", nil)
	c.Assert(err, IsNil)
	_, err = url.Parse(serviceURL)
	c.Assert(err, IsNil)
	c.Assert(strings.HasSuffix(serviceURL, "/imagemetadata"), Equals, true)
}

func (s *localLiveSuite) TestJujuToolsEndpoint(c *C) {
	err := s.client.Authenticate()
	c.Assert(err, IsNil)
	serviceURL, err := s.client.MakeServiceURL("juju-tools", nil)
	c.Assert(err, IsNil)
	_, err = url.Parse(serviceURL)
	c.Assert(err, IsNil)
}
