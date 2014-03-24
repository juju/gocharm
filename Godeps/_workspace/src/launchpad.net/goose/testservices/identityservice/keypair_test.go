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

type KeyPairSuite struct {
	httpsuite.HTTPSuite
}

var _ = Suite(&KeyPairSuite{})

func makeKeyPair(user, secret string) (identity *KeyPair) {
	identity = NewKeyPair()
	// Ensure that it conforms to the interface
	var _ IdentityService = identity
	if user != "" {
		identity.AddUser(user, secret, "tenant")
	}
	return
}

func (s *KeyPairSuite) setupKeyPair(user, secret string) {
	var identity *KeyPair
	identity = makeKeyPair(user, secret)
	identity.SetupHTTP(s.Mux)
	return
}

func (s *KeyPairSuite) setupKeyPairWithServices(user, secret string, services []Service) {
	var identity *KeyPair
	identity = makeKeyPair(user, secret)
	for _, service := range services {
		identity.AddService(service)
	}
	identity.SetupHTTP(s.Mux)
	return
}

const authKeyPairTemplate = `{
    "auth": {
        "tenantName": "tenant-something",
        "apiAccessKeyCredentials": {
            "accessKey": "%s",
            "secretKey": "%s"
        }
    }
}`

func keyPairAuthRequest(URL, access, secret string) (*http.Response, error) {
	client := &http.DefaultClient
	body := strings.NewReader(fmt.Sprintf(authKeyPairTemplate, access, secret))
	request, err := http.NewRequest("POST", URL+"/tokens", body)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

func (s *KeyPairSuite) TestNotJSON(c *C) {
	// We do everything in keyPairAuthRequest, except set the Content-Type
	s.setupKeyPair("user", "secret")
	client := &http.DefaultClient
	body := strings.NewReader(fmt.Sprintf(authTemplate, "user", "secret"))
	request, err := http.NewRequest("POST", s.Server.URL+"/tokens", body)
	c.Assert(err, IsNil)
	res, err := client.Do(request)
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusBadRequest, notJSON)
}

func (s *KeyPairSuite) TestBadJSON(c *C) {
	// We do everything in keyPairAuthRequest, except set the Content-Type
	s.setupKeyPair("user", "secret")
	res, err := keyPairAuthRequest(s.Server.URL, `garbage"in`, "secret")
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusBadRequest, notJSON)
}

func (s *KeyPairSuite) TestNoSuchUser(c *C) {
	s.setupKeyPair("user", "secret")
	res, err := keyPairAuthRequest(s.Server.URL, "not-user", "secret")
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusUnauthorized, notAuthorized)
}

func (s *KeyPairSuite) TestBadPassword(c *C) {
	s.setupKeyPair("user", "secret")
	res, err := keyPairAuthRequest(s.Server.URL, "user", "not-secret")
	defer res.Body.Close()
	c.Assert(err, IsNil)
	CheckErrorResponse(c, res, http.StatusUnauthorized, invalidUser)
}

func (s *KeyPairSuite) TestValidAuthorization(c *C) {
	compute_url := "http://testing.invalid/compute"
	s.setupKeyPairWithServices("user", "secret", []Service{
		{"nova", "compute", []Endpoint{
			{PublicURL: compute_url},
		}}})
	res, err := keyPairAuthRequest(s.Server.URL, "user", "secret")
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
