package httpsuite

// This package provides an HTTPSuite infrastructure that lets you bring up an
// HTTP server. The server will handle requests based on whatever Handlers are
// attached to HTTPSuite.Mux. This Mux is reset after every test case, and the
// server is shut down at the end of the test suite.

import (
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

var _ = Suite(&HTTPSuite{})

type HTTPSuite struct {
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
	UseTLS     bool
}

func (s *HTTPSuite) SetUpSuite(c *C) {
	// fmt.Printf("Starting New Server\n")
	if s.UseTLS {
		s.Server = httptest.NewTLSServer(nil)
	} else {
		s.Server = httptest.NewServer(nil)
	}
}

func (s *HTTPSuite) SetUpTest(c *C) {
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux
}

func (s *HTTPSuite) TearDownTest(c *C) {
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
}

func (s *HTTPSuite) TearDownSuite(c *C) {
	if s.Server != nil {
		// fmt.Printf("Stopping Server\n")
		s.Server.Close()
	}
}
