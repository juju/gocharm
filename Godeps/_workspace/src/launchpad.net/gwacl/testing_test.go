// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    . "launchpad.net/gocheck"
    "net/http"
)

type testTesting struct{}

var _ = Suite(&testTesting{})

func (*testTesting) TestNewTestStorageContextCreatesCompleteContext(c *C) {
    client := &http.Client{Transport: &TestTransport{}}
    context := NewTestStorageContext(client)
    context.Account = "myaccount"

    c.Check(context.Account, Equals, "myaccount")
    c.Check(context.getAccountURL(), Matches, ".*myaccount.*")
}

func (*testTesting) TestNewTestStorageContextWorksWithTransport(c *C) {
    errorMessage := "canned-error"
    error := fmt.Errorf(errorMessage)
    transport := &TestTransport{Error: error}
    client := &http.Client{Transport: transport}
    context := NewTestStorageContext(client)
    request := &ListContainersRequest{Marker: ""}
    _, err := context.ListContainers(request)
    c.Check(err, ErrorMatches, ".*"+errorMessage+".*")
}

func (*testTesting) TestNewDispatcherResponse(c *C) {
    body := []byte("test body")
    statusCode := http.StatusOK
    errorObject := fmt.Errorf("canned-error")
    dispatcherResponse := NewDispatcherResponse(body, statusCode, errorObject)
    c.Check(dispatcherResponse.errorObject, Equals, errorObject)
    c.Check(dispatcherResponse.response.Body, DeepEquals, body)
    c.Check(dispatcherResponse.response.StatusCode, Equals, statusCode)
}

func (*testTesting) TestPatchManagementAPIResponses(c *C) {
    response := NewDispatcherResponse([]byte("<Images></Images>"), http.StatusOK, nil)
    responses := []DispatcherResponse{response, response}
    requests := PatchManagementAPIResponses(responses)
    api := makeAPI(c)
    _, err := api.ListOSImages()
    c.Assert(err, IsNil)
    _, err = api.ListOSImages()
    c.Assert(err, IsNil)
    c.Assert(len(*requests), Equals, 2)
    c.Check((*requests)[0].URL, Equals, api.session.composeURL("services/images"))
    c.Check((*requests)[1].URL, Equals, api.session.composeURL("services/images"))
}
