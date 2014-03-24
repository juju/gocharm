// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "net/http"
)

// NewTestStorageContext returns a StorageContext object built using
// the given *http.Client object.  It's meant to be used in the tests
// of other applications using gwacl to create a StorageContext that will
// interact with a fake client object.
func NewTestStorageContext(client *http.Client) *StorageContext {
    storageContext := &StorageContext{}
    storageContext.client = client
    storageContext.AzureEndpoint = "http://127.0.0.1/"
    return storageContext
}

// PatchManagementAPIResponses patches gwacl's ManagementAPI objects so that
// they can be used in tests.  Calling PatchManagementAPIResponses will make
// the ManagementAPI objects talk to a fake http server instead of talking to
// the Azure server and get the pre-canned responses from a fake http server.
// Use the returned X509Requests to inspect the issued requests.
// It's meant to be used in the tests of other applications using gwacl's
// ManagementAPI objects.
func PatchManagementAPIResponses(responses []DispatcherResponse) *[]*X509Request {
    rigPreparedResponseDispatcher(responses)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)
    return &recordedRequests
}

// NewDispatcherResponse creates a DispatcherResponse that can then be used by
// PatchManagementAPIResponses to simulate responses from the Azure server.
// It's meant to be used in the tests of other applications using gwacl's
// ManagementAPI objects.
func NewDispatcherResponse(body []byte, statusCode int, errorObject error) DispatcherResponse {
    return DispatcherResponse{
        response: &x509Response{
            Body:       body,
            StatusCode: statusCode,
        },
        errorObject: errorObject}
}

// MockingTransport is used as an http.Client.Transport for testing.  It
// records the sequence of requests, and returns a predetermined sequence of
// Responses and errors.
type MockingTransport struct {
    Exchanges     []*MockingTransportExchange
    ExchangeCount int
}

// MockingTransport implements the http.RoundTripper interface.
var _ http.RoundTripper = &MockingTransport{}

func (t *MockingTransport) AddExchange(response *http.Response, error error) {
    exchange := MockingTransportExchange{Response: response, Error: error}
    t.Exchanges = append(t.Exchanges, &exchange)
}

func (t *MockingTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
    exchange := t.Exchanges[t.ExchangeCount]
    t.ExchangeCount += 1
    exchange.Request = req
    return exchange.Response, exchange.Error
}

// MockingTransportExchange is a recording of a request and a response over
// HTTP.
type MockingTransportExchange struct {
    Request  *http.Request
    Response *http.Response
    Error    error
}
