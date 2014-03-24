// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/pem"
    "fmt"
    . "launchpad.net/gocheck"
    "math/big"
    "net/http"
    "os"
    "time"
)

// defaultManagement is the international management API for Azure.
// (Mainland China gets a different URL).
const defaultManagement = "https://management.core.windows.net/"

// x509DispatcherFixture records the current x509 dispatcher before a test,
// and restores it after.  This gives your test the freedom to replace the
// dispatcher with test doubles, using any of the rig*Dispatcher functions.
// Call the fixture's SetUpTest/TearDownTest methods before/after your test,
// or if you have no other setup/teardown methods, just embed the fixture in
// your test suite.
type x509DispatcherFixture struct {
    oldDispatcher func(*x509Session, *X509Request) (*x509Response, error)
}

func (suite *x509DispatcherFixture) SetUpTest(c *C) {
    // Record the original X509 dispatcher.  Will be restored at the end of
    // each test.
    suite.oldDispatcher = _X509Dispatcher
}

func (suite *x509DispatcherFixture) TearDownTest(c *C) {
    // Restore old dispatcher.
    _X509Dispatcher = suite.oldDispatcher
}

type x509SessionSuite struct {
    x509DispatcherFixture
}

var _ = Suite(&x509SessionSuite{})

// Create a cert and pem file in a temporary dir in /tmp and return the
// names of the files.  The caller is responsible for cleaning up the files.
func makeX509Certificate() (string, string) {
    // Code is shamelessly stolen from
    // http://golang.org/src/pkg/crypto/tls/generate_cert.go
    priv, err := rsa.GenerateKey(rand.Reader, 1024)
    if err != nil {
        panic(fmt.Errorf("Failed to generate rsa key: %v", err))
    }

    // Create a template for x509.CreateCertificate.
    now := time.Now()
    template := x509.Certificate{
        SerialNumber: new(big.Int).SetInt64(0),
        Subject: pkix.Name{
            CommonName:   "localhost",
            Organization: []string{"Bogocorp"},
        },
        NotBefore:    now.Add(-5 * time.Minute).UTC(),
        NotAfter:     now.AddDate(1, 0, 0).UTC(), // valid for 1 year.
        SubjectKeyId: []byte{1, 2, 3, 4},
        KeyUsage: x509.KeyUsageKeyEncipherment |
            x509.KeyUsageDigitalSignature,
    }

    // Create the certificate itself.
    derBytes, err := x509.CreateCertificate(
        rand.Reader, &template, &template, &priv.PublicKey, priv)
    if err != nil {
        panic(fmt.Errorf("Failed to generate x509 certificate: %v", err))
    }

    // Write the certificate file out.
    dirname := os.TempDir() + "/" + MakeRandomString(10)
    os.Mkdir(dirname, 0700)
    certFile := dirname + "/cert.pem"
    certOut, err := os.Create(certFile)
    if err != nil {
        panic(fmt.Errorf("Failed to create %s: %v", certFile, err))
    }
    pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
    certOut.Close()

    // Write the key file out.
    keyFile := dirname + "/key.pem"
    keyOut, err := os.OpenFile(
        keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
    if err != nil {
        panic(fmt.Errorf("Failed to create %s: %v", keyFile, err))
    }
    pem.Encode(
        keyOut,
        &pem.Block{
            Type:  "RSA PRIVATE KEY",
            Bytes: x509.MarshalPKCS1PrivateKey(priv)})
    keyOut.Close()

    return certFile, keyFile
}

func (suite *x509SessionSuite) TestNewX509Session(c *C) {
    session, err := newX509Session("subscriptionid", "", "China East", NoRetryPolicy)
    c.Assert(err, IsNil)
    c.Assert(session.baseURL, NotNil)
    c.Check(session.baseURL.String(), Equals, GetEndpoint("China East").ManagementAPI())
}

func (suite *x509SessionSuite) TestComposeURLComposesURLWithRelativePath(c *C) {
    const subscriptionID = "subscriptionid"
    const path = "foo/bar"
    session, err := newX509Session(subscriptionID, "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)

    url := session.composeURL(path)

    c.Check(url, Matches, defaultManagement+subscriptionID+"/"+path)
}

func (suite *x509SessionSuite) TestComposeURLRejectsAbsolutePath(c *C) {
    defer func() {
        err := recover()
        c.Assert(err, NotNil)
        c.Check(err, ErrorMatches, ".*absolute.*path.*")
    }()
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)

    // This panics because we're passing an absolute path.
    session.composeURL("/foo")
}

func (suite *x509SessionSuite) TestGetServerErrorProducesServerError(c *C) {
    msg := "huhwhat"
    status := http.StatusNotFound
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)

    err = session.getServerError(status, []byte{}, msg)
    c.Assert(err, NotNil)

    c.Check(err, ErrorMatches, ".*"+msg+".*")
    serverError := err.(*ServerError)
    c.Check(serverError.StatusCode(), Equals, status)
}

func (suite *x509SessionSuite) TestGetServerErrorLikes20x(c *C) {
    goodCodes := []int{
        http.StatusOK,
        http.StatusNoContent,
    }
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)

    for _, status := range goodCodes {
        c.Check(session.getServerError(status, []byte{}, ""), IsNil)
    }
}

func (suite *x509SessionSuite) TestGetServerReturnsErrorsForFailures(c *C) {
    badCodes := []int{
        http.StatusSwitchingProtocols,
        http.StatusBadRequest,
        http.StatusPaymentRequired,
        http.StatusForbidden,
        http.StatusGone,
        http.StatusInternalServerError,
        http.StatusNotImplemented,
    }
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)

    for _, status := range badCodes {
        c.Check(session.getServerError(status, []byte{}, ""), NotNil)
    }
}

