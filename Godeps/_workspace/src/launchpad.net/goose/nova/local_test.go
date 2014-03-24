package nova_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/errors"
	goosehttp "launchpad.net/goose/http"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/testservices"
	"launchpad.net/goose/testservices/hook"
	"launchpad.net/goose/testservices/identityservice"
	"launchpad.net/goose/testservices/openstackservice"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
)

func registerLocalTests() {
	// Test using numeric ids.
	Suite(&localLiveSuite{
		useNumericIds: true,
	})
	// Test using string ids.
	Suite(&localLiveSuite{
		useNumericIds: false,
	})
}

// localLiveSuite runs tests from LiveTests using a fake
// nova server that runs within the test process itself.
type localLiveSuite struct {
	LiveTests
	useNumericIds bool
	// The following attributes are for using testing doubles.
	Server                *httptest.Server
	Mux                   *http.ServeMux
	oldHandler            http.Handler
	openstack             *openstackservice.Openstack
	retryErrorCount       int  // The current retry error count.
	retryErrorCountToSend int  // The number of retry errors to send.
	noMoreIPs             bool // If true, addFloatingIP will return ErrNoMoreFloatingIPs
	ipLimitExceeded       bool // If true, addFloatingIP will return ErrIPLimitExceeded
	badTokens             int  // If > 0, authHook will set an invalid token in the AccessResponse data.
}

func (s *localLiveSuite) SetUpSuite(c *C) {
	var idInfo string
	if s.useNumericIds {
		idInfo = "with numeric ids"
	} else {
		idInfo = "with string ids"
	}
	c.Logf("Using identity and nova service test doubles %s", idInfo)
	nova.UseNumericIds(s.useNumericIds)

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
		Region:     "some region",
		TenantName: "tenant",
	}
	s.openstack = openstackservice.New(s.cred, identity.AuthUserPass)
	s.openstack.SetupHTTP(s.Mux)

	s.testFlavor = "m1.small"
	s.testImageId = "1"
	s.LiveTests.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.retryErrorCount = 0
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
}

// Additional tests to be run against the service double only go here.

func (s *localLiveSuite) retryLimitHook(sc hook.ServiceControl) hook.ControlProcessor {
	return func(sc hook.ServiceControl, args ...interface{}) error {
		sendError := s.retryErrorCount < s.retryErrorCountToSend
		if sendError {
			s.retryErrorCount++
			return testservices.RateLimitExceededError
		}
		return nil
	}
}

func (s *localLiveSuite) setupClient(c *C, logger *log.Logger) *nova.Client {
	client := client.NewClient(s.cred, identity.AuthUserPass, logger)
	return nova.New(client)
}

func (s *localLiveSuite) setupRetryErrorTest(c *C, logger *log.Logger) (*nova.Client, *nova.SecurityGroup) {
	novaClient := s.setupClient(c, logger)
	// Delete the artifact if it already exists.
	testGroup, err := novaClient.SecurityGroupByName("test_group")
	if err != nil {
		c.Assert(errors.IsNotFound(err), Equals, true)
	} else {
		novaClient.DeleteSecurityGroup(testGroup.Id)
		c.Assert(err, IsNil)
	}
	testGroup, err = novaClient.CreateSecurityGroup("test_group", "test")
	c.Assert(err, IsNil)
	return novaClient, testGroup
}

// TestRateLimitRetry checks that when we make too many requests and receive a Retry-After response, the retry
// occurs and the request ultimately succeeds.
func (s *localLiveSuite) TestRateLimitRetry(c *C) {
	// Capture the logged output so we can check for retry messages.
	var logout bytes.Buffer
	logger := log.New(&logout, "", log.LstdFlags)
	novaClient, testGroup := s.setupRetryErrorTest(c, logger)
	s.retryErrorCountToSend = goosehttp.MaxSendAttempts - 1
	s.openstack.Nova.RegisterControlPoint("removeSecurityGroup", s.retryLimitHook(s.openstack.Nova))
	defer s.openstack.Nova.RegisterControlPoint("removeSecurityGroup", nil)
	err := novaClient.DeleteSecurityGroup(testGroup.Id)
	c.Assert(err, IsNil)
	// Ensure we got at least one retry message.
	output := logout.String()
	c.Assert(strings.Contains(output, "Too many requests, retrying in"), Equals, true)
}

