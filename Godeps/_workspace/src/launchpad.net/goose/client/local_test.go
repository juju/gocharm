package client_test

import (
	"encoding/json"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/errors"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/swift"
	"launchpad.net/goose/testing/httpsuite"
	"launchpad.net/goose/testservices"
	"launchpad.net/goose/testservices/identityservice"
	"launchpad.net/goose/testservices/openstackservice"
	"net/url"
	"runtime"
	"sync"
	"time"
)

func registerLocalTests(authModes []identity.AuthMode) {
	for _, authMode := range authModes {
		Suite(&localLiveSuite{
			LiveTests: LiveTests{
				authMode: authMode,
			},
		})
	}
	Suite(&localHTTPSSuite{HTTPSuite: httpsuite.HTTPSuite{UseTLS: true}})
}

// localLiveSuite runs tests from LiveTests using a fake
// identity server that runs within the test process itself.
type localLiveSuite struct {
	LiveTests
	// The following attributes are for using testing doubles.
	httpsuite.HTTPSuite
	service testservices.HttpService
}

func (s *localLiveSuite) SetUpSuite(c *C) {
	c.Logf("Using identity service test double")
	s.HTTPSuite.SetUpSuite(c)
	s.cred = &identity.Credentials{
		URL:        s.Server.URL,
		User:       "fred",
		Secrets:    "secret",
		Region:     "zone1.some region",
		TenantName: "tenant",
	}
	switch s.authMode {
	default:
		panic("Invalid authentication method")
	case identity.AuthKeyPair:
		// The openstack test service sets up keypair authentication.
		s.service = openstackservice.New(s.cred, identity.AuthKeyPair)
		// Add an additional endpoint so region filtering can be properly tested.
		serviceDef := identityservice.Service{"nova", "compute", []identityservice.Endpoint{
			identityservice.Endpoint{PublicURL: "http://nova2", Region: "zone2.RegionOne"},
		}}
		s.service.(*openstackservice.Openstack).Identity.AddService(serviceDef)
	case identity.AuthUserPass:
		// The openstack test service sets up userpass authentication.
		s.service = openstackservice.New(s.cred, identity.AuthUserPass)
		// Add an additional endpoint so region filtering can be properly tested.
		serviceDef := identityservice.Service{"nova", "compute", []identityservice.Endpoint{
			identityservice.Endpoint{PublicURL: "http://nova2", Region: "zone2.RegionOne"},
		}}
		s.service.(*openstackservice.Openstack).Identity.AddService(serviceDef)

	case identity.AuthLegacy:
		legacy := identityservice.NewLegacy()
		legacy.AddUser(s.cred.User, s.cred.Secrets, s.cred.TenantName)
		legacy.SetManagementURL("http://management.test.invalid/url")
		s.service = legacy
	}
	s.LiveTests.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.HTTPSuite.TearDownSuite(c)
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.service.SetupHTTP(s.Mux)
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
	s.HTTPSuite.TearDownTest(c)
}

// Additional tests to be run against the service double only go here.

func (s *localLiveSuite) TestInvalidRegion(c *C) {
	if s.authMode == identity.AuthLegacy {
		c.Skip("legacy authentication doesn't use regions")
	}
	creds := &identity.Credentials{
		User:    "fred",
		URL:     s.Server.URL,
		Secrets: "secret",
		Region:  "invalid",
	}
	cl := client.NewClient(creds, s.authMode, nil)
	err := cl.Authenticate()
	c.Assert(err.Error(), Matches, "(.|\n)*invalid region(.|\n)*")
}

