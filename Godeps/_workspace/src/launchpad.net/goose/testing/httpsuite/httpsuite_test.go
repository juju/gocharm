package httpsuite

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/url"
	"reflect"
	"testing"
)

type HTTPTestSuite struct {
	HTTPSuite
}

type HTTPSTestSuite struct {
	HTTPSuite
}

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&HTTPTestSuite{})
var _ = Suite(&HTTPSTestSuite{HTTPSuite{UseTLS: true}})

type HelloHandler struct{}

func (h *HelloHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(200)
	w.Write([]byte("Hello World\n"))
}

func (s *HTTPTestSuite) TestHelloWorld(c *C) {
	s.Mux.Handle("/", &HelloHandler{})
	// fmt.Printf("Running HelloWorld\n")
	response, err := http.Get(s.Server.URL)
	c.Check(err, IsNil)
	content, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	c.Check(err, IsNil)
	c.Check(response.Status, Equals, "200 OK")
	c.Check(response.StatusCode, Equals, 200)
	c.Check(string(content), Equals, "Hello World\n")
}

func (s *HTTPSTestSuite) TestHelloWorldWithTLS(c *C) {
	s.Mux.Handle("/", &HelloHandler{})
	c.Check(s.Server.URL[:8], Equals, "https://")
	response, err := http.Get(s.Server.URL)
	// Default http.Get fails because the cert is self-signed
	c.Assert(err, NotNil)
	c.Assert(reflect.TypeOf(err.(*url.Error).Err), Equals, reflect.TypeOf(x509.UnknownAuthorityError{}))
	// Connect again with a Client that doesn't validate the cert
	insecureClient := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	response, err = insecureClient.Get(s.Server.URL)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	c.Check(err, IsNil)
	c.Check(response.Status, Equals, "200 OK")
	c.Check(response.StatusCode, Equals, 200)
	c.Check(string(content), Equals, "Hello World\n")
}