// TestRateLimitRetryExceeded checks that an error is raised if too many retry responses are received from the server.
func (s *localLiveSuite) TestRateLimitRetryExceeded(c *C) {
	novaClient, testGroup := s.setupRetryErrorTest(c, nil)
	s.retryErrorCountToSend = goosehttp.MaxSendAttempts
	s.openstack.Nova.RegisterControlPoint("removeSecurityGroup", s.retryLimitHook(s.openstack.Nova))
	defer s.openstack.Nova.RegisterControlPoint("removeSecurityGroup", nil)
	err := novaClient.DeleteSecurityGroup(testGroup.Id)
	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Matches, "(.|\n)*Maximum number of attempts.*")
}

func (s *localLiveSuite) addFloatingIPHook(sc hook.ServiceControl) hook.ControlProcessor {
	return func(sc hook.ServiceControl, args ...interface{}) error {
		if s.noMoreIPs {
			return testservices.NoMoreFloatingIPs
		} else if s.ipLimitExceeded {
			return testservices.IPLimitExceeded
		}
		return nil
	}
}

func (s *localLiveSuite) TestAddFloatingIPErrors(c *C) {
	novaClient := s.setupClient(c, nil)
	fips, err := novaClient.ListFloatingIPs()
	c.Assert(err, IsNil)
	c.Assert(fips, HasLen, 0)
	cleanup := s.openstack.Nova.RegisterControlPoint("addFloatingIP", s.addFloatingIPHook(s.openstack.Nova))
	defer cleanup()
	s.noMoreIPs = true
	fip, err := novaClient.AllocateFloatingIP()
	c.Assert(err, ErrorMatches, "(.|\n)*Zero floating ips available.*")
	c.Assert(fip, IsNil)
	s.noMoreIPs = false
	s.ipLimitExceeded = true
	fip, err = novaClient.AllocateFloatingIP()
	c.Assert(err, ErrorMatches, "(.|\n)*Maximum number of floating ips exceeded.*")
	c.Assert(fip, IsNil)
	s.ipLimitExceeded = false
	fip, err = novaClient.AllocateFloatingIP()
	c.Assert(err, IsNil)
	c.Assert(fip.IP, Not(Equals), "")
}

func (s *localLiveSuite) authHook(sc hook.ServiceControl) hook.ControlProcessor {
	return func(sc hook.ServiceControl, args ...interface{}) error {
		res := args[0].(*identityservice.AccessResponse)
		if s.badTokens > 0 {
			res.Access.Token.Id = "xxx"
			s.badTokens--
		}
		return nil
	}
}

func (s *localLiveSuite) TestReauthenticate(c *C) {
	novaClient := s.setupClient(c, nil)
	up := s.openstack.Identity.(*identityservice.UserPass)
	cleanup := up.RegisterControlPoint("authorisation", s.authHook(up))
	defer cleanup()

	// An invalid token is returned after the first authentication step, resulting in the ListServers call
	// returning a 401. Subsequent authentication calls return the correct token so the auth retry does it's job.
	s.badTokens = 1
	_, err := novaClient.ListServers(nil)
	c.Assert(err, IsNil)
}

func (s *localLiveSuite) TestReauthenticateFailure(c *C) {
	novaClient := s.setupClient(c, nil)
	up := s.openstack.Identity.(*identityservice.UserPass)
	cleanup := up.RegisterControlPoint("authorisation", s.authHook(up))
	defer cleanup()

	// If the re-authentication fails, ensure an Unauthorised error is returned.
	s.badTokens = 2
	_, err := novaClient.ListServers(nil)
	c.Assert(errors.IsUnauthorised(err), Equals, true)
}
