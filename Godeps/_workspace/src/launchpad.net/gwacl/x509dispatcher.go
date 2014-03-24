// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "bytes"
    "fmt"
    "io/ioutil"
    "launchpad.net/gwacl/fork/http"
    . "launchpad.net/gwacl/logging"
    "net/url"
)

type X509Request struct {
    APIVersion  string
    Method      string
    URL         string
    Payload     []byte
    ContentType string
}

// newX509RequestGET initializes an X509Request for a GET.  You may still need
// to set further values.
func newX509RequestGET(url, apiVersion string) *X509Request {
    return &X509Request{
        Method:     "GET",
        URL:        url,
        APIVersion: apiVersion,
    }
}

// newX509RequestPOST initializes an X509Request for a POST.  You may still
// need to set further values.
func newX509RequestPOST(url, apiVersion string, payload []byte, contentType string) *X509Request {
    return &X509Request{
        Method:      "POST",
        URL:         url,
        APIVersion:  apiVersion,
        Payload:     payload,
        ContentType: contentType,
    }
}

// newX509RequestDELETE initializes an X509Request for a DELETE.
func newX509RequestDELETE(url, apiVersion string) *X509Request {
    return &X509Request{
        Method:     "DELETE",
        URL:        url,
        APIVersion: apiVersion,
    }
}

// newX509RequestPUT initializes an X509Request for a PUT.  You may still
// need to set further values.
func newX509RequestPUT(url, apiVersion string, payload []byte, contentType string) *X509Request {
    return &X509Request{
        Method:      "PUT",
        URL:         url,
        APIVersion:  apiVersion,
        Payload:     payload,
        ContentType: contentType,
    }
}

type x509Response struct {
    StatusCode int
    // TODO: What exactly do we get back?  How will we know its encoding?
    Body   []byte
    Header http.Header
}

func performX509Request(session *x509Session, request *X509Request) (*x509Response, error) {
    response := &x509Response{}

    Debugf("Request: %s %s", request.Method, request.URL)
    if len(request.Payload) > 0 {
        Debugf("Request body:\n%s", request.Payload)
    }

    bodyReader := ioutil.NopCloser(bytes.NewReader(request.Payload))
    httpRequest, err := http.NewRequest(request.Method, request.URL, bodyReader)
    if err != nil {
        return nil, err
    }
    httpRequest.Header.Set("Content-Type", request.ContentType)
    httpRequest.Header.Set("x-ms-version", request.APIVersion)
    retrier := session.retryPolicy.getForkedHttpRetrier(session.client)
    httpResponse, err := handleRequestRedirects(retrier, httpRequest)
    if err != nil {
        return nil, err
    }

    response.StatusCode = httpResponse.StatusCode
    response.Body, err = readAndClose(httpResponse.Body)
    if err != nil {
        return nil, err
    }
    response.Header = httpResponse.Header

    Debugf("Response: %d %s", response.StatusCode, http.StatusText(response.StatusCode))
    if response.Header != nil {
        buf := bytes.Buffer{}
        response.Header.Write(&buf)
        Debugf("Response headers:\n%s", buf.String())
    }
    if len(response.Body) > 0 {
        Debugf("Response body:\n%s", response.Body)
    }

    return response, nil
}

func handleRequestRedirects(retrier *forkedHttpRetrier, request *http.Request) (*http.Response, error) {
    const maxRedirects = 10
    // Handle temporary redirects.
    redirects := -1
    for {
        redirects++
        if redirects >= maxRedirects {
            return nil, fmt.Errorf("stopped after %d redirects", redirects)
        }
        response, err := retrier.RetryRequest(request)
        // For GET and HEAD, we cause the request execution
        // to return httpRedirectErr if a temporary redirect
        // is returned, and then deal with it here.
        if err, ok := err.(*url.Error); ok && err.Err == httpRedirectErr {
            request.URL, err.Err = url.Parse(err.URL)
            if err.Err != nil {
                return nil, err
            }
            continue
        }
        // For other methods, we must check the response code.
        if err == nil && response.StatusCode == http.StatusTemporaryRedirect {
            request.URL, err = response.Location()
            if err != nil {
                return nil, err
            }
            continue
        }
        return response, err
    }
}
