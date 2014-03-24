package gwacl

import (
    "io/ioutil"
    . "launchpad.net/gocheck"
    "net/http"
    "net/http/httptest"
    "time"
)

type x509DispatcherSuite struct{}

var _ = Suite(&x509DispatcherSuite{})

type Request struct {
    *http.Request
    BodyContent []byte
}

// makeRecordingHTTPServer creates an http server (don't forget to Close() it when done)
// that serves at the given base URL, copies incoming requests into the given
// channel, and finally returns the given status code.  If body is not nil, it
// will be returned as the request body.
func makeRecordingHTTPServer(requests chan *Request, status int, body []byte, headers http.Header) *httptest.Server {
    var server *httptest.Server
    returnRequest := func(w http.ResponseWriter, r *http.Request) {
        // Capture all the request body content for later inspection.
        requestBody, err := ioutil.ReadAll(r.Body)
        if err != nil {
            panic(err)
        }
        requests <- &Request{r, requestBody}
        // Set a default Location so we can test redirect loops easily.
        w.Header().Set("Location", server.URL+r.URL.Path)
        for header, values := range headers {
            for _, value := range values {
                w.Header().Set(header, value)
            }
        }
        w.WriteHeader(status)
        if body != nil {
            w.Write(body)
        }
    }
    serveMux := http.NewServeMux()
    serveMux.HandleFunc("/", returnRequest)
    server = httptest.NewServer(serveMux)
    return server
}

func (*x509DispatcherSuite) TestGetRequestDoesHTTPGET(c *C) {
    httpRequests := make(chan *Request, 1)
    server := makeRecordingHTTPServer(httpRequests, http.StatusOK, nil, nil)
    defer server.Close()
    // No real certificate needed since we're testing on http, not https.
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    version := "test-version"
    request := newX509RequestGET(server.URL+path, version)

    response, err := performX509Request(session, request)
    c.Assert(err, IsNil)
    c.Assert(response.StatusCode, Equals, http.StatusOK)

    httpRequest := <-httpRequests
    c.Check(httpRequest.Method, Equals, "GET")
    c.Check(httpRequest.Header[http.CanonicalHeaderKey("X-Ms-Version")], DeepEquals, []string{version})
    c.Check(httpRequest.URL.String(), Equals, path)
    c.Check(httpRequest.BodyContent, HasLen, 0)
}

