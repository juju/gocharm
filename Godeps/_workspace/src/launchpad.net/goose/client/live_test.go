package client_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/identity"
)

func registerOpenStackTests(cred *identity.Credentials, authModes []identity.AuthMode) {
	for _, authMode := range authModes {
		Suite(&LiveTests{
			cred:     cred,
			authMode: authMode,
		})
	}
}

type LiveTests struct {
	cred     *identity.Credentials
	authMode identity.AuthMode
}

func (s *LiveTests) SetUpSuite(c *C) {
	c.Logf("Running tests with authentication method %v", s.authMode)
}

func (s *LiveTests) TearDownSuite(c *C) {
	// noop, called by local test suite.
}

func (s *LiveTests) SetUpTest(c *C) {
	// noop, called by local test suite.
}

func (s *LiveTests) TearDownTest(c *C) {
	// noop, called by local test suite.
}

func (s *LiveTests) TestAuthenticateFail(c *C) {
	cred := *s.cred
	cred.User = "fred"
	cred.Secrets = "broken"
	cred.Region = ""
	osclient := client.NewClient(&cred, s.authMode, nil)
	c.Assert(osclient.IsAuthenticated(), Equals, false)
	err := osclient.Authenticate()
	c.Assert(err, ErrorMatches, "authentication failed(\n|.)*")
}

func (s *LiveTests) TestAuthenticate(c *C) {
	cl := client.NewClient(s.cred, s.authMode, nil)
	err := cl.Authenticate()
	c.Assert(err, IsNil)
	c.Assert(cl.IsAuthenticated(), Equals, true)

	// Check service endpoints are discovered
	url, err := cl.MakeServiceURL("compute", nil)
	c.Check(err, IsNil)
	c.Check(url, NotNil)
	url, err = cl.MakeServiceURL("object-store", nil)
	c.Check(err, IsNil)
	c.Check(url, NotNil)
}
