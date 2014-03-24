package identityservice

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/testing/httpsuite"
	"net/http"
)

type LegacySuite struct {
	httpsuite.HTTPSuite
}

var _ = Suite(&LegacySuite{})

func (s *LegacySuite) setupLegacy(user, secret string) (token, managementURL string) {
	managementURL = s.Server.URL
	identity := NewLegacy()
	// Ensure that it conforms to the interface
	var _ IdentityService = identity
	identity.SetManagementURL(managementURL)
	identity.SetupHTTP(s.Mux)
	if user != "" {
		userInfo := identity.AddUser(user, secret, "tenant")
		token = userInfo.Token
	}
	return
}

func LegacyAuthRequest(URL, user, key string) (*http.Response, error) {
	client := &http.DefaultClient
	request, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		return nil, err
	}
	if user != "" {
		request.Header.Set("X-Auth-User", user)
	}
	if key != "" {
		request.Header.Set("X-Auth-Key", key)
	}
	return client.Do(request)
}

func AssertUnauthorized(c *C, response *http.Response) {
	content, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	response.Body.Close()
	c.Check(response.Header.Get("X-Auth-Token"), Equals, "")
	c.Check(response.Header.Get("X-Server-Management-Url"), Equals, "")
	c.Check(string(content), Equals, "")
	c.Check(response.StatusCode, Equals, http.StatusUnauthorized)
}

func (s *LegacySuite) TestLegacyFailedAuth(c *C) {
	s.setupLegacy("", "")
	// No headers set for Authentication
	response, err := LegacyAuthRequest(s.Server.URL, "", "")
	c.Assert(err, IsNil)
	AssertUnauthorized(c, response)
}

func (s *LegacySuite) TestLegacyFailedOnlyUser(c *C) {
	s.setupLegacy("", "")
	// Missing secret key
	response, err := LegacyAuthRequest(s.Server.URL, "user", "")
	c.Assert(err, IsNil)
	AssertUnauthorized(c, response)
}

func (s *LegacySuite) TestLegacyNoSuchUser(c *C) {
	s.setupLegacy("user", "key")
	// No user matching the username
	response, err := LegacyAuthRequest(s.Server.URL, "notuser", "key")
	c.Assert(err, IsNil)
	AssertUnauthorized(c, response)
}

func (s *LegacySuite) TestLegacyInvalidAuth(c *C) {
	s.setupLegacy("user", "secret-key")
	// Wrong key
	response, err := LegacyAuthRequest(s.Server.URL, "user", "bad-key")
	c.Assert(err, IsNil)
	AssertUnauthorized(c, response)
}

func (s *LegacySuite) TestLegacyAuth(c *C) {
	token, serverURL := s.setupLegacy("user", "secret-key")
	response, err := LegacyAuthRequest(s.Server.URL, "user", "secret-key")
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	c.Check(response.Header.Get("X-Auth-Token"), Equals, token)
	c.Check(response.Header.Get("X-Server-Management-Url"), Equals, serverURL+"/compute")
	c.Check(response.Header.Get("X-Storage-Url"), Equals, serverURL+"/object-store")
	c.Check(string(content), Equals, "")
	c.Check(response.StatusCode, Equals, http.StatusNoContent)
}
