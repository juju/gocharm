package identityservice

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/testing/httpsuite"
	"net/http"
	"strings"
)

type UserPassSuite struct {
	httpsuite.HTTPSuite
}

var _ = Suite(&UserPassSuite{})

func makeUserPass(user, secret string) (identity *UserPass) {
	identity = NewUserPass()
	// Ensure that it conforms to the interface
	var _ IdentityService = identity
	if user != "" {
		identity.AddUser(user, secret, "tenant")
	}
	return
}

func (s *UserPassSuite) setupUserPass(user, secret string) {
	var identity *UserPass
	identity = makeUserPass(user, secret)
	identity.SetupHTTP(s.Mux)
	return
}

func (s *UserPassSuite) setupUserPassWithServices(user, secret string, services []Service) {
	var identity *UserPass
	identity = makeUserPass(user, secret)
	for _, service := range services {
		identity.AddService(service)
	}
	identity.SetupHTTP(s.Mux)
	return
}

var authTemplate = `{
    "auth": {
        "tenantName": "tenant-something", 
        "passwordCredentials": {
            "username": "%s", 
            "password": "%s"
        }
    }
}`

func userPassAuthRequest(URL, user, key string) (*http.Response, error) {
	client := &http.DefaultClient
	body := strings.NewReader(fmt.Sprintf(authTemplate, user, key))
	request, err := http.NewRequest("POST", URL+"/tokens", body)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

func CheckErrorResponse(c *C, r *http.Response, status int, msg string) {
	c.Check(r.StatusCode, Equals, status)
	c.Assert(r.Header.Get("Content-Type"), Equals, "application/json")
	body, err := ioutil.ReadAll(r.Body)
	c.Assert(err, IsNil)
	var errmsg ErrorWrapper
	err = json.Unmarshal(body, &errmsg)
	c.Assert(err, IsNil)
	c.Check(errmsg.Error.Code, Equals, status)
	c.Check(errmsg.Error.Title, Equals, http.StatusText(status))
	if msg != "" {
		c.Check(errmsg.Error.Message, Equals, msg)
	}
}

func (s *UserPassSuite) TestNotJSON(c *C) {
	// We do everything in userPassAuthRequest, except set the Content-Type
	s.setupUserPass("user", "secret")
	client := &http.DefaultClient
	body := strings.NewReader(fmt.Sprintf(authTemplate, "user", "secret"))
	request, err := http.NewRequest("POST", s.Server.URL+"/tokens", body)
	c.Assert(err, IsNil)
	res, err := client.Do(request)
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusBadRequest, notJSON)
}

func (s *UserPassSuite) TestBadJSON(c *C) {
	// We do everything in userPassAuthRequest, except set the Content-Type
	s.setupUserPass("user", "secret")
	res, err := userPassAuthRequest(s.Server.URL, "garbage\"in", "secret")
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusBadRequest, notJSON)
}

func (s *UserPassSuite) TestNoSuchUser(c *C) {
	s.setupUserPass("user", "secret")
	res, err := userPassAuthRequest(s.Server.URL, "not-user", "secret")
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusUnauthorized, notAuthorized)
}

func (s *UserPassSuite) TestBadPassword(c *C) {
	s.setupUserPass("user", "secret")
	res, err := userPassAuthRequest(s.Server.URL, "user", "not-secret")
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusUnauthorized, invalidUser)
}

func (s *UserPassSuite) TestValidAuthorization(c *C) {
	compute_url := "http://testing.invalid/compute"
	s.setupUserPassWithServices("user", "secret", []Service{
		{"nova", "compute", []Endpoint{
			{PublicURL: compute_url},
		}}})
	res, err := userPassAuthRequest(s.Server.URL, "user", "secret")
	defer res.Body.Close()
	c.Assert(err, IsNil)
	c.Check(res.StatusCode, Equals, http.StatusOK)
	c.Check(res.Header.Get("Content-Type"), Equals, "application/json")
	content, err := ioutil.ReadAll(res.Body)
	c.Assert(err, IsNil)
	var response AccessResponse
	err = json.Unmarshal(content, &response)
	c.Assert(err, IsNil)
	c.Check(response.Access.Token.Id, NotNil)
	novaURL := ""
	for _, service := range response.Access.ServiceCatalog {
		if service.Type == "compute" {
			novaURL = service.Endpoints[0].PublicURL
			break
		}
	}
	c.Assert(novaURL, Equals, compute_url)
}
