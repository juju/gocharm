// Swift double testing service - HTTP API tests

package swiftservice

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/swift"
	"launchpad.net/goose/testing/httpsuite"
	"launchpad.net/goose/testservices/identityservice"
	"net/http"
	"net/url"
)

type SwiftHTTPSuite struct {
	httpsuite.HTTPSuite
	service *Swift
	token   string
}

var _ = Suite(&SwiftHTTPSuite{})

type SwiftHTTPSSuite struct {
	httpsuite.HTTPSuite
	service *Swift
	token   string
}

var _ = Suite(&SwiftHTTPSSuite{HTTPSuite: httpsuite.HTTPSuite{UseTLS: true}})

func (s *SwiftHTTPSuite) SetUpSuite(c *C) {
	s.HTTPSuite.SetUpSuite(c)
	identityDouble := identityservice.NewUserPass()
	s.service = New(s.Server.URL, versionPath, tenantId, region, identityDouble)
	userInfo := identityDouble.AddUser("fred", "secret", "tenant")
	s.token = userInfo.Token
}

func (s *SwiftHTTPSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.service.SetupHTTP(s.Mux)
}

func (s *SwiftHTTPSuite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
}

func (s *SwiftHTTPSuite) TearDownSuite(c *C) {
	s.HTTPSuite.TearDownSuite(c)
}

func (s *SwiftHTTPSuite) sendRequest(c *C, method, path string, body []byte,
	expectedStatusCode int) (resp *http.Response) {
	return s.sendRequestWithParams(c, method, path, nil, body, expectedStatusCode)
}

func (s *SwiftHTTPSuite) sendRequestWithParams(c *C, method, path string, params map[string]string, body []byte,
	expectedStatusCode int) (resp *http.Response) {
	var req *http.Request
	var err error
	URL := s.service.endpointURL(path)
	if len(params) > 0 {
		urlParams := make(url.Values, len(params))
		for k, v := range params {
			urlParams.Set(k, v)
		}
		URL += "?" + urlParams.Encode()
	}
	if body != nil {
		req, err = http.NewRequest(method, URL, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, URL, nil)
	}
	c.Assert(err, IsNil)
	if s.token != "" {
		req.Header.Add("X-Auth-Token", s.token)
	}
	client := &http.DefaultClient
	resp, err = client.Do(req)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, expectedStatusCode)
	return resp
}

func (s *SwiftHTTPSuite) ensureNotContainer(name string, c *C) {
	ok := s.service.HasContainer("test")
	c.Assert(ok, Equals, false)
}

func (s *SwiftHTTPSuite) ensureContainer(name string, c *C) {
	s.ensureNotContainer(name, c)
	err := s.service.AddContainer("test")
	c.Assert(err, IsNil)
}

func (s *SwiftHTTPSuite) removeContainer(name string, c *C) {
	ok := s.service.HasContainer("test")
	c.Assert(ok, Equals, true)
	err := s.service.RemoveContainer("test")
	c.Assert(err, IsNil)
}

func (s *SwiftHTTPSuite) ensureNotObject(container, object string, c *C) {
	_, err := s.service.GetObject(container, object)
	c.Assert(err, Not(IsNil))
}

func (s *SwiftHTTPSuite) ensureObject(container, object string, data []byte, c *C) {
	s.ensureNotObject(container, object, c)
	err := s.service.AddObject(container, object, data)
	c.Assert(err, IsNil)
}

func (s *SwiftHTTPSuite) ensureObjectData(container, object string, data []byte, c *C) {
	objdata, err := s.service.GetObject(container, object)
	c.Assert(err, IsNil)
	c.Assert(objdata, DeepEquals, data)
}

func (s *SwiftHTTPSuite) removeObject(container, object string, c *C) {
	err := s.service.RemoveObject(container, object)
	c.Assert(err, IsNil)
	s.ensureNotObject(container, object, c)
}

