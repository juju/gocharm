// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).
//
// Test helpers for dealing with http requests through the http package.

package gwacl

import (
    "encoding/base64"
    "fmt"
    "io"
    "io/ioutil"
    "net/http"
    "strings"
)

// TestTransport is used as an http.Client.Transport for testing.  It records
// the latest request, and returns a predetermined Response and error.
type TestTransport struct {
    Request  *http.Request
    Response *http.Response
    Error    error
}

// TestTransport implements the http.RoundTripper interface.
var _ http.RoundTripper = &TestTransport{}

func (t *TestTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
    t.Request = req
    return t.Response, t.Error
}

// makeFakeCreatedResponse returns an HTTP response with the Created status.
func makeFakeCreatedResponse() *http.Response {
    return &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusCreated),
        StatusCode: http.StatusCreated,
        Body:       Empty,
    }
}

// makeResponseBody creates an http response body containing the given string.
// Use this to initialize an http.Response.Body with a given string, without
// having to care about the type details.
func makeResponseBody(content string) io.ReadCloser {
    return ioutil.NopCloser(strings.NewReader(content))
}

// Convenience factory to create a StorageContext with a random name and
// random base64-encoded key.
func makeStorageContext(transport http.RoundTripper) *StorageContext {
    context := &StorageContext{
        Account:       MakeRandomString(10),
        Key:           base64.StdEncoding.EncodeToString(MakeRandomByteSlice(10)),
        AzureEndpoint: APIEndpoint("http://" + MakeRandomString(5) + ".example.com/"),
    }
    context.client = &http.Client{Transport: transport}
    return context
}
