// Nova double testing service - HTTP API tests

package novaservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/testing/httpsuite"
	"launchpad.net/goose/testservices/identityservice"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type NovaHTTPSuite struct {
	httpsuite.HTTPSuite
	service *Nova
	token   string
}

var _ = Suite(&NovaHTTPSuite{})

type NovaHTTPSSuite struct {
	httpsuite.HTTPSuite
	service *Nova
	token   string
}

var _ = Suite(&NovaHTTPSSuite{HTTPSuite: httpsuite.HTTPSuite{UseTLS: true}})

func (s *NovaHTTPSuite) SetUpSuite(c *C) {
	s.HTTPSuite.SetUpSuite(c)
	identityDouble := identityservice.NewUserPass()
	userInfo := identityDouble.AddUser("fred", "secret", "tenant")
	s.token = userInfo.Token
	s.service = New(s.Server.URL, versionPath, userInfo.TenantId, region, identityDouble)
}

func (s *NovaHTTPSuite) TearDownSuite(c *C) {
	s.HTTPSuite.TearDownSuite(c)
}

func (s *NovaHTTPSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.service.SetupHTTP(s.Mux)
}

func (s *NovaHTTPSuite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
}

// assertJSON asserts the passed http.Response's body can be
// unmarshalled into the given expected object, populating it with the
// successfully parsed data.
func assertJSON(c *C, resp *http.Response, expected interface{}) {
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	err = json.Unmarshal(body, &expected)
	c.Assert(err, IsNil)
}

// assertBody asserts the passed http.Response's body matches the
// expected response, replacing any variables in the expected body.
func assertBody(c *C, resp *http.Response, expected *errorResponse) {
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	expBody := expected.requestBody(resp.Request)
	// cast to string for easier asserts debugging
	c.Assert(string(body), Equals, string(expBody))
}

// sendRequest constructs an HTTP request from the parameters and
// sends it, returning the response or an error.
func (s *NovaHTTPSuite) sendRequest(method, url string, body []byte, headers http.Header) (*http.Response, error) {
	if !strings.HasPrefix(url, "http") {
		url = "http://" + s.service.Hostname + strings.TrimLeft(url, "/")
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for header, values := range headers {
		for _, value := range values {
			req.Header.Add(header, value)
		}
	}
	// workaround for https://code.google.com/p/go/issues/detail?id=4454
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return http.DefaultClient.Do(req)
}

// authRequest is a shortcut for sending requests with pre-set token
// header and correct version prefix and tenant ID in the URL.
func (s *NovaHTTPSuite) authRequest(method, path string, body []byte, headers http.Header) (*http.Response, error) {
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set(authToken, s.token)
	url := s.service.endpointURL(true, path)
	return s.sendRequest(method, url, body, headers)
}

// jsonRequest serializes the passed body object to JSON and sends a
// the request with authRequest().
func (s *NovaHTTPSuite) jsonRequest(method, path string, body interface{}, headers http.Header) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return s.authRequest(method, path, jsonBody, headers)
}

// setHeader creates http.Header map, sets the given header, and
// returns the map.
func setHeader(header, value string) http.Header {
	h := make(http.Header)
	h.Set(header, value)
	return h
}

// SimpleTest defines a simple request without a body and expected response.
type SimpleTest struct {
	unauth  bool
	method  string
	url     string
	headers http.Header
	expect  *errorResponse
}