// Test service lookup with inexact region matching.
func (s *localLiveSuite) TestInexactRegionMatch(c *C) {
	if s.authMode == identity.AuthLegacy {
		c.Skip("legacy authentication doesn't use regions")
	}
	cl := client.NewClient(s.cred, s.authMode, nil)
	err := cl.Authenticate()
	serviceURL, err := cl.MakeServiceURL("compute", []string{})
	c.Assert(err, IsNil)
	_, err = url.Parse(serviceURL)
	c.Assert(err, IsNil)
	serviceURL, err = cl.MakeServiceURL("object-store", []string{})
	c.Assert(err, IsNil)
	_, err = url.Parse(serviceURL)
	c.Assert(err, IsNil)
}

type fakeAuthenticator struct {
	mu        sync.Mutex
	nrCallers int
	// authStart is used as a gate to signal the fake authenticator that it can start.
	authStart chan struct{}
}

func newAuthenticator(bufsize int) *fakeAuthenticator {
	return &fakeAuthenticator{
		authStart: make(chan struct{}, bufsize),
	}
}

func (auth *fakeAuthenticator) Auth(creds *identity.Credentials) (*identity.AuthDetails, error) {
	auth.mu.Lock()
	auth.nrCallers++
	auth.mu.Unlock()
	// Wait till the test says the authenticator can proceed.
	<-auth.authStart
	runtime.Gosched()
	defer func() {
		auth.mu.Lock()
		auth.nrCallers--
		auth.mu.Unlock()
	}()
	auth.mu.Lock()
	tooManyCallers := auth.nrCallers > 1
	auth.mu.Unlock()
	if tooManyCallers {
		return nil, fmt.Errorf("Too many callers of Auth function")
	}
	URLs := make(map[string]identity.ServiceURLs)
	endpoints := make(map[string]string)
	endpoints["compute"] = "http://localhost"
	URLs[creds.Region] = endpoints
	return &identity.AuthDetails{
		Token:             "token",
		TenantId:          "tenant",
		UserId:            "1",
		RegionServiceURLs: URLs,
	}, nil
}

func (s *localLiveSuite) TestAuthenticationTimeout(c *C) {
	cl := client.NewClient(s.cred, s.authMode, nil)
	defer client.SetAuthenticationTimeout(1 * time.Millisecond)()
	auth := newAuthenticator(0)
	client.SetAuthenticator(cl, auth)

	var err error
	err = cl.Authenticate()
	// Wake up the authenticator after we have timed out.
	auth.authStart <- struct{}{}
	c.Assert(errors.IsTimeout(err), Equals, true)
}

func (s *localLiveSuite) assertAuthenticationSuccess(c *C) client.Client {
	cl := client.NewClient(s.cred, s.authMode, nil)
	cl.SetRequiredServiceTypes([]string{"compute"})
	defer client.SetAuthenticationTimeout(1 * time.Millisecond)()
	auth := newAuthenticator(1)
	client.SetAuthenticator(cl, auth)

	// Signal that the authenticator can proceed immediately.
	auth.authStart <- struct{}{}
	err := cl.Authenticate()
	c.Assert(err, IsNil)
	// It completed with no error but check it also ran correctly.
	c.Assert(cl.IsAuthenticated(), Equals, true)
	return cl
}

func (s *localLiveSuite) TestAuthenticationSuccess(c *C) {
	cl := s.assertAuthenticationSuccess(c)
	URL, err := cl.MakeServiceURL("compute", nil)
	c.Assert(err, IsNil)
	c.Assert(URL, Equals, "http://localhost")
}

func (s *localLiveSuite) TestMakeServiceURL(c *C) {
	cl := s.assertAuthenticationSuccess(c)
	URL, err := cl.MakeServiceURL("compute", []string{"foo"})
	c.Assert(err, IsNil)
	c.Assert(URL, Equals, "http://localhost/foo")
}

func (s *localLiveSuite) TestMakeServiceURLRetainsTrailingSlash(c *C) {
	cl := s.assertAuthenticationSuccess(c)
	URL, err := cl.MakeServiceURL("compute", []string{"foo", "bar/"})
	c.Assert(err, IsNil)
	c.Assert(URL, Equals, "http://localhost/foo/bar/")
}

