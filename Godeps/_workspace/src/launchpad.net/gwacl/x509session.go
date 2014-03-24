// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "errors"
    "fmt"
    "launchpad.net/gwacl/fork/http"
    "launchpad.net/gwacl/fork/tls"
    "net/url"
    "strings"
)

type x509Session struct {
    subscriptionId string
    certFile       string
    client         *http.Client
    baseURL        *url.URL
    retryPolicy    RetryPolicy
}

// httpRedirectErr is a unique error used to prevent
// net/http from automatically following redirects.
// See commentary on CheckRedirect in newX509Session.
var httpRedirectErr = errors.New("redirect")

// newX509Session creates and returns a new x509Session based on credentials
// and X509 certificate files.
// For testing purposes, certFile can be passed as the empty string and it
// will be ignored.
func newX509Session(subscriptionId, certFile, location string, retryPolicy RetryPolicy) (*x509Session, error) {
    certs := []tls.Certificate{}
    if certFile != "" {
        //
        cert, err := tls.LoadX509KeyPair(certFile, certFile)
        if err != nil {
            return nil, err
        }
        certs = append(certs, cert)
    }
    client := http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                Certificates: certs,
            },
            // See https://code.google.com/p/go/issues/detail?id=4677
            // We need to force the connection to close each time so that we don't
            // hit the above Go bug.
            DisableKeepAlives: true,
        },
        // See https://code.google.com/p/go/issues/detail?id=4800
        // We need to follow temporary redirects (307s), while
        // retaining headers. We also need to follow redirects
        // for POST and DELETE automatically.
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return httpRedirectErr
        },
    }

    endpointURL := GetEndpoint(location).ManagementAPI()
    baseURL, err := url.Parse(endpointURL)
    if err != nil {
        panic(fmt.Errorf("cannot parse Azure endpoint URL '%s' - %v", endpointURL, err))
    }

    session := x509Session{
        subscriptionId: subscriptionId,
        certFile:       certFile,
        client:         &client,
        baseURL:        baseURL,
        retryPolicy:    retryPolicy,
    }
    return &session, nil
}

// composeURL puts together a URL for an item on the Azure API based on
// the starting point used by the session, and a given relative path from
// there.
func (session *x509Session) composeURL(path string) string {
    if strings.HasPrefix(path, "/") {
        panic(fmt.Errorf("got absolute API path '%s' instead of relative one", path))
    }
    escapedID := url.QueryEscape(session.subscriptionId)
    pathURL, err := url.Parse(escapedID + "/" + path)
    if err != nil {
        panic(err)
    }
    return session.baseURL.ResolveReference(pathURL).String()
}

// _X509Dispatcher is the function used to dispatch requests.  We call the
// function through this pointer, not directly, so that tests can inject
// fakes.
var _X509Dispatcher = performX509Request

// getServerError returns a ServerError matching the given server response
// status, or nil if the server response indicates success.
func (session *x509Session) getServerError(status int, body []byte, description string) error {
    if status < http.StatusOK || status >= http.StatusMultipleChoices {
        return newHTTPError(status, body, description)
    }
    return nil
}

// get performs a GET request to the Azure management API.
// It returns the response body and/or an error.  If the error is a
// ServerError, the returned body will be the one received from the server.
// For any other kind of error, the returned body will be nil.
func (session *x509Session) get(path, apiVersion string) (*x509Response, error) {
    request := newX509RequestGET(session.composeURL(path), apiVersion)
    response, err := _X509Dispatcher(session, request)
    if err != nil {
        return nil, err
    }
    err = session.getServerError(response.StatusCode, response.Body, "GET request failed")
    return response, err
}

// post performs a POST request to the Azure management API.
// It returns the response body and/or an error.  If the error is a
// ServerError, the returned body will be the one received from the server.
// For any other kind of error, the returned body will be nil.
// Be aware that Azure may perform POST operations asynchronously.  If you are
// not sure, call blockUntilCompleted() on the response.
func (session *x509Session) post(path, apiVersion string, body []byte, contentType string) (*x509Response, error) {
    request := newX509RequestPOST(session.composeURL(path), apiVersion, body, contentType)
    response, err := _X509Dispatcher(session, request)
    if err != nil {
        return nil, err
    }
    err = session.getServerError(response.StatusCode, response.Body, "POST request failed")
    return response, err
}

// delete performs a DELETE request to the Azure management API.
// Be aware that Azure may perform DELETE operations asynchronously.  If you
// are not sure, call blockUntilCompleted() on the response.
func (session *x509Session) delete(path, apiVersion string) (*x509Response, error) {
    request := newX509RequestDELETE(session.composeURL(path), apiVersion)
    response, err := _X509Dispatcher(session, request)
    if err != nil {
        return response, err
    }
    err = session.getServerError(response.StatusCode, response.Body, "DELETE request failed")
    return response, err
}

// put performs a PUT request to the Azure management API.
// Be aware that Azure may perform PUT operations asynchronously.  If you are
// not sure, call blockUntilCompleted() on the response.
func (session *x509Session) put(path, apiVersion string, body []byte, contentType string) (*x509Response, error) {
    request := newX509RequestPUT(session.composeURL(path), apiVersion, body, contentType)
    response, err := _X509Dispatcher(session, request)
    if err != nil {
        return nil, err
    }
    err = session.getServerError(response.StatusCode, response.Body, "PUT request failed")
    return response, err
}