func (s *NovaHTTPSuite) simpleTests() []SimpleTest {
	var simpleTests = []SimpleTest{
		{
			unauth:  true,
			method:  "GET",
			url:     "/any",
			headers: make(http.Header),
			expect:  errUnauthorized,
		},
		{
			unauth:  true,
			method:  "POST",
			url:     "/any",
			headers: setHeader(authToken, "phony"),
			expect:  errUnauthorized,
		},
		{
			unauth:  true,
			method:  "GET",
			url:     "/",
			headers: setHeader(authToken, s.token),
			expect:  errNoVersion,
		},
		{
			unauth:  true,
			method:  "GET",
			url:     "/any",
			headers: setHeader(authToken, s.token),
			expect:  errMultipleChoices,
		},
		{
			unauth:  true,
			method:  "POST",
			url:     "/any/unknown/one",
			headers: setHeader(authToken, s.token),
			expect:  errMultipleChoices,
		},
		{
			method: "POST",
			url:    "/any/unknown/one",
			expect: errNotFound,
		},
		{
			unauth:  true,
			method:  "GET",
			url:     versionPath + "/phony_token",
			headers: setHeader(authToken, s.token),
			expect:  errBadRequest,
		},
		{
			method: "GET",
			url:    "/flavors/",
			expect: errNotFound,
		},
		{
			method: "GET",
			url:    "/flavors/invalid",
			expect: errNotFound,
		},
		{
			method: "POST",
			url:    "/flavors",
			expect: errBadRequest2,
		},
		{
			method: "POST",
			url:    "/flavors/invalid",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/flavors",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/flavors/invalid",
			expect: errNotFoundJSON,
		},
		{
			method: "DELETE",
			url:    "/flavors",
			expect: errNotFound,
		},
		{
			method: "DELETE",
			url:    "/flavors/invalid",
			expect: errForbidden,
		},
		{
			method: "GET",
			url:    "/flavors/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "POST",
			url:    "/flavors/detail",
			expect: errNotFound,
		},
		{
			method: "POST",
			url:    "/flavors/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/flavors/detail",
			expect: errNotFoundJSON,
		},
		{
			method: "PUT",
			url:    "/flavors/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "DELETE",
			url:    "/flavors/detail",
			expect: errForbidden,
		},
		{
			method: "DELETE",
			url:    "/flavors/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "GET",
			url:    "/servers/invalid",
			expect: errNotFoundJSON,
		},
		{
			method: "POST",
			url:    "/servers",
			expect: errBadRequest2,
		},
		{
			method: "POST",
			url:    "/servers/invalid",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/servers",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/servers/invalid",
			expect: errBadRequest2,
		},
		{
			method: "DELETE",
			url:    "/servers",
			expect: errNotFound,
		},
		{
			method: "DELETE",
			url:    "/servers/invalid",
			expect: errNotFoundJSON,
		},
		{
			method: "GET",
			url:    "/servers/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "POST",
			url:    "/servers/detail",
			expect: errNotFound,
		},
		{
			method: "POST",
			url:    "/servers/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/servers/detail",
			expect: errBadRequest2,
		},
		{
			method: "PUT",
			url:    "/servers/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "DELETE",
			url:    "/servers/detail",
			expect: errNotFoundJSON,
		},
		{
			method: "DELETE",
			url:    "/servers/detail/invalid",
			expect: errNotFound,
		},
		{
			method: "GET",
			url:    "/os-security-groups/42",
			expect: errNotFoundJSONSG,
		},
		{
			method: "POST",
			url:    "/os-security-groups",
			expect: errBadRequest2,
		},
		{
			method: "POST",
			url:    "/os-security-groups/invalid",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/os-security-groups",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/os-security-groups/invalid",
			expect: errNotFoundJSON,
		},
		{
			method: "DELETE",
			url:    "/os-security-groups",
			expect: errNotFound,
		},
		{
			method: "DELETE",
			url:    "/os-security-groups/42",
			expect: errNotFoundJSONSG,
		},
		{
			method: "GET",
			url:    "/os-security-group-rules",
			expect: errNotFoundJSON,
		},
		{
			method: "GET",
			url:    "/os-security-group-rules/invalid",
			expect: errNotFoundJSON,
		},
		{
			method: "GET",
			url:    "/os-security-group-rules/42",
			expect: errNotFoundJSON,
		},
		{
			method: "POST",
			url:    "/os-security-group-rules",
			expect: errBadRequest2,
		},
		{
			method: "POST",
			url:    "/os-security-group-rules/invalid",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/os-security-group-rules",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/os-security-group-rules/invalid",
			expect: errNotFoundJSON,
		},
		{
			method: "DELETE",
			url:    "/os-security-group-rules",
			expect: errNotFound,
		},
		{
			method: "DELETE",
			url:    "/os-security-group-rules/42",
			expect: errNotFoundJSONSGR,
		},
		{
			method: "GET",
			url:    "/os-floating-ips/42",
			expect: errNotFoundJSON,
		},
		{
			method: "POST",
			url:    "/os-floating-ips/invalid",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/os-floating-ips",
			expect: errNotFound,
		},
		{
			method: "PUT",
			url:    "/os-floating-ips/invalid",
			expect: errNotFoundJSON,
		},
		{
			method: "DELETE",
			url:    "/os-floating-ips",
			expect: errNotFound,
		},
		{
			method: "DELETE",
			url:    "/os-floating-ips/invalid",
			expect: errNotFoundJSON,
		},
	}
	return simpleTests
}

