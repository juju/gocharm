// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/xml"
    "fmt"
    . "launchpad.net/gocheck"
    "launchpad.net/gwacl/dedent"
    "net/http"
    "time"
)

type pollerSuite struct{}

var _ = Suite(&pollerSuite{})

func (suite *pollerSuite) makeAPI(c *C) *ManagementAPI {
    subscriptionId := "subscriptionId"
    subscriptionId = subscriptionId
    api, err := NewManagementAPI(subscriptionId, "", "West US")
    c.Assert(err, IsNil)
    return api
}

// testPoller is a struct which implements the Poller interface.  It records
// the number of calls to testPoller.Poll().
type testPoller struct {
    recordedPollCalls int
    notDoneCalls      int
    errorCalls        int
}

var testPollerResponse = x509Response{}
var testPollerError = fmt.Errorf("Test error")

// newTestPoller return a pointer to a testPoller object that will return
// false when IsDone() will be called 'notDoneCalls' number of times and that
// will error when Poll() will be called 'errorCalls' number of times.
func newTestPoller(notDoneCalls int, errorCalls int) *testPoller {
    return &testPoller{0, notDoneCalls, errorCalls}
}

func (poller *testPoller) poll() (*x509Response, error) {
    if poller.errorCalls > 0 {
        poller.errorCalls -= 1
        return nil, testPollerError
    }
    poller.recordedPollCalls += 1
    return &testPollerResponse, nil
}

func (poller *testPoller) isDone(response *x509Response, pollerError error) (bool, error) {
    if pollerError != nil {
        return true, pollerError
    }
    if poller.notDoneCalls == 0 {
        return true, nil
    }
    poller.notDoneCalls = poller.notDoneCalls - 1
    return false, nil
}

func (suite *pollerSuite) TestperformPollingPollsOnceImmediately(c *C) {
    poller := newTestPoller(0, 0)
    interval := time.Second * 10
    start := time.Now()
    response, err := performPolling(poller, interval, interval*2)
    c.Assert(err, Equals, nil)
    c.Assert(time.Since(start) < interval, Equals, true)
    c.Assert(response, DeepEquals, &testPollerResponse)
}

func (suite *pollerSuite) TestperformPollingReturnsError(c *C) {
    poller := newTestPoller(0, 1)
    response, err := performPolling(poller, time.Nanosecond, time.Minute)

    c.Assert(err, Equals, testPollerError)
    c.Assert(response, IsNil)
}

func (suite *pollerSuite) TestperformPollingTimesout(c *C) {
    poller := newTestPoller(10, 0)
    response, err := performPolling(poller, time.Millisecond, 5*time.Millisecond)

    c.Assert(response, IsNil)
    c.Check(err, ErrorMatches, ".*polling timed out waiting for an asynchronous operation.*")
}

func (suite *pollerSuite) TestperformPollingRetries(c *C) {
    poller := newTestPoller(2, 0)
    response, err := performPolling(poller, time.Nanosecond, time.Minute)

    c.Assert(err, IsNil)
    c.Assert(response, DeepEquals, &testPollerResponse)
    // Poll() has been called 3 times: two calls for which IsDone() returned
    // false and one for which IsDone() return true.
    c.Assert(poller.recordedPollCalls, Equals, 3)
}

func (suite *pollerSuite) TestnewOperationPoller(c *C) {
    api := suite.makeAPI(c)
    operationID := "operationID"

    poller := newOperationPoller(api, operationID)

    operationPollerInstance := poller.(operationPoller)
    c.Check(operationPollerInstance.api, Equals, api)
    c.Check(operationPollerInstance.operationID, Equals, operationID)
}

func (suite *pollerSuite) TestOperationPollerPoll(c *C) {
    api := suite.makeAPI(c)
    operationID := "operationID"
    poller := newOperationPoller(api, operationID)
    recordedRequests := setUpDispatcher("operationID")

    _, err := poller.poll()

    c.Assert(err, IsNil)
    expectedURL := defaultManagement + api.session.subscriptionId + "/operations/" + operationID
    checkOneRequest(c, recordedRequests, expectedURL, "2009-10-01", nil, "GET")
}