func checkAuthentication(cl client.AuthenticatingClient) error {
	err := cl.Authenticate()
	if err != nil {
		return err
	}
	URL, err := cl.MakeServiceURL("compute", nil)
	if err != nil {
		return err
	}
	if URL != "http://localhost" {
		return fmt.Errorf("Unexpected URL: %s", URL)
	}
	return nil
}

func (s *localLiveSuite) TestAuthenticationForbidsMultipleCallers(c *C) {
	if s.authMode == identity.AuthLegacy {
		c.Skip("legacy authentication")
	}
	cl := client.NewClient(s.cred, s.authMode, nil)
	cl.SetRequiredServiceTypes([]string{"compute"})
	auth := newAuthenticator(2)
	client.SetAuthenticator(cl, auth)

	// Signal that the authenticator can proceed immediately.
	auth.authStart <- struct{}{}
	auth.authStart <- struct{}{}
	var allDone sync.WaitGroup
	allDone.Add(2)
	var err1, err2 error
	go func() {
		err1 = checkAuthentication(cl)
		allDone.Done()
	}()
	go func() {
		err2 = checkAuthentication(cl)
		allDone.Done()
	}()
	allDone.Wait()
	c.Assert(err1, IsNil)
	c.Assert(err2, IsNil)
}

type configurableAuth struct {
	regionsURLs map[string]identity.ServiceURLs
}

func NewConfigurableAuth(regionsURLData string) *configurableAuth {
	auth := &configurableAuth{}
	err := json.Unmarshal([]byte(regionsURLData), &auth.regionsURLs)
	if err != nil {
		panic(err)
	}
	return auth
}

func (auth *configurableAuth) Auth(creds *identity.Credentials) (*identity.AuthDetails, error) {
	return &identity.AuthDetails{
		Token:             "token",
		TenantId:          "tenant",
		UserId:            "1",
		RegionServiceURLs: auth.regionsURLs,
	}, nil
}

type authRegionTest struct {
	region        string
	regionURLInfo string
	errorMsg      string
}

var missingEndpointMsgf = "(.|\n)*the configured region %q does not allow access to all required services, namely: %s(.|\n)*access to these services is missing: %s"
var missingEndpointSuggestRegionMsgf = "(.|\n)*the configured region %q does not allow access to all required services, namely: %s(.|\n)*access to these services is missing: %s(.|\n)*one of these regions may be suitable instead: %s"
var invalidRegionMsgf = "(.|\n)*invalid region %q"

var authRegionTests = []authRegionTest{
	authRegionTest{
		"a.region.1",
		`{"a.region.1":{"compute":"http://foo"}}`,
		fmt.Sprintf(missingEndpointMsgf, "a.region.1", "compute, object-store", "object-store"),
	},
	authRegionTest{
		"b.region.1",
		`{"a.region.1":{"compute":"http://foo"}}`,
		fmt.Sprintf(invalidRegionMsgf, "b.region.1"),
	},
	authRegionTest{
		"b.region.1",
		`{"a.region.1":{"compute":"http://foo"}, "region.1":{"object-store":"http://foobar"}}`,
		fmt.Sprintf(missingEndpointSuggestRegionMsgf, "b.region.1", "compute, object-store", "compute", "a.region.1"),
	},
	authRegionTest{
		"region.1",
		`{"a.region.1":{"compute":"http://foo"}, "region.1":{"object-store":"http://foobar"}}`,
		fmt.Sprintf(missingEndpointSuggestRegionMsgf, "region.1", "compute, object-store", "compute", "a.region.1"),
	},
}

func (s *localLiveSuite) TestNonAccessibleServiceType(c *C) {
	if s.authMode == identity.AuthLegacy {
		c.Skip("legacy authentication")
	}
	for _, at := range authRegionTests {
		s.cred.Region = at.region
		cl := client.NewClient(s.cred, s.authMode, nil)
		auth := NewConfigurableAuth(at.regionURLInfo)
		client.SetAuthenticator(cl, auth)
		err := cl.Authenticate()
		c.Assert(err, ErrorMatches, at.errorMsg)
	}
}