func (s *NovaHTTPSuite) TestSimpleRequestTests(c *C) {
	simpleTests := s.simpleTests()
	for i, t := range simpleTests {
		c.Logf("#%d. %s %s -> %d", i, t.method, t.url, t.expect.code)
		if t.headers == nil {
			t.headers = make(http.Header)
			t.headers.Set(authToken, s.token)
		}
		var (
			resp *http.Response
			err  error
		)
		if t.unauth {
			resp, err = s.sendRequest(t.method, t.url, nil, t.headers)
		} else {
			resp, err = s.authRequest(t.method, t.url, nil, t.headers)
		}
		c.Assert(err, IsNil)
		c.Assert(resp.StatusCode, Equals, t.expect.code)
		assertBody(c, resp, t.expect)
	}
	fmt.Printf("total: %d\n", len(simpleTests))
}

func (s *NovaHTTPSuite) TestGetFlavors(c *C) {
	// The test service has 3 default flavours.
	var expected struct {
		Flavors []nova.Entity
	}
	resp, err := s.authRequest("GET", "/flavors", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Flavors, HasLen, 3)
	entities := s.service.allFlavorsAsEntities()
	c.Assert(entities, HasLen, 3)
	sort.Sort(nova.EntitySortBy{"Id", expected.Flavors})
	sort.Sort(nova.EntitySortBy{"Id", entities})
	c.Assert(expected.Flavors, DeepEquals, entities)
	var expectedFlavor struct {
		Flavor nova.FlavorDetail
	}
	resp, err = s.authRequest("GET", "/flavors/1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expectedFlavor)
	c.Assert(expectedFlavor.Flavor.Name, Equals, "m1.tiny")
}

func (s *NovaHTTPSuite) TestGetFlavorsDetail(c *C) {
	// The test service has 3 default flavours.
	flavors := s.service.allFlavors()
	c.Assert(flavors, HasLen, 3)
	var expected struct {
		Flavors []nova.FlavorDetail
	}
	resp, err := s.authRequest("GET", "/flavors/detail", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Flavors, HasLen, 3)
	sort.Sort(nova.FlavorDetailSortBy{"Id", expected.Flavors})
	sort.Sort(nova.FlavorDetailSortBy{"Id", flavors})
	c.Assert(expected.Flavors, DeepEquals, flavors)
	resp, err = s.authRequest("GET", "/flavors/detail/1", nil, nil)
	c.Assert(err, IsNil)
	assertBody(c, resp, errNotFound)
}

func (s *NovaHTTPSuite) TestGetServers(c *C) {
	entities := s.service.allServersAsEntities(nil)
	c.Assert(entities, HasLen, 0)
	var expected struct {
		Servers []nova.Entity
	}
	resp, err := s.authRequest("GET", "/servers", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 0)
	servers := []nova.ServerDetail{
		{Id: "sr1", Name: "server 1"},
		{Id: "sr2", Name: "server 2"},
	}
	for i, server := range servers {
		s.service.buildServerLinks(&server)
		servers[i] = server
		err := s.service.addServer(server)
		c.Assert(err, IsNil)
		defer s.service.removeServer(server.Id)
	}
	entities = s.service.allServersAsEntities(nil)
	resp, err = s.authRequest("GET", "/servers", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 2)
	if expected.Servers[0].Id != entities[0].Id {
		expected.Servers[0], expected.Servers[1] = expected.Servers[1], expected.Servers[0]
	}
	c.Assert(expected.Servers, DeepEquals, entities)
	var expectedServer struct {
		Server nova.ServerDetail
	}
	resp, err = s.authRequest("GET", "/servers/sr1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expectedServer)
	c.Assert(expectedServer.Server, DeepEquals, servers[0])
}

func (s *NovaHTTPSuite) TestGetServersWithFilters(c *C) {
	entities := s.service.allServersAsEntities(nil)
	c.Assert(entities, HasLen, 0)
	var expected struct {
		Servers []nova.Entity
	}
	url := "/servers?status=RESCUE&status=BUILD&name=srv2&name=srv1"
	resp, err := s.authRequest("GET", url, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 0)
	servers := []nova.ServerDetail{
		{Id: "sr1", Name: "srv1", Status: nova.StatusBuild},
		{Id: "sr2", Name: "srv2", Status: nova.StatusRescue},
		{Id: "sr3", Name: "srv3", Status: nova.StatusActive},
	}
	for i, server := range servers {
		s.service.buildServerLinks(&server)
		servers[i] = server
		err := s.service.addServer(server)
		c.Assert(err, IsNil)
		defer s.service.removeServer(server.Id)
	}
	resp, err = s.authRequest("GET", url, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 1)
	c.Assert(expected.Servers[0].Id, Equals, servers[0].Id)
	c.Assert(expected.Servers[0].Name, Equals, servers[0].Name)
}

func (s *NovaHTTPSuite) TestNewUUID(c *C) {
	uuid, err := newUUID()
	c.Assert(err, IsNil)
	var p1, p2, p3, p4, p5 string
	num, err := fmt.Sscanf(uuid, "%8x-%4x-%4x-%4x-%12x", &p1, &p2, &p3, &p4, &p5)
	c.Assert(err, IsNil)
	c.Assert(num, Equals, 5)
	uuid2, err := newUUID()
	c.Assert(err, IsNil)
	c.Assert(uuid2, Not(Equals), uuid)
}

func (s *NovaHTTPSuite) assertAddresses(c *C, serverId string) {
	server, err := s.service.server(serverId)
	c.Assert(err, IsNil)
	c.Assert(server.Addresses, HasLen, 2)
	c.Assert(server.Addresses["public"], HasLen, 2)
	c.Assert(server.Addresses["private"], HasLen, 2)
	for network, addresses := range server.Addresses {
		for _, addr := range addresses {
			if addr.Version == 4 && network == "public" {
				c.Assert(addr.Address, Matches, `127\.10\.0\.\d{1,3}`)
			} else if addr.Version == 4 && network == "private" {
				c.Assert(addr.Address, Matches, `127\.0\.0\.\d{1,3}`)
			}
		}

	}
}

func (s *NovaHTTPSuite) TestRunServer(c *C) {
	entities := s.service.allServersAsEntities(nil)
	c.Assert(entities, HasLen, 0)
	var req struct {
		Server struct {
			FlavorRef      string              `json:"flavorRef"`
			ImageRef       string              `json:"imageRef"`
			Name           string              `json:"name"`
			SecurityGroups []map[string]string `json:"security_groups"`
		} `json:"server"`
	}
	resp, err := s.jsonRequest("POST", "/servers", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusBadRequest)
	assertBody(c, resp, errBadRequestSrvName)
	req.Server.Name = "srv1"
	resp, err = s.jsonRequest("POST", "/servers", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusBadRequest)
	assertBody(c, resp, errBadRequestSrvImage)
	req.Server.ImageRef = "image"
	resp, err = s.jsonRequest("POST", "/servers", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusBadRequest)
	assertBody(c, resp, errBadRequestSrvFlavor)
	req.Server.FlavorRef = "flavor"
	var expected struct {
		Server struct {
			SecurityGroups []map[string]string `json:"security_groups"`
			Id             string
			Links          []nova.Link
			AdminPass      string
		}
	}
	resp, err = s.jsonRequest("POST", "/servers", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Server.SecurityGroups, HasLen, 1)
	c.Assert(expected.Server.SecurityGroups[0]["name"], Equals, "default")
	c.Assert(expected.Server.Id, Not(Equals), "")
	c.Assert(expected.Server.Links, HasLen, 2)
	c.Assert(expected.Server.AdminPass, Not(Equals), "")
	s.assertAddresses(c, expected.Server.Id)
	srv, err := s.service.server(expected.Server.Id)
	c.Assert(err, IsNil)
	c.Assert(srv.Links, DeepEquals, expected.Server.Links)
	s.service.removeServer(srv.Id)
	req.Server.Name = "test2"
	req.Server.SecurityGroups = []map[string]string{
		{"name": "default"},
		{"name": "group1"},
		{"name": "group2"},
	}
	err = s.service.addSecurityGroup(nova.SecurityGroup{Id: "1", Name: "group1"})
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup("1")
	err = s.service.addSecurityGroup(nova.SecurityGroup{Id: "2", Name: "group2"})
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup("2")
	resp, err = s.jsonRequest("POST", "/servers", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Server.SecurityGroups, DeepEquals, req.Server.SecurityGroups)
	srv, err = s.service.server(expected.Server.Id)
	c.Assert(err, IsNil)
	ok := s.service.hasServerSecurityGroup(srv.Id, "1")
	c.Assert(ok, Equals, true)
	ok = s.service.hasServerSecurityGroup(srv.Id, "2")
	c.Assert(ok, Equals, true)
	ok = s.service.hasServerSecurityGroup(srv.Id, "999")
	c.Assert(ok, Equals, true)
	s.service.removeServerSecurityGroup(srv.Id, "1")
	s.service.removeServerSecurityGroup(srv.Id, "2")
	s.service.removeServerSecurityGroup(srv.Id, "999")
	s.service.removeServer(srv.Id)
}

func (s *NovaHTTPSuite) TestDeleteServer(c *C) {
	server := nova.ServerDetail{Id: "sr1"}
	_, err := s.service.server(server.Id)
	c.Assert(err, NotNil)
	err = s.service.addServer(server)
	c.Assert(err, IsNil)
	defer s.service.removeServer(server.Id)
	resp, err := s.authRequest("DELETE", "/servers/sr1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusNoContent)
	_, err = s.service.server(server.Id)
	c.Assert(err, NotNil)
}

func (s *NovaHTTPSuite) TestGetServersDetail(c *C) {
	servers := s.service.allServers(nil)
	c.Assert(servers, HasLen, 0)
	var expected struct {
		Servers []nova.ServerDetail `json:"servers"`
	}
	resp, err := s.authRequest("GET", "/servers/detail", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 0)
	servers = []nova.ServerDetail{
		{Id: "sr1", Name: "server 1"},
		{Id: "sr2", Name: "server 2"},
	}
	for i, server := range servers {
		s.service.buildServerLinks(&server)
		servers[i] = server
		err := s.service.addServer(server)
		c.Assert(err, IsNil)
		defer s.service.removeServer(server.Id)
	}
	resp, err = s.authRequest("GET", "/servers/detail", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 2)
	if expected.Servers[0].Id != servers[0].Id {
		expected.Servers[0], expected.Servers[1] = expected.Servers[1], expected.Servers[0]
	}
	c.Assert(expected.Servers, DeepEquals, servers)
	resp, err = s.authRequest("GET", "/servers/detail/sr1", nil, nil)
	c.Assert(err, IsNil)
	assertBody(c, resp, errNotFound)
}

func (s *NovaHTTPSuite) TestGetServersDetailWithFilters(c *C) {
	servers := s.service.allServers(nil)
	c.Assert(servers, HasLen, 0)
	var expected struct {
		Servers []nova.ServerDetail `json:"servers"`
	}
	url := "/servers/detail?status=RESCUE&status=BUILD&name=srv2&name=srv1"
	resp, err := s.authRequest("GET", url, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 0)
	servers = []nova.ServerDetail{
		{Id: "sr1", Name: "srv1", Status: nova.StatusBuild},
		{Id: "sr2", Name: "srv2", Status: nova.StatusRescue},
		{Id: "sr3", Name: "srv3", Status: nova.StatusActive},
	}
	for i, server := range servers {
		s.service.buildServerLinks(&server)
		servers[i] = server
		err := s.service.addServer(server)
		c.Assert(err, IsNil)
		defer s.service.removeServer(server.Id)
	}
	resp, err = s.authRequest("GET", url, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Servers, HasLen, 1)
	c.Assert(expected.Servers[0], DeepEquals, servers[0])
}

func (s *NovaHTTPSuite) TestGetSecurityGroups(c *C) {
	// There is always a default security group.
	groups := s.service.allSecurityGroups()
	c.Assert(groups, HasLen, 1)
	var expected struct {
		Groups []nova.SecurityGroup `json:"security_groups"`
	}
	resp, err := s.authRequest("GET", "/os-security-groups", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Groups, HasLen, 1)
	groups = []nova.SecurityGroup{
		{
			Id:       "1",
			Name:     "group 1",
			TenantId: s.service.TenantId,
			Rules:    []nova.SecurityGroupRule{},
		},
		{
			Id:       "2",
			Name:     "group 2",
			TenantId: s.service.TenantId,
			Rules:    []nova.SecurityGroupRule{},
		},
	}
	for _, group := range groups {
		err := s.service.addSecurityGroup(group)
		c.Assert(err, IsNil)
		defer s.service.removeSecurityGroup(group.Id)
	}
	resp, err = s.authRequest("GET", "/os-security-groups", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Groups, HasLen, len(groups)+1)
	checkGroupsInList(c, groups, expected.Groups)
	var expectedGroup struct {
		Group nova.SecurityGroup `json:"security_group"`
	}
	resp, err = s.authRequest("GET", "/os-security-groups/1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expectedGroup)
	c.Assert(expectedGroup.Group, DeepEquals, groups[0])
}

func (s *NovaHTTPSuite) TestAddSecurityGroup(c *C) {
	group := nova.SecurityGroup{
		Id:          "1",
		Name:        "group 1",
		Description: "desc",
		TenantId:    s.service.TenantId,
		Rules:       []nova.SecurityGroupRule{},
	}
	_, err := s.service.securityGroup(group.Id)
	c.Assert(err, NotNil)
	var req struct {
		Group struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"security_group"`
	}
	req.Group.Name = group.Name
	req.Group.Description = group.Description
	var expected struct {
		Group nova.SecurityGroup `json:"security_group"`
	}
	resp, err := s.jsonRequest("POST", "/os-security-groups", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Group, DeepEquals, group)
	err = s.service.removeSecurityGroup(group.Id)
	c.Assert(err, IsNil)
}

func (s *NovaHTTPSuite) TestDeleteSecurityGroup(c *C) {
	group := nova.SecurityGroup{Id: "1", Name: "group 1"}
	_, err := s.service.securityGroup(group.Id)
	c.Assert(err, NotNil)
	err = s.service.addSecurityGroup(group)
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup(group.Id)
	resp, err := s.authRequest("DELETE", "/os-security-groups/1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	_, err = s.service.securityGroup(group.Id)
	c.Assert(err, NotNil)
}

func (s *NovaHTTPSuite) TestAddSecurityGroupRule(c *C) {
	group1 := nova.SecurityGroup{Id: "1", Name: "src"}
	group2 := nova.SecurityGroup{Id: "2", Name: "tgt"}
	err := s.service.addSecurityGroup(group1)
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup(group1.Id)
	err = s.service.addSecurityGroup(group2)
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup(group2.Id)
	riIngress := nova.RuleInfo{
		ParentGroupId: "1",
		FromPort:      1234,
		ToPort:        4321,
		IPProtocol:    "tcp",
		Cidr:          "1.2.3.4/5",
	}
	riGroup := nova.RuleInfo{
		ParentGroupId: group2.Id,
		GroupId:       &group1.Id,
	}
	iprange := make(map[string]string)
	iprange["cidr"] = riIngress.Cidr
	rule1 := nova.SecurityGroupRule{
		Id:            "1",
		ParentGroupId: group1.Id,
		FromPort:      &riIngress.FromPort,
		ToPort:        &riIngress.ToPort,
		IPProtocol:    &riIngress.IPProtocol,
		IPRange:       iprange,
	}
	rule2 := nova.SecurityGroupRule{
		Id:            "2",
		ParentGroupId: group2.Id,
		Group: nova.SecurityGroupRef{
			Name:     group1.Name,
			TenantId: s.service.TenantId,
		},
	}
	ok := s.service.hasSecurityGroupRule(group1.Id, rule1.Id)
	c.Assert(ok, Equals, false)
	ok = s.service.hasSecurityGroupRule(group2.Id, rule2.Id)
	c.Assert(ok, Equals, false)
	var req struct {
		Rule nova.RuleInfo `json:"security_group_rule"`
	}
	req.Rule = riIngress
	var expected struct {
		Rule nova.SecurityGroupRule `json:"security_group_rule"`
	}
	resp, err := s.jsonRequest("POST", "/os-security-group-rules", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Rule.Id, Equals, rule1.Id)
	c.Assert(expected.Rule.ParentGroupId, Equals, rule1.ParentGroupId)
	c.Assert(expected.Rule.Group, Equals, nova.SecurityGroupRef{})
	c.Assert(*expected.Rule.FromPort, Equals, *rule1.FromPort)
	c.Assert(*expected.Rule.ToPort, Equals, *rule1.ToPort)
	c.Assert(*expected.Rule.IPProtocol, Equals, *rule1.IPProtocol)
	c.Assert(expected.Rule.IPRange, DeepEquals, rule1.IPRange)
	defer s.service.removeSecurityGroupRule(rule1.Id)
	req.Rule = riGroup
	resp, err = s.jsonRequest("POST", "/os-security-group-rules", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Rule.Id, Equals, rule2.Id)
	c.Assert(expected.Rule.ParentGroupId, Equals, rule2.ParentGroupId)
	c.Assert(expected.Rule.Group, DeepEquals, rule2.Group)
	err = s.service.removeSecurityGroupRule(rule2.Id)
	c.Assert(err, IsNil)
}

func (s *NovaHTTPSuite) TestDeleteSecurityGroupRule(c *C) {
	group1 := nova.SecurityGroup{Id: "1", Name: "src"}
	group2 := nova.SecurityGroup{Id: "2", Name: "tgt"}
	err := s.service.addSecurityGroup(group1)
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup(group1.Id)
	err = s.service.addSecurityGroup(group2)
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup(group2.Id)
	riGroup := nova.RuleInfo{
		ParentGroupId: group2.Id,
		GroupId:       &group1.Id,
	}
	rule := nova.SecurityGroupRule{
		Id:            "1",
		ParentGroupId: group2.Id,
		Group: nova.SecurityGroupRef{
			Name:     group1.Name,
			TenantId: group1.TenantId,
		},
	}
	err = s.service.addSecurityGroupRule(rule.Id, riGroup)
	c.Assert(err, IsNil)
	resp, err := s.authRequest("DELETE", "/os-security-group-rules/1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	ok := s.service.hasSecurityGroupRule(group2.Id, rule.Id)
	c.Assert(ok, Equals, false)
}

func (s *NovaHTTPSuite) TestAddServerSecurityGroup(c *C) {
	group := nova.SecurityGroup{Id: "1", Name: "group"}
	err := s.service.addSecurityGroup(group)
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup(group.Id)
	server := nova.ServerDetail{Id: "sr1"}
	err = s.service.addServer(server)
	c.Assert(err, IsNil)
	defer s.service.removeServer(server.Id)
	ok := s.service.hasServerSecurityGroup(server.Id, group.Id)
	c.Assert(ok, Equals, false)
	var req struct {
		Group struct {
			Name string `json:"name"`
		} `json:"addSecurityGroup"`
	}
	req.Group.Name = group.Name
	resp, err := s.jsonRequest("POST", "/servers/"+server.Id+"/action", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	ok = s.service.hasServerSecurityGroup(server.Id, group.Id)
	c.Assert(ok, Equals, true)
	err = s.service.removeServerSecurityGroup(server.Id, group.Id)
	c.Assert(err, IsNil)
}

func (s *NovaHTTPSuite) TestGetServerSecurityGroups(c *C) {
	server := nova.ServerDetail{Id: "sr1"}
	groups := []nova.SecurityGroup{
		{
			Id:       "1",
			Name:     "group1",
			TenantId: s.service.TenantId,
			Rules:    []nova.SecurityGroupRule{},
		},
		{
			Id:       "2",
			Name:     "group2",
			TenantId: s.service.TenantId,
			Rules:    []nova.SecurityGroupRule{},
		},
	}
	srvGroups := s.service.allServerSecurityGroups(server.Id)
	c.Assert(srvGroups, HasLen, 0)
	err := s.service.addServer(server)
	c.Assert(err, IsNil)
	defer s.service.removeServer(server.Id)
	for _, group := range groups {
		err = s.service.addSecurityGroup(group)
		c.Assert(err, IsNil)
		defer s.service.removeSecurityGroup(group.Id)
		err = s.service.addServerSecurityGroup(server.Id, group.Id)
		c.Assert(err, IsNil)
		defer s.service.removeServerSecurityGroup(server.Id, group.Id)
	}
	srvGroups = s.service.allServerSecurityGroups(server.Id)
	var expected struct {
		Groups []nova.SecurityGroup `json:"security_groups"`
	}
	resp, err := s.authRequest("GET", "/servers/"+server.Id+"/os-security-groups", nil, nil)
	c.Assert(err, IsNil)
	assertJSON(c, resp, &expected)
	c.Assert(expected.Groups, DeepEquals, groups)
}

func (s *NovaHTTPSuite) TestDeleteServerSecurityGroup(c *C) {
	group := nova.SecurityGroup{Id: "1", Name: "group"}
	err := s.service.addSecurityGroup(group)
	c.Assert(err, IsNil)
	defer s.service.removeSecurityGroup(group.Id)
	server := nova.ServerDetail{Id: "sr1"}
	err = s.service.addServer(server)
	c.Assert(err, IsNil)
	defer s.service.removeServer(server.Id)
	ok := s.service.hasServerSecurityGroup(server.Id, group.Id)
	c.Assert(ok, Equals, false)
	err = s.service.addServerSecurityGroup(server.Id, group.Id)
	c.Assert(err, IsNil)
	var req struct {
		Group struct {
			Name string `json:"name"`
		} `json:"removeSecurityGroup"`
	}
	req.Group.Name = group.Name
	resp, err := s.jsonRequest("POST", "/servers/"+server.Id+"/action", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	ok = s.service.hasServerSecurityGroup(server.Id, group.Id)
	c.Assert(ok, Equals, false)
}

func (s *NovaHTTPSuite) TestPostFloatingIP(c *C) {
	fip := nova.FloatingIP{Id: "1", IP: "10.0.0.1", Pool: "nova"}
	c.Assert(s.service.allFloatingIPs(), HasLen, 0)
	var expected struct {
		IP nova.FloatingIP `json:"floating_ip"`
	}
	resp, err := s.authRequest("POST", "/os-floating-ips", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.IP, DeepEquals, fip)
	err = s.service.removeFloatingIP(fip.Id)
	c.Assert(err, IsNil)
}

func (s *NovaHTTPSuite) TestGetFloatingIPs(c *C) {
	c.Assert(s.service.allFloatingIPs(), HasLen, 0)
	var expected struct {
		IPs []nova.FloatingIP `json:"floating_ips"`
	}
	resp, err := s.authRequest("GET", "/os-floating-ips", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	c.Assert(expected.IPs, HasLen, 0)
	fips := []nova.FloatingIP{
		{Id: "1", IP: "1.2.3.4", Pool: "nova"},
		{Id: "2", IP: "4.3.2.1", Pool: "nova"},
	}
	for _, fip := range fips {
		err := s.service.addFloatingIP(fip)
		defer s.service.removeFloatingIP(fip.Id)
		c.Assert(err, IsNil)
	}
	resp, err = s.authRequest("GET", "/os-floating-ips", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expected)
	if expected.IPs[0].Id != fips[0].Id {
		expected.IPs[0], expected.IPs[1] = expected.IPs[1], expected.IPs[0]
	}
	c.Assert(expected.IPs, DeepEquals, fips)
	var expectedIP struct {
		IP nova.FloatingIP `json:"floating_ip"`
	}
	resp, err = s.authRequest("GET", "/os-floating-ips/1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	assertJSON(c, resp, &expectedIP)
	c.Assert(expectedIP.IP, DeepEquals, fips[0])
}

func (s *NovaHTTPSuite) TestDeleteFloatingIP(c *C) {
	fip := nova.FloatingIP{Id: "1", IP: "10.0.0.1", Pool: "nova"}
	err := s.service.addFloatingIP(fip)
	c.Assert(err, IsNil)
	defer s.service.removeFloatingIP(fip.Id)
	resp, err := s.authRequest("DELETE", "/os-floating-ips/1", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	_, err = s.service.floatingIP(fip.Id)
	c.Assert(err, NotNil)
}

func (s *NovaHTTPSuite) TestAddServerFloatingIP(c *C) {
	fip := nova.FloatingIP{Id: "1", IP: "1.2.3.4"}
	server := nova.ServerDetail{Id: "sr1"}
	err := s.service.addFloatingIP(fip)
	c.Assert(err, IsNil)
	defer s.service.removeFloatingIP(fip.Id)
	err = s.service.addServer(server)
	c.Assert(err, IsNil)
	defer s.service.removeServer(server.Id)
	c.Assert(s.service.hasServerFloatingIP(server.Id, fip.IP), Equals, false)
	var req struct {
		AddFloatingIP struct {
			Address string `json:"address"`
		} `json:"addFloatingIp"`
	}
	req.AddFloatingIP.Address = fip.IP
	resp, err := s.jsonRequest("POST", "/servers/"+server.Id+"/action", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	c.Assert(s.service.hasServerFloatingIP(server.Id, fip.IP), Equals, true)
	err = s.service.removeServerFloatingIP(server.Id, fip.Id)
	c.Assert(err, IsNil)
}

func (s *NovaHTTPSuite) TestRemoveServerFloatingIP(c *C) {
	fip := nova.FloatingIP{Id: "1", IP: "1.2.3.4"}
	server := nova.ServerDetail{Id: "sr1"}
	err := s.service.addFloatingIP(fip)
	c.Assert(err, IsNil)
	defer s.service.removeFloatingIP(fip.Id)
	err = s.service.addServer(server)
	c.Assert(err, IsNil)
	defer s.service.removeServer(server.Id)
	err = s.service.addServerFloatingIP(server.Id, fip.Id)
	c.Assert(err, IsNil)
	defer s.service.removeServerFloatingIP(server.Id, fip.Id)
	c.Assert(s.service.hasServerFloatingIP(server.Id, fip.IP), Equals, true)
	var req struct {
		RemoveFloatingIP struct {
			Address string `json:"address"`
		} `json:"removeFloatingIp"`
	}
	req.RemoveFloatingIP.Address = fip.IP
	resp, err := s.jsonRequest("POST", "/servers/"+server.Id+"/action", req, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusAccepted)
	c.Assert(s.service.hasServerFloatingIP(server.Id, fip.IP), Equals, false)
}

func (s *NovaHTTPSSuite) SetUpSuite(c *C) {
	s.HTTPSuite.SetUpSuite(c)
	identityDouble := identityservice.NewUserPass()
	userInfo := identityDouble.AddUser("fred", "secret", "tenant")
	s.token = userInfo.Token
	c.Assert(s.Server.URL[:8], Equals, "https://")
	s.service = New(s.Server.URL, versionPath, userInfo.TenantId, region, identityDouble)
}

func (s *NovaHTTPSSuite) TearDownSuite(c *C) {
	s.HTTPSuite.TearDownSuite(c)
}

func (s *NovaHTTPSSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.service.SetupHTTP(s.Mux)
}

func (s *NovaHTTPSSuite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
}

func (s *NovaHTTPSSuite) TestHasHTTPSServiceURL(c *C) {
	endpoints := s.service.Endpoints()
	c.Assert(endpoints[0].PublicURL[:8], Equals, "https://")
}
