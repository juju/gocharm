// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).
//
// Helpers for testing with x509 requests.  These help inject fake responses
// into the x509 request dispatcher.

package gwacl

import (
    "launchpad.net/gwacl/fork/http"
)

// rigRecordingDispatcher sets up a request dispatcher that records incoming
// requests by appending them to *record.  It returns the result of whatever
// dispatcher was already active.
// If you also want the dispatcher to return a particular result, rig it for
// that result first (using one of the other rig...Dispatcher functions) and
// then chain the recording dispatcher in front of it.
func rigRecordingDispatcher(record *[]*X509Request) {
    previousDispatcher := _X509Dispatcher
    _X509Dispatcher = func(session *x509Session, request *X509Request) (*x509Response, error) {
        *record = append(*record, request)
        return previousDispatcher(session, request)
    }
}

// rigFixedResponseDispatcher sets up a request dispatcher that always returns
// a prepared response.
func rigFixedResponseDispatcher(response *x509Response) {
    _X509Dispatcher = func(*x509Session, *X509Request) (*x509Response, error) {
        return response, nil
    }
}

// rigFailingDispatcher sets up a request dispatcher that returns a given
// error.
func rigFailingDispatcher(failure error) {
    _X509Dispatcher = func(*x509Session, *X509Request) (*x509Response, error) {
        return nil, failure
    }
}

type DispatcherResponse struct {
    response    *x509Response
    errorObject error
}

// rigPreparedResponseDispatcher sets up a request dispatcher that returns,
// for each consecutive request, the next of a series of prepared responses.
func rigPreparedResponseDispatcher(responses []DispatcherResponse) {
    index := 0
    _X509Dispatcher = func(*x509Session, *X509Request) (*x509Response, error) {
        response := responses[index]
        index += 1
        return response.response, response.errorObject
    }
}

// rigRecordingPreparedResponseDispatcher sets up a request dispatcher that
// returns, for each consecutive request, the next of a series of prepared
// responses, and records each request.
func rigRecordingPreparedResponseDispatcher(record *[]*X509Request, responses []DispatcherResponse) {
    index := 0
    _X509Dispatcher = func(session *x509Session, request *X509Request) (*x509Response, error) {
        *record = append(*record, request)
        response := responses[index]
        index += 1
        return response.response, response.errorObject
    }
}

// setUpDispatcher sets up a request dispatcher that:
// - records requests
// - returns empty responses
func setUpDispatcher(operationID string) *[]*X509Request {
    header := http.Header{}
    header.Set("X-Ms-Request-Id", operationID)
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte{},
        Header:     header,
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)
    return &recordedRequests
}