func (suite *x509SessionSuite) TestGetIssuesRequest(c *C) {
    subscriptionID := "subscriptionID"
    uri := "resource"
    session, err := newX509Session(subscriptionID, "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    // Record incoming requests, and have them return a given reply.
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte("Response body"),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    version := "test-version"
    receivedResponse, err := session.get(uri, version)
    c.Assert(err, IsNil)

    c.Assert(len(recordedRequests), Equals, 1)
    request := recordedRequests[0]
    c.Check(request.URL, Equals, defaultManagement+subscriptionID+"/"+uri)
    c.Check(request.Method, Equals, "GET")
    c.Check(request.APIVersion, Equals, version)
    c.Check(*receivedResponse, DeepEquals, fixedResponse)
}

func (suite *x509SessionSuite) TestGetReportsClientSideError(c *C) {
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    msg := "could not dispatch request"
    rigFailingDispatcher(fmt.Errorf(msg))

    body, err := session.get("flop", "version")
    c.Assert(err, NotNil)

    c.Check(body, IsNil)
    c.Check(err, ErrorMatches, ".*"+msg+".*")
}

func (suite *x509SessionSuite) TestGetReportsServerSideError(c *C) {
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    fixedResponse := x509Response{
        StatusCode: http.StatusForbidden,
        Body:       []byte("Body"),
    }
    rigFixedResponseDispatcher(&fixedResponse)

    response, err := session.get("fail", "version")
    c.Assert(err, NotNil)

    serverError := err.(*ServerError)
    c.Check(serverError.StatusCode(), Equals, fixedResponse.StatusCode)
    c.Check(*response, DeepEquals, fixedResponse)
}

func (suite *x509SessionSuite) TestPostIssuesRequest(c *C) {
    subscriptionID := "subscriptionID"
    uri := "resource"
    version := "test-version"
    requestBody := []byte("Request body")
    requestContentType := "bogusContentType"
    session, err := newX509Session(subscriptionID, "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    // Record incoming requests, and have them return a given reply.
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte("Response body"),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    receivedResponse, err := session.post(uri, version, requestBody, requestContentType)
    c.Assert(err, IsNil)

    c.Assert(len(recordedRequests), Equals, 1)
    request := recordedRequests[0]
    c.Check(request.URL, Equals, defaultManagement+subscriptionID+"/"+uri)
    c.Check(request.Method, Equals, "POST")
    c.Check(request.APIVersion, Equals, version)
    c.Check(request.ContentType, Equals, requestContentType)
    c.Check(request.Payload, DeepEquals, requestBody)
    c.Check(*receivedResponse, DeepEquals, fixedResponse)
}

func (suite *x509SessionSuite) TestPostReportsClientSideError(c *C) {
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    msg := "could not dispatch request"
    rigFailingDispatcher(fmt.Errorf(msg))

    body, err := session.post("flop", "version", []byte("body"), "contentType")
    c.Assert(err, NotNil)

    c.Check(body, IsNil)
    c.Check(err, ErrorMatches, ".*"+msg+".*")
}

func (suite *x509SessionSuite) TestPostReportsServerSideError(c *C) {
    session, err := newX509Session("subscriptionid", "", "West US", NoRetryPolicy)
    fixedResponse := x509Response{
        StatusCode: http.StatusForbidden,
        Body:       []byte("Body"),
    }
    rigFixedResponseDispatcher(&fixedResponse)

    reponse, err := session.post("fail", "version", []byte("request body"), "contentType")
    c.Assert(err, NotNil)

    serverError := err.(*ServerError)
    c.Check(serverError.StatusCode(), Equals, fixedResponse.StatusCode)
    c.Check(*reponse, DeepEquals, fixedResponse)
}

func (suite *x509SessionSuite) TestDeleteIssuesRequest(c *C) {
    subscriptionID := "subscriptionID"
    uri := "resource"
    version := "test-version"
    session, err := newX509Session(subscriptionID, "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    // Record incoming requests, and have them return a given reply.
    fixedResponse := x509Response{StatusCode: http.StatusOK}
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    response, err := session.delete(uri, version)
    c.Assert(err, IsNil)

    c.Check(*response, DeepEquals, fixedResponse)
    c.Assert(len(recordedRequests), Equals, 1)
    request := recordedRequests[0]
    c.Check(request.URL, Equals, defaultManagement+subscriptionID+"/"+uri)
    c.Check(request.Method, Equals, "DELETE")
    c.Check(request.APIVersion, Equals, version)
}

func (suite *x509SessionSuite) TestPutIssuesRequest(c *C) {
    subscriptionID := "subscriptionID"
    uri := "resource"
    version := "test-version"
    requestBody := []byte("Request body")
    session, err := newX509Session(subscriptionID, "", "West US", NoRetryPolicy)
    c.Assert(err, IsNil)
    // Record incoming requests, and have them return a given reply.
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    _, err = session.put(uri, version, requestBody, "text/plain")
    c.Assert(err, IsNil)

    c.Assert(len(recordedRequests), Equals, 1)
    request := recordedRequests[0]
    c.Check(request.URL, Equals, defaultManagement+subscriptionID+"/"+uri)
    c.Check(request.Method, Equals, "PUT")
    c.Check(request.APIVersion, Equals, version)
    c.Check(request.Payload, DeepEquals, requestBody)
}
