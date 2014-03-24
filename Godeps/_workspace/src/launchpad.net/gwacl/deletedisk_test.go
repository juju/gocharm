// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    . "launchpad.net/gocheck"
    "net/http"
    "time"
)

type deleteDiskSuite struct{}

var _ = Suite(&deleteDiskSuite{})

// Real-world error messages and names.
const (
    diskInUseErrorTemplate = "BadRequest - A disk with name %s is currently in use by virtual machine gwaclrolemvo1yab running within hosted service gwacl623yosxtppsa9577xy5, deployment gwaclmachinewes4n64f. (http code 400: Bad Request)"
    diskName               = "gwacldiske5w7lkj"
    diskDoesNotExistError  = "DELETE request failed: ResourceNotFound - The disk with the specified name does not exist. (http code 404: Not Found)"
)

func (suite *deleteDiskSuite) TestIsInUseError(c *C) {
    var testValues = []struct {
        errorString    string
        diskName       string
        expectedResult bool
    }{
        {fmt.Sprintf(diskInUseErrorTemplate, diskName), diskName, true},
        {fmt.Sprintf(diskInUseErrorTemplate, diskName), "another-disk", false},
        {"unknown error", diskName, false},
        {diskDoesNotExistError, diskName, false},
    }
    for _, test := range testValues {
        c.Check(isInUseError(test.errorString, test.diskName), Equals, test.expectedResult)
    }
}

func (suite *deleteDiskSuite) TestIsDoneReturnsTrueIfNilError(c *C) {
    poller := diskDeletePoller{nil, "", false}
    randomResponse := x509Response{StatusCode: http.StatusAccepted}
    done, err := poller.isDone(&randomResponse, nil)
    c.Check(done, Equals, true)
    c.Check(err, IsNil)
}

func (suite *deleteDiskSuite) TestIsDoneReturnsFalseIfDiskInUseError(c *C) {
    diskName := "gwacldiske5w7lkj"
    diskInUseError := fmt.Errorf(diskInUseErrorTemplate, diskName)
    poller := diskDeletePoller{nil, diskName, false}
    done, err := poller.isDone(nil, diskInUseError)
    c.Check(done, Equals, false)
    c.Check(err, IsNil)
}

func (suite *deleteDiskSuite) TestIsDoneReturnsTrueIfAnotherError(c *C) {
    anotherError := fmt.Errorf("Unknown error")
    poller := diskDeletePoller{nil, "disk-name", false}
    done, err := poller.isDone(nil, anotherError)
    c.Check(done, Equals, true)
    c.Check(err, Equals, anotherError)
}

func (suite *deleteDiskSuite) TestPollCallsDeleteDisk(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    diskName := "gwacldiske5w7lkj"
    poller := diskDeletePoller{api, diskName, false}

    response, err := poller.poll()

    c.Assert(response, IsNil)
    c.Assert(err, IsNil)
    expectedURL := api.session.composeURL("services/disks/" + diskName)
    checkOneRequest(c, recordedRequests, expectedURL, "2012-08-01", nil, "DELETE")
}

func (suite *deleteDiskSuite) TestManagementAPIDeleteDiskPolls(c *C) {
    firstResponse := DispatcherResponse{
        response:    &x509Response{},
        errorObject: fmt.Errorf(diskInUseErrorTemplate, diskName)}
    secondResponse := DispatcherResponse{
        response:    &x509Response{StatusCode: http.StatusOK},
        errorObject: nil}
    responses := []DispatcherResponse{firstResponse, secondResponse}
    rigPreparedResponseDispatcher(responses)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    api := makeAPI(c)
    diskName := "gwacldiske5w7lkj"
    poller := diskDeletePoller{api, diskName, false}

    response, err := performPolling(poller, time.Nanosecond, time.Minute)

    c.Assert(response, IsNil)
    c.Assert(err, IsNil)
    expectedURL := api.session.composeURL("services/disks/" + diskName)
    c.Check(len(recordedRequests), Equals, 2)
    checkRequest(c, recordedRequests[0], expectedURL, "2012-08-01", nil, "DELETE")
    checkRequest(c, recordedRequests[1], expectedURL, "2012-08-01", nil, "DELETE")
}