func (*x509DispatcherSuite) TestRetryPolicyCausesRequestsToBeRetried(c *C) {
    nbRetries := 2
    nbRequests := nbRetries + 1
    httpRequests := make(chan *Request, nbRequests)
    server := makeRecordingHTTPServer(httpRequests, http.StatusConflict, nil, nil)
    defer server.Close()
    // No real certificate needed since we're testing on http, not https.
    retryPolicy := RetryPolicy{NbRetries: nbRetries, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Nanosecond}
    session, err := newX509Session("subscriptionid", "", "West US", retryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    version := "test-version"
    request := newX509RequestGET(server.URL+path, version)

    response, err := performX509Request(session, request)
    c.Assert(err, IsNil)
    c.Assert(response.StatusCode, Equals, http.StatusConflict)

    // nbRequests request were performed.
    c.Check(httpRequests, HasLen, nbRequests)
}

func (*x509DispatcherSuite) TestPostRequestDoesHTTPPOST(c *C) {
    httpRequests := make(chan *Request, 1)
    requestBody := []byte{1, 2, 3}
    responseBody := []byte{4, 5, 6}
    requestContentType := "bogusContentType"
    server := makeRecordingHTTPServer(httpRequests, http.StatusOK, responseBody, nil)
    defer server.Close()
    // No real certificate needed since we're testing on http, not https.
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    version := "test-version"
    request := newX509RequestPOST(server.URL+path, version, requestBody, requestContentType)

    response, err := performX509Request(session, request)
    c.Assert(err, IsNil)
    c.Assert(response.StatusCode, Equals, http.StatusOK)
    c.Check(response.Body, DeepEquals, responseBody)

    httpRequest := <-httpRequests
    c.Check(httpRequest.Header[http.CanonicalHeaderKey("Content-Type")], DeepEquals, []string{requestContentType})
    c.Check(httpRequest.Header[http.CanonicalHeaderKey("X-Ms-Version")], DeepEquals, []string{request.APIVersion})
    c.Check(httpRequest.Method, Equals, "POST")
    c.Check(httpRequest.URL.String(), Equals, path)
    c.Check(httpRequest.BodyContent, DeepEquals, requestBody)
}

func (*x509DispatcherSuite) TestDeleteRequestDoesHTTPDELETE(c *C) {
    httpRequests := make(chan *Request, 1)
    server := makeRecordingHTTPServer(httpRequests, http.StatusOK, nil, nil)
    defer server.Close()
    // No real certificate needed since we're testing on http, not https.
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    version := "test-version"
    request := newX509RequestDELETE(server.URL+path, version)

    response, err := performX509Request(session, request)
    c.Assert(err, IsNil)
    c.Assert(response.StatusCode, Equals, http.StatusOK)

    httpRequest := <-httpRequests
    c.Check(httpRequest.Method, Equals, "DELETE")
    c.Check(httpRequest.Header[http.CanonicalHeaderKey("X-Ms-Version")], DeepEquals, []string{version})
    c.Check(httpRequest.URL.String(), Equals, path)
    c.Check(httpRequest.BodyContent, HasLen, 0)
}

func (*x509DispatcherSuite) TestPutRequestDoesHTTPPUT(c *C) {
    httpRequests := make(chan *Request, 1)
    requestBody := []byte{1, 2, 3}
    responseBody := []byte{4, 5, 6}
    server := makeRecordingHTTPServer(httpRequests, http.StatusOK, responseBody, nil)
    defer server.Close()
    // No real certificate needed since we're testing on http, not https.
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    version := "test-version"
    request := newX509RequestPUT(server.URL+path, version, requestBody, "application/octet-stream")

    response, err := performX509Request(session, request)
    c.Assert(err, IsNil)
    c.Assert(response.StatusCode, Equals, http.StatusOK)
    c.Check(response.Body, DeepEquals, responseBody)

    httpRequest := <-httpRequests
    c.Check(httpRequest.Method, Equals, "PUT")
    c.Check(httpRequest.Header[http.CanonicalHeaderKey("X-Ms-Version")], DeepEquals, []string{version})
    c.Check(httpRequest.URL.String(), Equals, path)
    c.Check(httpRequest.BodyContent, DeepEquals, requestBody)
}

func (*x509DispatcherSuite) TestRequestRegistersHeader(c *C) {
    customHeader := http.CanonicalHeaderKey("x-gwacl-test")
    customValue := []string{"present"}
    returnRequest := func(w http.ResponseWriter, r *http.Request) {
        w.Header()[customHeader] = customValue
        w.WriteHeader(http.StatusOK)
    }
    serveMux := http.NewServeMux()
    serveMux.HandleFunc("/", returnRequest)
    server := httptest.NewServer(serveMux)
    defer server.Close()
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    request := newX509RequestGET(server.URL+path, "testversion")

    response, err := performX509Request(session, request)
    c.Assert(err, IsNil)

    c.Check(response.Header[customHeader], DeepEquals, customValue)
}

func (*x509DispatcherSuite) TestRequestsFollowRedirects(c *C) {
    httpRequests := make(chan *Request, 2)
    serverConflict := makeRecordingHTTPServer(httpRequests, http.StatusConflict, nil, nil)
    defer serverConflict.Close()
    redirPath := "/else/where"
    responseHeaders := make(http.Header)
    responseHeaders.Set("Location", serverConflict.URL+redirPath)
    serverRedir := makeRecordingHTTPServer(httpRequests, http.StatusTemporaryRedirect, nil, responseHeaders)
    defer serverRedir.Close()
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    version := "test-version"

    // Test both GET and DELETE: DELETE does not normally
    // automatically follow redirects, however Azure requires
    // us to.
    requests := []*X509Request{
        newX509RequestGET(serverRedir.URL+path, version),
        newX509RequestDELETE(serverRedir.URL+path, version),
    }
    for _, request := range requests {
        response, err := performX509Request(session, request)
        c.Assert(err, IsNil)
        c.Assert(response.StatusCode, Equals, http.StatusConflict)
        c.Assert(httpRequests, HasLen, 2)
        c.Assert((<-httpRequests).URL.String(), Equals, path)
        c.Assert((<-httpRequests).URL.String(), Equals, redirPath)
    }
}

func (*x509DispatcherSuite) TestRequestsLimitRedirects(c *C) {
    httpRequests := make(chan *Request, 10)
    serverRedir := makeRecordingHTTPServer(httpRequests, http.StatusTemporaryRedirect, nil, nil)
    defer serverRedir.Close()
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    path := "/foo/bar"
    version := "test-version"
    request := newX509RequestGET(serverRedir.URL+path, version)

    response, err := performX509Request(session, request)
    c.Assert(err, ErrorMatches, "stopped after 10 redirects")
    c.Assert(response, IsNil)
    c.Assert(httpRequests, HasLen, 10)
    close(httpRequests)
    for req := range httpRequests {
        c.Assert(req.URL.String(), Equals, path)
    }
}