func (s *SwiftHTTPSuite) TestPUTContainerMissingCreated(c *C) {
	s.ensureNotContainer("test", c)

	s.sendRequest(c, "PUT", "test", nil, http.StatusCreated)

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestPUTContainerExistsAccepted(c *C) {
	s.ensureContainer("test", c)

	s.sendRequest(c, "PUT", "test", nil, http.StatusAccepted)

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestGETContainerMissingNotFound(c *C) {
	s.ensureNotContainer("test", c)

	s.sendRequest(c, "GET", "test", nil, http.StatusNotFound)

	s.ensureNotContainer("test", c)
}

func (s *SwiftHTTPSuite) TestGETContainerExistsOK(c *C) {
	s.ensureContainer("test", c)
	data := []byte("test data")
	s.ensureObject("test", "obj", data, c)

	resp := s.sendRequest(c, "GET", "test", nil, http.StatusOK)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	var containerData []swift.ContainerContents
	err = json.Unmarshal(body, &containerData)
	c.Assert(err, IsNil)
	c.Assert(len(containerData), Equals, 1)
	c.Assert(containerData[0].Name, Equals, "obj")

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestGETContainerWithPrefix(c *C) {
	s.ensureContainer("test", c)
	data := []byte("test data")
	s.ensureObject("test", "foo", data, c)
	s.ensureObject("test", "foobar", data, c)

	resp := s.sendRequestWithParams(c, "GET", "test", map[string]string{"prefix": "foob"}, nil, http.StatusOK)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	var containerData []swift.ContainerContents
	err = json.Unmarshal(body, &containerData)
	c.Assert(err, IsNil)
	c.Assert(len(containerData), Equals, 1)
	c.Assert(containerData[0].Name, Equals, "foobar")

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestDELETEContainerMissingNotFound(c *C) {
	s.ensureNotContainer("test", c)

	s.sendRequest(c, "DELETE", "test", nil, http.StatusNotFound)
}

func (s *SwiftHTTPSuite) TestDELETEContainerExistsNoContent(c *C) {
	s.ensureContainer("test", c)

	s.sendRequest(c, "DELETE", "test", nil, http.StatusNoContent)

	s.ensureNotContainer("test", c)
}

func (s *SwiftHTTPSuite) TestPUTObjectMissingCreated(c *C) {
	s.ensureContainer("test", c)
	s.ensureNotObject("test", "obj", c)

	data := []byte("test data")
	s.sendRequest(c, "PUT", "test/obj", data, http.StatusCreated)

	s.ensureObjectData("test", "obj", data, c)
	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestPUTObjectExistsCreated(c *C) {
	data := []byte("test data")
	s.ensureContainer("test", c)
	s.ensureObject("test", "obj", data, c)

	newdata := []byte("new test data")
	s.sendRequest(c, "PUT", "test/obj", newdata, http.StatusCreated)

	s.ensureObjectData("test", "obj", newdata, c)
	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestPUTObjectContainerMissingNotFound(c *C) {
	s.ensureNotContainer("test", c)

	data := []byte("test data")
	s.sendRequest(c, "PUT", "test/obj", data, http.StatusNotFound)

	s.ensureNotContainer("test", c)
}

func (s *SwiftHTTPSuite) TestGETObjectMissingNotFound(c *C) {
	s.ensureContainer("test", c)
	s.ensureNotObject("test", "obj", c)

	s.sendRequest(c, "GET", "test/obj", nil, http.StatusNotFound)

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestGETObjectContainerMissingNotFound(c *C) {
	s.ensureNotContainer("test", c)

	s.sendRequest(c, "GET", "test/obj", nil, http.StatusNotFound)

	s.ensureNotContainer("test", c)
}

func (s *SwiftHTTPSuite) TestGETObjectExistsOK(c *C) {
	data := []byte("test data")
	s.ensureContainer("test", c)
	s.ensureObject("test", "obj", data, c)

	resp := s.sendRequest(c, "GET", "test/obj", nil, http.StatusOK)

	s.ensureObjectData("test", "obj", data, c)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	c.Assert(body, DeepEquals, data)

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestDELETEObjectMissingNotFound(c *C) {
	s.ensureContainer("test", c)
	s.ensureNotObject("test", "obj", c)

	s.sendRequest(c, "DELETE", "test/obj", nil, http.StatusNotFound)

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestDELETEObjectContainerMissingNotFound(c *C) {
	s.ensureNotContainer("test", c)

	s.sendRequest(c, "DELETE", "test/obj", nil, http.StatusNotFound)

	s.ensureNotContainer("test", c)
}

func (s *SwiftHTTPSuite) TestDELETEObjectExistsNoContent(c *C) {
	data := []byte("test data")
	s.ensureContainer("test", c)
	s.ensureObject("test", "obj", data, c)

	s.sendRequest(c, "DELETE", "test/obj", nil, http.StatusNoContent)

	s.removeContainer("test", c)
}

func (s *SwiftHTTPSuite) TestUnauthorizedFails(c *C) {
	oldtoken := s.token
	defer func() {
		s.token = oldtoken
	}()
	// TODO(wallyworld) - 2013-02-11 bug=1121682
	// until ACLs are supported, empty tokens are assumed to be used when we need to access a public container.
	// token = ""
	// s.sendRequest(c, "GET", "test", nil, http.StatusUnauthorized)

	s.token = "invalid"
	s.sendRequest(c, "PUT", "test", nil, http.StatusUnauthorized)

	s.sendRequest(c, "DELETE", "test", nil, http.StatusUnauthorized)
}

func (s *SwiftHTTPSSuite) SetUpSuite(c *C) {
	s.HTTPSuite.SetUpSuite(c)
	identityDouble := identityservice.NewUserPass()
	userInfo := identityDouble.AddUser("fred", "secret", "tenant")
	s.token = userInfo.Token
	c.Assert(s.Server.URL[:8], Equals, "https://")
	s.service = New(s.Server.URL, versionPath, userInfo.TenantId, region, identityDouble)
}

func (s *SwiftHTTPSSuite) TearDownSuite(c *C) {
	s.HTTPSuite.TearDownSuite(c)
}

func (s *SwiftHTTPSSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.service.SetupHTTP(s.Mux)
}

func (s *SwiftHTTPSSuite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
}

func (s *SwiftHTTPSSuite) TestHasHTTPSServiceURL(c *C) {
	endpoints := s.service.Endpoints()
	c.Assert(endpoints[0].PublicURL[:8], Equals, "https://")
}
