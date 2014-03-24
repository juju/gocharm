package swift_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/testing/httpsuite"
	"launchpad.net/goose/testservices/openstackservice"
)

func registerLocalTests() {
	Suite(&localLiveSuite{})
}

// localLiveSuite runs tests from LiveTests using a fake
// swift server that runs within the test process itself.
type localLiveSuite struct {
	LiveTests
	LiveTestsPublicContainer
	// The following attributes are for using testing doubles.
	httpsuite.HTTPSuite
	openstack *openstackservice.Openstack
}

func (s *localLiveSuite) SetUpSuite(c *C) {
	c.Logf("Using identity and swift service test doubles")
	s.HTTPSuite.SetUpSuite(c)
	// Set up an Openstack service.
	s.LiveTests.cred = &identity.Credentials{
		URL:        s.Server.URL,
		User:       "fred",
		Secrets:    "secret",
		Region:     "some region",
		TenantName: "tenant",
	}
	s.LiveTestsPublicContainer.cred = s.LiveTests.cred
	s.openstack = openstackservice.New(s.LiveTests.cred,
		identity.AuthUserPass)

	s.LiveTests.SetUpSuite(c)
	s.LiveTestsPublicContainer.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.LiveTestsPublicContainer.TearDownSuite(c)
	s.HTTPSuite.TearDownSuite(c)
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.openstack.SetupHTTP(s.Mux)
	s.LiveTests.SetUpTest(c)
	s.LiveTestsPublicContainer.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
	s.LiveTestsPublicContainer.TearDownTest(c)
	s.HTTPSuite.TearDownTest(c)
}

// Additional tests to be run against the service double only go here.