type localHTTPSSuite struct {
	// The following attributes are for using testing doubles.
	httpsuite.HTTPSuite
	service testservices.HttpService
	cred    *identity.Credentials
}

func (s *localHTTPSSuite) SetUpSuite(c *C) {
	c.Logf("Using identity service test double")
	s.HTTPSuite.SetUpSuite(c)
	c.Assert(s.Server.URL[:8], Equals, "https://")
	s.cred = &identity.Credentials{
		URL:        s.Server.URL,
		User:       "fred",
		Secrets:    "secret",
		Region:     "zone1.some region",
		TenantName: "tenant",
	}
	// The openstack test service sets up userpass authentication.
	s.service = openstackservice.New(s.cred, identity.AuthUserPass)
	// Add an additional endpoint so region filtering can be properly tested.
	serviceDef := identityservice.Service{"nova", "compute", []identityservice.Endpoint{
		identityservice.Endpoint{PublicURL: "https://nova2", Region: "zone2.RegionOne"},
	}}
	s.service.(*openstackservice.Openstack).Identity.AddService(serviceDef)
}

func (s *localHTTPSSuite) TearDownSuite(c *C) {
	s.HTTPSuite.TearDownSuite(c)
}

func (s *localHTTPSSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.service.SetupHTTP(s.Mux)
}

func (s *localHTTPSSuite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
}

func (s *localHTTPSSuite) TestDefaultClientRefusesSelfSigned(c *C) {
	cl := client.NewClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
}

func (s *localHTTPSSuite) TestNonValidatingClientAcceptsSelfSigned(c *C) {
	cl := client.NewNonValidatingClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, IsNil)

	// Requests into this client should be https:// URLs
	swiftURL, err := cl.MakeServiceURL("object-store", []string{"test_container"})
	c.Assert(err, IsNil)
	c.Assert(swiftURL[:8], Equals, "https://")
	// We use swiftClient.CreateContainer to test a Binary request
	swiftClient := swift.New(cl)
	c.Assert(swiftClient.CreateContainer("test_container", swift.Private), IsNil)

	// And we use List to test the JsonRequest
	contents, err := swiftClient.List("test_container", "", "", "", 0)
	c.Assert(err, IsNil)
	c.Check(contents, DeepEquals, []swift.ContainerContents{})
}

func (s *localHTTPSSuite) setupPublicContainer(c *C) string {
	// First set up a container that can be read publically
	authClient := client.NewNonValidatingClient(s.cred, identity.AuthUserPass, nil)
	authSwift := swift.New(authClient)
	err := authSwift.CreateContainer("test_container", swift.PublicRead)
	c.Assert(err, IsNil)

	baseURL, err := authClient.MakeServiceURL("object-store", nil)
	c.Assert(err, IsNil)
	c.Assert(baseURL[:8], Equals, "https://")
	return baseURL
}

func (s *localHTTPSSuite) TestDefaultPublicClientRefusesSelfSigned(c *C) {
	baseURL := s.setupPublicContainer(c)
	swiftClient := swift.New(client.NewPublicClient(baseURL, nil))
	contents, err := swiftClient.List("test_container", "", "", "", 0)
	c.Assert(err, ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
	c.Assert(contents, DeepEquals, []swift.ContainerContents(nil))
}

func (s *localHTTPSSuite) TestNonValidatingPublicClientAcceptsSelfSigned(c *C) {
	baseURL := s.setupPublicContainer(c)
	swiftClient := swift.New(client.NewNonValidatingPublicClient(baseURL, nil))
	contents, err := swiftClient.List("test_container", "", "", "", 0)
	c.Assert(err, IsNil)
	c.Assert(contents, DeepEquals, []swift.ContainerContents{})
}
