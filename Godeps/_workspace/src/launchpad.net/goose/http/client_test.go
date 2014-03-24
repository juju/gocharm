package http

import (
	"bytes"
	"fmt"
	"io/ioutil"

	. "launchpad.net/gocheck"

	"launchpad.net/goose/testing/httpsuite"
	"net/http"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type LoopingHTTPSuite struct {
	httpsuite.HTTPSuite
}

func (s *LoopingHTTPSuite) setupLoopbackRequest() (*http.Header, chan string, *Client) {
	var headers http.Header
	bodyChan := make(chan string, 1)
	handler := func(resp http.ResponseWriter, req *http.Request) {
		headers = req.Header
		bodyBytes, _ := ioutil.ReadAll(req.Body)
		req.Body.Close()
		bodyChan <- string(bodyBytes)
		resp.Header().Add("Content-Length", "0")
		resp.WriteHeader(http.StatusNoContent)
		resp.Write([]byte{})
	}
	s.Mux.HandleFunc("/", handler)
	client := New()
	return &headers, bodyChan, client
}

type HTTPClientTestSuite struct {
	LoopingHTTPSuite
}

type HTTPSClientTestSuite struct {
	LoopingHTTPSuite
}

var _ = Suite(&HTTPClientTestSuite{})
var _ = Suite(&HTTPSClientTestSuite{LoopingHTTPSuite{httpsuite.HTTPSuite{UseTLS: true}}})

func (s *HTTPClientTestSuite) assertHeaderValues(c *C, token string) {
	emptyHeaders := http.Header{}
	headers := createHeaders(emptyHeaders, "content-type", token)
	contentTypes := []string{"content-type"}
	headerData := map[string][]string{
		"Content-Type": contentTypes, "Accept": contentTypes, "User-Agent": []string{gooseAgent()}}
	if token != "" {
		headerData["X-Auth-Token"] = []string{token}
	}
	expectedHeaders := http.Header(headerData)
	c.Assert(headers, DeepEquals, expectedHeaders)
	c.Assert(emptyHeaders, DeepEquals, http.Header{})
}

func (s *HTTPClientTestSuite) TestCreateHeadersNoToken(c *C) {
	s.assertHeaderValues(c, "")
}

func (s *HTTPClientTestSuite) TestCreateHeadersWithToken(c *C) {
	s.assertHeaderValues(c, "token")
}

func (s *HTTPClientTestSuite) TestCreateHeadersCopiesSupplied(c *C) {
	initialHeaders := make(http.Header)
	initialHeaders["Foo"] = []string{"Bar"}
	contentType := contentTypeJSON
	contentTypes := []string{contentType}
	headers := createHeaders(initialHeaders, contentType, "")
	// it should not change the headers passed in
	c.Assert(initialHeaders, DeepEquals, http.Header{"Foo": []string{"Bar"}})
	// The initial headers should be in the output
	c.Assert(headers, DeepEquals,
		http.Header{"Foo": []string{"Bar"}, "Content-Type": contentTypes, "Accept": contentTypes, "User-Agent": []string{gooseAgent()}})
}

func (s *HTTPClientTestSuite) TestBinaryRequestSetsUserAgent(c *C) {
	headers, _, client := s.setupLoopbackRequest()
	req := &RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := client.BinaryRequest("POST", s.Server.URL, "", req, nil)
	c.Assert(err, IsNil)
	agent := headers.Get("User-Agent")
	c.Check(agent, Not(Equals), "")
	c.Check(agent, Equals, gooseAgent())
}

func (s *HTTPClientTestSuite) TestJSONRequestSetsUserAgent(c *C) {
	headers, _, client := s.setupLoopbackRequest()
	req := &RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := client.JsonRequest("POST", s.Server.URL, "", req, nil)
	c.Assert(err, IsNil)
	agent := headers.Get("User-Agent")
	c.Check(agent, Not(Equals), "")
	c.Check(agent, Equals, gooseAgent())
}

func (s *HTTPClientTestSuite) TestBinaryRequestSetsContentLength(c *C) {
	headers, bodyChan, client := s.setupLoopbackRequest()
	content := "binary\ncontent\n"
	req := &RequestData{
		ExpectedStatus: []int{http.StatusNoContent},
		ReqReader:      bytes.NewBufferString(content),
		ReqLength:      len(content),
	}
	err := client.BinaryRequest("POST", s.Server.URL, "", req, nil)
	c.Assert(err, IsNil)
	encoding := headers.Get("Transfer-Encoding")
	c.Check(encoding, Equals, "")
	length := headers.Get("Content-Length")
	c.Check(length, Equals, fmt.Sprintf("%d", len(content)))
	body, ok := <-bodyChan
	c.Assert(ok, Equals, true)
	c.Check(body, Equals, content)
}

func (s *HTTPClientTestSuite) TestJSONRequestSetsContentLength(c *C) {
	headers, bodyChan, client := s.setupLoopbackRequest()
	reqMap := map[string]string{"key": "value"}
	req := &RequestData{
		ExpectedStatus: []int{http.StatusNoContent},
		ReqValue:       reqMap,
	}
	err := client.JsonRequest("POST", s.Server.URL, "", req, nil)
	c.Assert(err, IsNil)
	encoding := headers.Get("Transfer-Encoding")
	c.Check(encoding, Equals, "")
	length := headers.Get("Content-Length")
	body, ok := <-bodyChan
	c.Assert(ok, Equals, true)
	c.Check(body, Not(Equals), "")
	c.Check(length, Equals, fmt.Sprintf("%d", len(body)))
}

func (s *HTTPClientTestSuite) TestBinaryRequestSetsToken(c *C) {
	headers, _, client := s.setupLoopbackRequest()
	req := &RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := client.BinaryRequest("POST", s.Server.URL, "token", req, nil)
	c.Assert(err, IsNil)
	agent := headers.Get("X-Auth-Token")
	c.Check(agent, Equals, "token")
}

func (s *HTTPClientTestSuite) TestJSONRequestSetsToken(c *C) {
	headers, _, client := s.setupLoopbackRequest()
	req := &RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := client.JsonRequest("POST", s.Server.URL, "token", req, nil)
	c.Assert(err, IsNil)
	agent := headers.Get("X-Auth-Token")
	c.Check(agent, Equals, "token")
}

func (s *HTTPClientTestSuite) TestHttpTransport(c *C) {
	transport := http.DefaultTransport.(*http.Transport)
	c.Assert(transport.DisableKeepAlives, Equals, true)
}

func (s *HTTPSClientTestSuite) TestDefaultClientRejectSelfSigned(c *C) {
	_, _, client := s.setupLoopbackRequest()
	req := &RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := client.BinaryRequest("POST", s.Server.URL, "", req, nil)
	c.Assert(err, NotNil)
	c.Check(err, ErrorMatches, "(.|\\n)*x509: certificate signed by unknown authority")
}

func (s *HTTPSClientTestSuite) TestInsecureClientAllowsSelfSigned(c *C) {
	headers, _, _ := s.setupLoopbackRequest()
	client := NewNonSSLValidating()
	req := &RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := client.BinaryRequest("POST", s.Server.URL, "", req, nil)
	c.Assert(err, IsNil)
	agent := headers.Get("User-Agent")
	c.Check(agent, Not(Equals), "")
	c.Check(agent, Equals, gooseAgent())
}