var operationXMLTemplate = dedent.Dedent(`
<?xml version="1.0" encoding="utf-8"?>
  <Operation xmlns="http://schemas.microsoft.com/windowsazure">
    <ID>bogus-request-id</ID>
    <Status>%s</Status>
  </Operation>
`)

func (suite *pollerSuite) TestOperationPollerIsDoneReturnsTrueIfOperationDone(c *C) {
    poller := newOperationPoller(suite.makeAPI(c), "operationID")
    operationStatuses := []string{"Succeeded", "Failed"}
    for _, operationStatus := range operationStatuses {
        body := fmt.Sprintf(operationXMLTemplate, operationStatus)
        response := x509Response{
            Body:       []byte(body),
            StatusCode: http.StatusOK,
        }

        isDone, err := poller.isDone(&response, nil)
        c.Assert(err, IsNil)
        c.Assert(isDone, Equals, true)
    }
}

func (suite *pollerSuite) TestOperationPollerIsDoneReturnsFalse(c *C) {
    poller := newOperationPoller(suite.makeAPI(c), "operationID")
    notDoneResponses := []x509Response{
        // 'InProgress' response.
        {
            Body:       []byte(fmt.Sprintf(operationXMLTemplate, "InProgress")),
            StatusCode: http.StatusOK,
        },
        // Error statuses.
        {StatusCode: http.StatusNotFound},
        {StatusCode: http.StatusBadRequest},
        {StatusCode: http.StatusInternalServerError},
    }
    for _, response := range notDoneResponses {
        isDone, _ := poller.isDone(&response, nil)
        c.Assert(isDone, Equals, false)
    }
}

func (suite *pollerSuite) TestOperationPollerIsDoneReturnsXMLParsingError(c *C) {
    poller := newOperationPoller(suite.makeAPI(c), "operationID")
    // Invalid XML content.
    response := x509Response{
        Body:       []byte("><invalid XML"),
        StatusCode: http.StatusOK,
    }
    _, err := poller.isDone(&response, nil)
    c.Assert(err, NotNil)
    c.Check(err, FitsTypeOf, new(xml.SyntaxError))
}

func (suite *pollerSuite) TestStartOperationPollingRetries(c *C) {
    // This is an end-to-end test of the operation poller; there is a certain
    // amount of duplication with the unit tests for performPolling() but
    // it's probably worth it to thoroughly test performOperationPolling().

    // Fake 2 responses in sequence: a 'InProgress' response and then a
    // 'Succeeded' response.
    firstResponse := DispatcherResponse{
        response: &x509Response{
            Body:       []byte(fmt.Sprintf(operationXMLTemplate, "InProgress")),
            StatusCode: http.StatusOK,
        },
        errorObject: nil}
    secondResponse := DispatcherResponse{
        response: &x509Response{
            Body:       []byte(fmt.Sprintf(operationXMLTemplate, "Succeeded")),
            StatusCode: http.StatusOK,
        },
        errorObject: nil}
    responses := []DispatcherResponse{firstResponse, secondResponse}
    rigPreparedResponseDispatcher(responses)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    // Setup poller and start it.
    operationID := "operationID"
    poller := newOperationPoller(suite.makeAPI(c), operationID)
    response, err := performPolling(poller, time.Nanosecond, time.Minute)

    c.Assert(err, IsNil)
    c.Assert(response, DeepEquals, secondResponse.response)
    operationPollerInstance := poller.(operationPoller)
    expectedURL := defaultManagement + operationPollerInstance.api.session.subscriptionId + "/operations/" + operationID
    c.Assert(len(recordedRequests), Equals, 2)
    checkRequest(c, recordedRequests[0], expectedURL, "2009-10-01", nil, "GET")
    checkRequest(c, recordedRequests[1], expectedURL, "2009-10-01", nil, "GET")
}
