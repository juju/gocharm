// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/base64"
    "encoding/xml"
    "errors"
    "fmt"
    "io/ioutil"
    . "launchpad.net/gocheck"
    "launchpad.net/gwacl/dedent"
    "launchpad.net/gwacl/fork/http"
    "net/url"
    "strings"
    "time"
)

type managementBaseAPISuite struct {
    x509DispatcherFixture
    oldPollerInterval time.Duration
}

var _ = Suite(&managementBaseAPISuite{})

func makeX509ResponseWithOperationHeader(operationID string) *x509Response {
    header := http.Header{}
    header.Set(operationIDHeaderName, operationID)
    response := x509Response{
        StatusCode: http.StatusAccepted,
        Header:     header,
    }
    return &response
}

func makeAPI(c *C) *ManagementAPI {
    subscriptionId := "subscriptionId"
    api, err := NewManagementAPI(subscriptionId, "", "West US")
    c.Assert(err, IsNil)
    // Polling is disabled by default.
    api.PollerInterval = 0
    return api
}

func (suite *managementBaseAPISuite) TestGetOperationIDExtractsHeader(c *C) {
    operationID := "operationID"
    response := makeX509ResponseWithOperationHeader(operationID)

    returnedOperationID, err := getOperationID(response)
    c.Assert(err, IsNil)
    c.Check(returnedOperationID, Equals, operationID)
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedSucceedsOnSyncSuccess(c *C) {
    // Set of expected error returns for success statuses.
    // (They all map to nil.  It makes it easier further down to report
    // unexpected errors in a helpful way).
    expectedErrors := map[int]error{
        http.StatusOK:        nil,
        http.StatusCreated:   nil,
        http.StatusNoContent: nil,
    }
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond

    receivedErrors := make(map[int]error)
    for status := range expectedErrors {
        err := api.blockUntilCompleted(&x509Response{StatusCode: status})
        receivedErrors[status] = err
    }

    c.Check(receivedErrors, DeepEquals, expectedErrors)
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedReturnsHTTPErrorOnSyncFailure(c *C) {
    // Set of failure statuses, and whether they're supposed to return
    // HTTPError.
    // (They all map to true.  It makes it easier further down to report
    // failures in a helpful way).
    expectedErrors := map[int]bool{
        http.StatusBadRequest:          true,
        http.StatusUnauthorized:        true,
        http.StatusForbidden:           true,
        http.StatusNotFound:            true,
        http.StatusConflict:            true,
        http.StatusInternalServerError: true,
        http.StatusNotImplemented:      true,
    }
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond

    receivedErrors := make(map[int]bool)
    for status := range expectedErrors {
        err := api.blockUntilCompleted(&x509Response{StatusCode: status})
        _, ok := err.(HTTPError)
        receivedErrors[status] = ok
    }

    c.Check(receivedErrors, DeepEquals, expectedErrors)
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedPollsOnAsyncOperation(c *C) {
    const operationID = "async-operation-id"
    // We set the dispatcher up for failure, and prove that
    // blockUntilCompleted() makes a polling request.
    failure := errors.New("Simulated failure")
    rigFailingDispatcher(failure)
    requests := make([]*X509Request, 0)
    rigRecordingDispatcher(&requests)
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond
    accepted := makeX509ResponseWithOperationHeader(operationID)

    err := api.blockUntilCompleted(accepted)

    c.Check(err, Equals, failure)
    c.Assert(len(requests), Equals, 1)
    requestURL := requests[0].URL
    urlParts := strings.Split(requestURL, "/")
    polledOperationID := urlParts[len(urlParts)-1]
    c.Check(polledOperationID, Equals, operationID)
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedErrorsIfAsyncOperationFails(c *C) {
    response := DispatcherResponse{
        response: &x509Response{
            Body:       []byte(fmt.Sprintf(operationXMLTemplate, "Failed")),
            StatusCode: http.StatusOK,
        },
        errorObject: nil}
    responses := []DispatcherResponse{response}
    rigPreparedResponseDispatcher(responses)
    operationID := "operationID"
    operationResponse := makeX509ResponseWithOperationHeader(operationID)
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond

    err := api.blockUntilCompleted(operationResponse)
    c.Check(err, ErrorMatches, ".*asynchronous operation failed.*")
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedErrorsOnInvalidXML(c *C) {
    response := DispatcherResponse{
        response: &x509Response{
            Body:       []byte(">invalidXML<"),
            StatusCode: http.StatusOK,
        },
        errorObject: nil}
    responses := []DispatcherResponse{response}
    rigPreparedResponseDispatcher(responses)
    operationID := "operationID"
    operationResponse := makeX509ResponseWithOperationHeader(operationID)
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond

    err := api.blockUntilCompleted(operationResponse)
    c.Check(err, FitsTypeOf, new(xml.SyntaxError))
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedErrorsIfPollingFails(c *C) {
    response := DispatcherResponse{
        response:    &x509Response{},
        errorObject: fmt.Errorf("unexpected error")}
    responses := []DispatcherResponse{response}
    rigPreparedResponseDispatcher(responses)
    operationID := "operationID"
    operationResponse := makeX509ResponseWithOperationHeader(operationID)
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond

    err := api.blockUntilCompleted(operationResponse)
    c.Check(err, ErrorMatches, ".*unexpected error.*")
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedErrorsCanTimeout(c *C) {
    response := &x509Response{
        Body:       []byte(fmt.Sprintf(operationXMLTemplate, InProgressOperationStatus)),
        StatusCode: http.StatusOK,
    }
    rigFixedResponseDispatcher(response)
    operationID := "operationID"
    operationResponse := makeX509ResponseWithOperationHeader(operationID)
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond
    api.PollerTimeout = 2 * time.Nanosecond

    err := api.blockUntilCompleted(operationResponse)
    c.Check(err, ErrorMatches, ".*polling timed out.*")
}

func (suite *managementBaseAPISuite) TestBlockUntilCompletedSucceedsIfAsyncOperationSucceeds(c *C) {
    response := DispatcherResponse{
        response: &x509Response{
            Body:       []byte(fmt.Sprintf(operationXMLTemplate, "Succeeded")),
            StatusCode: http.StatusOK,
        },
        errorObject: nil}
    responses := []DispatcherResponse{response}
    rigPreparedResponseDispatcher(responses)
    operationID := "operationID"
    operationResponse := makeX509ResponseWithOperationHeader(operationID)
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond

    err := api.blockUntilCompleted(operationResponse)
    c.Assert(err, IsNil)
}

var testCert = dedent.Dedent(`
    -----BEGIN PRIVATE KEY-----
    MIIBCgIBADANBgkqhkiG9w0BAQEFAASB9TCB8gIBAAIxAKQGQxP1i0VfCWn4KmMP
    taUFn8sMBKjP/9vHnUYdZRvvmoJCA1C6arBUDp8s2DNX+QIDAQABAjBLRqhwN4dU
    LfqHDKJ/Vg1aD8u3Buv4gYRBxdFR5PveyqHSt5eJ4g/x/4ndsvr2OqUCGQDNfNlD
    zxHCiEAwZZAPaAkn8jDkFupTljcCGQDMWCujiVZ1NNuBD/N32Yt8P9JDiNzZa08C
    GBW7VXLxbExpgnhb1V97vjQmTfthXQjYAwIYSTEjoFXm4+Bk5xuBh2IidgSeGZaC
    FFY9AhkAsteo31cyQw2xJ80SWrmsIw+ps7Cvt5W9
    -----END PRIVATE KEY-----
    -----BEGIN CERTIFICATE-----
    MIIBDzCByqADAgECAgkAgIBb3+lSwzEwDQYJKoZIhvcNAQEFBQAwFTETMBEGA1UE
    AxQKQEhvc3ROYW1lQDAeFw0xMzA3MTkxNjA1NTRaFw0yMzA3MTcxNjA1NTRaMBUx
    EzARBgNVBAMUCkBIb3N0TmFtZUAwTDANBgkqhkiG9w0BAQEFAAM7ADA4AjEApAZD
    E/WLRV8JafgqYw+1pQWfywwEqM//28edRh1lG++agkIDULpqsFQOnyzYM1f5AgMB
    AAGjDTALMAkGA1UdEwQCMAAwDQYJKoZIhvcNAQEFBQADMQABKfn08tKfzzqMMD2w
    PI2fs3bw5bRH8tmGjrsJeEdp9crCBS8I3hKcxCkTTRTowdY=
    -----END CERTIFICATE-----
    `[1:])

func (suite *managementBaseAPISuite) TestNewManagementAPI(c *C) {
    subscriptionId := "subscriptionId"
    certDir := c.MkDir()
    certFile := certDir + "/cert.pem"
    err := ioutil.WriteFile(certFile, []byte(testCert), 0600)
    c.Assert(err, IsNil)

    api, err := NewManagementAPI(subscriptionId, certFile, "West US")
    c.Assert(err, IsNil)

    c.Assert(api.session.subscriptionId, DeepEquals, subscriptionId)
    c.Assert(api.session.certFile, DeepEquals, certFile)
    c.Assert(api.session.retryPolicy, DeepEquals, NoRetryPolicy)
}

func (suite *managementBaseAPISuite) TestNewManagementAPIWithRetryPolicy(c *C) {
    subscriptionId := "subscriptionId"
    certDir := c.MkDir()
    certFile := certDir + "/cert.pem"
    err := ioutil.WriteFile(certFile, []byte(testCert), 0600)
    c.Assert(err, IsNil)
    retryPolicy := RetryPolicy{NbRetries: 5, HttpStatusCodes: []int{409}, Delay: time.Minute}

    api, err := NewManagementAPIWithRetryPolicy(subscriptionId, certFile, "West US", retryPolicy)
    c.Assert(err, IsNil)

    c.Assert(api.session.subscriptionId, DeepEquals, subscriptionId)
    c.Assert(api.session.certFile, DeepEquals, certFile)
    c.Assert(api.session.retryPolicy, DeepEquals, retryPolicy)
    c.Assert(api.GetRetryPolicy(), DeepEquals, retryPolicy)
}

func (suite *managementBaseAPISuite) TestNewManagementAPISetsDefaultPollerInterval(c *C) {
    api, err := NewManagementAPI("subscriptionId", "", "West US")
    c.Assert(err, IsNil)

    c.Assert(api.PollerInterval, Equals, DefaultPollerInterval)
}

func (suite *managementBaseAPISuite) TestNewManagementAPISetsDefaultPollerTimeout(c *C) {
    api, err := NewManagementAPI("subscriptionId", "", "West US")
    c.Assert(err, IsNil)

    c.Assert(api.PollerTimeout, Equals, DefaultPollerTimeout)
}

// setUpDispatcher sets up a request dispatcher that:
// - records requests
// - returns empty responses
func (suite *managementBaseAPISuite) setUpDispatcher() *[]*X509Request {
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte{},
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)
    return &recordedRequests
}

// checkOneRequest asserts that the given slice contains one request, with the
// given characteristics.
func checkOneRequest(c *C, recordedRequests *[]*X509Request, URL, version string, payload []byte, Method string) {
    requests := *recordedRequests
    c.Assert(len(requests), Equals, 1)
    request := requests[0]
    checkRequest(c, request, URL, version, payload, Method)
}

func checkRequest(c *C, request *X509Request, URL, version string, payload []byte, Method string) {
    c.Check(request.URL, Equals, URL)
    c.Check(
        strings.TrimSpace(string(request.Payload)),
        Equals,
        strings.TrimSpace(string(payload)))
    c.Check(request.Method, Equals, Method)
    c.Check(request.APIVersion, Equals, version)
}

func (suite *managementBaseAPISuite) TestGetOperationFailsIfNoHeader(c *C) {
    response := x509Response{
        StatusCode: http.StatusOK,
    }

    _, err := getOperationID(&response)
    c.Check(err, ErrorMatches, ".*no operation header.*")
}

func (suite *managementBaseAPISuite) TestListOSImagesRequestsListing(c *C) {
    api := makeAPI(c)
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusOK, Body: []byte("<Images></Images>")})
    requests := make([]*X509Request, 0)
    rigRecordingDispatcher(&requests)

    _, err := api.ListOSImages()
    c.Assert(err, IsNil)

    c.Assert(len(requests), Equals, 1)
    c.Check(requests[0].URL, Equals, api.session.composeURL("services/images"))
}

func (suite *managementBaseAPISuite) TestListOSImagesReturnsImages(c *C) {
    expectedImage := OSImage{
        LogicalSizeInGB:   199.0,
        Label:             MakeRandomString(10),
        MediaLink:         "http://storage.example.com/" + MakeRandomString(10),
        Name:              MakeRandomString(10),
        OS:                "Linux",
        EULA:              "http://eula.example.com/" + MakeRandomString(10),
        Description:       MakeRandomString(10),
        RecommendedVMSize: "Medium",
    }
    body := fmt.Sprintf(`
        <Images>
            <OSImage>
                <LogicalSizeInGB>%f</LogicalSizeInGB>
                <Label>%s</Label>
                <MediaLink>%s</MediaLink>
                <Name>%s</Name>
                <OS>%s</OS>
                <Eula>%s</Eula>
                <Description>%s</Description>
                <RecommendedVMSize>%s</RecommendedVMSize>
            </OSImage>
        </Images>
    `,
        expectedImage.LogicalSizeInGB, expectedImage.Label,
        expectedImage.MediaLink, expectedImage.Name, expectedImage.OS,
        expectedImage.EULA, expectedImage.Description,
        expectedImage.RecommendedVMSize)
    api := makeAPI(c)
    rigFixedResponseDispatcher(&x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(body),
    })

    listing, err := api.ListOSImages()
    c.Assert(err, IsNil)

    c.Assert(len(listing.Images), Equals, 1)
    c.Check(listing.Images[0], DeepEquals, expectedImage)
}

func (suite *managementBaseAPISuite) TestListOSImagesPreservesOrdering(c *C) {
    imageNames := []string{
        MakeRandomString(5),
        MakeRandomString(5),
        MakeRandomString(5),
    }
    body := fmt.Sprintf(`
            <Images>
                <OSImage><Name>%s</Name></OSImage>
                <OSImage><Name>%s</Name></OSImage>
                <OSImage><Name>%s</Name></OSImage>
            </Images>
        `,
        imageNames[0], imageNames[1], imageNames[2])
    api := makeAPI(c)
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusOK, Body: []byte(body)})

    listing, err := api.ListOSImages()
    c.Assert(err, IsNil)

    c.Assert(len(listing.Images), Equals, 3)
    receivedNames := make([]string, 3)
    for index := range listing.Images {
        receivedNames[index] = listing.Images[index].Name
    }
    c.Check(receivedNames, DeepEquals, imageNames)
}

func (suite *managementBaseAPISuite) TestListHostedServices(c *C) {
    api := makeAPI(c)
    url := MakeRandomString(10)
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(makeHostedServiceDescriptorList(url)),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    descriptors, err := api.ListHostedServices()

    c.Assert(err, IsNil)
    expectedURL := defaultManagement + api.session.subscriptionId + "/services/hostedservices"
    checkOneRequest(c, &recordedRequests, expectedURL, "2013-10-01", nil, "GET")
    c.Assert(descriptors[0].URL, Equals, url)
}

func (suite *managementBaseAPISuite) TestUpdateHostedService(c *C) {
    api := makeAPI(c)
    randomLabel := MakeRandomString(10)
    randomDescription := MakeRandomString(10)
    property := ExtendedProperty{
        Name:  "property-name",
        Value: "property-value",
    }
    base64Label := base64.StdEncoding.EncodeToString([]byte(randomLabel))
    requestPayload := []byte(makeUpdateHostedService(base64Label, randomDescription, property))
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := MakeRandomString(10)
    properties := []ExtendedProperty{
        property,
    }
    update := NewUpdateHostedService(randomLabel, randomDescription, properties)
    err := api.UpdateHostedService(serviceName, update)

    c.Assert(err, IsNil)
    expectedURL := defaultManagement + api.session.subscriptionId + "/services/hostedservices/" + serviceName
    checkOneRequest(c, &recordedRequests, expectedURL, "2013-10-01", requestPayload, "PUT")
}

func assertGetHostedServicePropertiesRequest(c *C, api *ManagementAPI, serviceName string, embedDetail bool, httpRequest *X509Request) {
    var query string
    if embedDetail {
        query = "embed-detail=true"
    } else {
        query = "embed-detail=false"
    }
    expectedURL := fmt.Sprintf("%s%s/services/hostedservices/%s?%s", defaultManagement,
        api.session.subscriptionId, serviceName, query)
    checkRequest(c, httpRequest, expectedURL, "2013-10-01", nil, "GET")
}

func (suite *managementBaseAPISuite) TestGetHostedServiceProperties_withoutDetails(c *C) {
    api := makeAPI(c)
    body := `
        <?xml version="1.0" encoding="utf-8"?>
        <HostedService xmlns="http://schemas.microsoft.com/windowsazure">
          <Url>hosted-service-url</Url>
          <ServiceName>hosted-service-name</ServiceName>
          <HostedServiceProperties>
            <Description>description</Description>
            <AffinityGroup>name-of-affinity-group</AffinityGroup>
            <Location>location-of-service</Location >
            <Label>base-64-encoded-name-of-service</Label>
            <Status>current-status-of-service</Status>
            <DateCreated>creation-date-of-service</DateCreated>
            <DateLastModified>last-modification-date-of-service</DateLastModified>
            <ExtendedProperties>
              <ExtendedProperty>
                <Name>name-of-property</Name>
                <Value>value-of-property</Value>
              </ExtendedProperty>
            </ExtendedProperties>
          </HostedServiceProperties>
        </HostedService>
        `
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(body),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := "serviceName"

    properties, err := api.GetHostedServiceProperties(serviceName, false)
    c.Assert(err, IsNil)
    c.Check(recordedRequests, HasLen, 1)
    assertGetHostedServicePropertiesRequest(c, api, serviceName, false, recordedRequests[0])

    c.Check(properties.URL, Equals, "hosted-service-url")
    c.Check(properties.ServiceName, Equals, "hosted-service-name")
    c.Check(properties.Description, Equals, "description")
    // Details were explicitly not requested, so this is empty.
    c.Check(len(properties.Deployments), Equals, 0)
}

func (suite *managementBaseAPISuite) TestGetHostedServiceProperties_withDetails(c *C) {
    api := makeAPI(c)
    body := `
        <?xml version="1.0" encoding="utf-8"?>
        <HostedService xmlns="http://schemas.microsoft.com/windowsazure">
          <Url>hosted-service-url</Url>
          <ServiceName>hosted-service-name</ServiceName>
          <HostedServiceProperties>
            <Description>description-of-service</Description>
            <AffinityGroup>name-of-affinity-group</AffinityGroup>
            <Location>location-of-service</Location>
            <Label>base-64-encoded-name-of-service</Label>
            <Status>current-status-of-service</Status>
            <DateCreated>creation-date-of-service</DateCreated>
            <DateLastModified>last-modification-date-of-service</DateLastModified>
            <ExtendedProperties>
              <ExtendedProperty>
                <Name>name-of-property</Name>
                <Value>value-of-property</Value>
              </ExtendedProperty>
            </ExtendedProperties>
          </HostedServiceProperties>
          <Deployments>
            <Deployment xmlns="http://schemas.microsoft.com/windowsazure">
              <Name>name-of-deployment</Name>
            </Deployment>
          </Deployments>
        </HostedService>
        `
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(body),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := "serviceName"

    properties, err := api.GetHostedServiceProperties(serviceName, true)
    c.Assert(err, IsNil)
    c.Check(recordedRequests, HasLen, 1)
    assertGetHostedServicePropertiesRequest(c, api, serviceName, true, recordedRequests[0])

    c.Check(properties.URL, Equals, "hosted-service-url")
    c.Check(properties.ServiceName, Equals, "hosted-service-name")
    c.Check(properties.Description, Equals, "description-of-service")
    c.Check(len(properties.Deployments), Equals, 1)
    c.Check(properties.Deployments[0].Name, Equals, "name-of-deployment")
}

func (suite *managementBaseAPISuite) TestAddHostedService(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    createHostedService := NewCreateHostedServiceWithLocation("testName", "testLabel", "East US")
    err := api.AddHostedService(createHostedService)
    c.Assert(err, IsNil)
    expectedURL := defaultManagement + api.session.subscriptionId + "/services/hostedservices"
    expectedPayload, err := marshalXML(createHostedService)
    c.Assert(err, IsNil)
    checkOneRequest(c, recordedRequests, expectedURL, "2013-10-01", expectedPayload, "POST")
}

func makeAvailabilityResponse(result, reason string) string {
    return fmt.Sprintf(`
        <?xml version="1.0" encoding="utf-8"?>
        <AvailabilityResponse xmlns="http://schemas.microsoft.com/windowsazure">
          <Result>%s</Result>
          <Reason>%s</Reason>
        </AvailabilityResponse>`, result, reason)
}

func (*managementBaseAPISuite) TestAddHostedServiceWithOKName(c *C) {
    api := makeAPI(c)
    body := makeAvailabilityResponse("true", "")
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(body),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := "service-name"
    err := api.CheckHostedServiceNameAvailability(serviceName)

    c.Assert(err, IsNil)
    expectedURL := (defaultManagement + api.session.subscriptionId +
        "/services/hostedservices/operations/isavailable/" + serviceName)
    checkOneRequest(c, &recordedRequests, expectedURL, "2013-10-01", nil, "GET")
}

func (*managementBaseAPISuite) TestAddHostedServiceWithBadName(c *C) {
    api := makeAPI(c)
    reason := "This is a false test response"
    body := makeAvailabilityResponse("false", reason)
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(body),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := "service-name"
    err := api.CheckHostedServiceNameAvailability(serviceName)

    c.Assert(err, ErrorMatches, reason)
    c.Check(recordedRequests, HasLen, 1)
    expectedURL := (defaultManagement + api.session.subscriptionId +
        "/services/hostedservices/operations/isavailable/" + serviceName)
    checkOneRequest(c, &recordedRequests, expectedURL, "2013-10-01", nil, "GET")
}

func (*managementBaseAPISuite) TestAddHostedServiceWithServerError(c *C) {
    api := makeAPI(c)
    fixedResponse := x509Response{
        StatusCode: http.StatusBadRequest,
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := "service-name"
    err := api.CheckHostedServiceNameAvailability(serviceName)

    c.Assert(err, ErrorMatches, ".*Bad Request.*")
}

func (*managementBaseAPISuite) TestAddHostedServiceWithBadXML(c *C) {
    api := makeAPI(c)
    body := `
        <AvailabilityResponse>
            <Result>foo</Result>
            <Reason>unclosed tag
        </AvailabilityResponse>`
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(body),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := "service-name"
    err := api.CheckHostedServiceNameAvailability(serviceName)

    c.Assert(err, ErrorMatches, ".*XML syntax error.*")
}

func assertDeleteHostedServiceRequest(c *C, api *ManagementAPI, serviceName string, httpRequest *X509Request) {
    expectedURL := fmt.Sprintf("%s%s/services/hostedservices/%s", defaultManagement,
        api.session.subscriptionId, serviceName)
    checkRequest(c, httpRequest, expectedURL, "2010-10-28", nil, "DELETE")
}

func (suite *managementBaseAPISuite) TestDeleteHostedService(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    hostedServiceName := "testName"
    err := api.DeleteHostedService(hostedServiceName)
    c.Assert(err, IsNil)
    c.Check(*recordedRequests, HasLen, 1)
    assertDeleteHostedServiceRequest(c, api, hostedServiceName, (*recordedRequests)[0])
}

func (suite *managementBaseAPISuite) TestDeleteHostedServiceWhenServiceDoesNotExist(c *C) {
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusNotFound})
    err := makeAPI(c).DeleteHostedService("hosted-service-name")
    c.Assert(err, IsNil)
}

func (suite *managementBaseAPISuite) TestAddDeployment(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    serviceName := "serviceName"
    configurationSet := NewLinuxProvisioningConfigurationSet("testHostname12345", "test", "test123#@!", "user-data", "false")
    vhd := NewOSVirtualHardDisk("hostCaching", "diskLabel", "diskName", "http://mediaLink", "sourceImageName", "os")
    role := NewRole("ExtraSmall", "test-role-123", []ConfigurationSet{*configurationSet}, []OSVirtualHardDisk{*vhd})
    deployment := NewDeploymentForCreateVMDeployment("test-machine-name", "Staging", "testLabel", []Role{*role}, "testNetwork")
    err := api.AddDeployment(deployment, serviceName)

    c.Assert(err, IsNil)
    expectedURL := defaultManagement + api.session.subscriptionId + "/services/hostedservices/" + serviceName + "/deployments"
    expectedPayload, err := marshalXML(deployment)
    c.Assert(err, IsNil)
    checkOneRequest(c, recordedRequests, expectedURL, "2013-10-01", expectedPayload, "POST")
}

func assertDeleteDeploymentRequest(c *C, api *ManagementAPI, hostedServiceName, deploymentName string, httpRequest *X509Request) {
    expectedURL := fmt.Sprintf(
        "%s%s/services/hostedservices/%s/deployments/%s", defaultManagement,
        api.session.subscriptionId, hostedServiceName, deploymentName)
    checkRequest(c, httpRequest, expectedURL, "2013-10-01", nil, "DELETE")
}

func (suite *managementBaseAPISuite) TestDeleteDeployment(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    hostedServiceName := "testHosterServiceName"
    deploymentName := "testDeploymentName"
    err := api.DeleteDeployment(hostedServiceName, deploymentName)
    c.Assert(err, IsNil)
    c.Assert(*recordedRequests, HasLen, 1)
    assertDeleteDeploymentRequest(c, api, hostedServiceName, deploymentName, (*recordedRequests)[0])
}

func (suite *managementBaseAPISuite) TestDeleteDeploymentWhenDeploymentDoesNotExist(c *C) {
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusNotFound})
    err := makeAPI(c).DeleteDeployment("hosted-service-name", "deployment-name")
    c.Assert(err, IsNil)
}

var getDeploymentResponse = `
<?xml version="1.0"?>
<Deployment xmlns="http://schemas.microsoft.com/windowsazure" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
  <Name>gwaclmachinekjn8minr</Name>
  <DeploymentSlot>Staging</DeploymentSlot>
  <PrivateID>53b117c3126a4f1b8b23bc36c2c94dd1</PrivateID>
  <Status>Running</Status>
  <Label>WjNkaFkyeHRZV05vYVc1bGEycHVPRzFwYm5JPQ==</Label>
  <Url>http://53b117c3126a4f1b8b23bc36c2c94dd1.cloudapp.net/</Url>
  <Configuration>PFNlcnZpY2VDb25maWd1cmF0aW9uIHhtbG5zOnhzZD0iaHR0cDovL3d3dy53
My5vcmcvMjAwMS9YTUxTY2hlbWEiIHhtbG5zOnhzaT0iaHR0cDovL3d3dy53My5vcmcvMjAwMS9YTU
xTY2hlbWEtaW5zdGFuY2UiIHhtbG5zPSJodHRwOi8vc2NoZW1hcy5taWNyb3NvZnQuY29tL1NlcnZp
Y2VIb3N0aW5nLzIwMDgvMTAvU2VydmljZUNvbmZpZ3VyYXRpb24iPg0KICA8Um9sZSBuYW1lPSJnd2
FjbHJvbGVoYXVxODFyIj4NCiAgICA8SW5zdGFuY2VzIGNvdW50PSIxIiAvPg0KICA8L1JvbGU+DQo8
L1NlcnZpY2VDb25maWd1cmF0aW9uPg==</Configuration>
  <RoleInstanceList>
    <RoleInstance>
      <RoleName>gwaclrolehauq81r</RoleName>
      <InstanceName>gwaclrolehauq81r</InstanceName>
      <InstanceStatus>ReadyRole</InstanceStatus>
      <InstanceUpgradeDomain>0</InstanceUpgradeDomain>
      <InstanceFaultDomain>0</InstanceFaultDomain>
      <InstanceSize>ExtraSmall</InstanceSize>
      <InstanceStateDetails/>
      <IpAddress>10.241.158.13</IpAddress>
      <PowerState>Started</PowerState>
      <HostName>gwaclhostsnx7m1co57n</HostName>
      <RemoteAccessCertificateThumbprint>68db67cd8a6047a6cf6da0f92a7ee67d</RemoteAccessCertificateThumbprint>
    </RoleInstance>
  </RoleInstanceList>
  <UpgradeDomainCount>1</UpgradeDomainCount>
  <RoleList>
    <Role i:type="PersistentVMRole">
      <RoleName>gwaclrolehauq81r</RoleName>
      <OsVersion/>
      <RoleType>PersistentVMRole</RoleType>
      <ConfigurationSets>
        <ConfigurationSet i:type="NetworkConfigurationSet">
          <ConfigurationSetType>NetworkConfiguration</ConfigurationSetType>
          <SubnetNames/>
        </ConfigurationSet>
      </ConfigurationSets>
      <DataVirtualHardDisks/>
      <OSVirtualHardDisk>
        <HostCaching>ReadWrite</HostCaching>
        <DiskLabel>gwaclauonntmontirrz9rgltt8d5f4evtjeagbcx7kf8umqhs3t421m21t798ebw</DiskLabel>
        <DiskName>gwacldiskdvmvahc</DiskName>
        <MediaLink>http://gwacl3133mh3fs9jck6yk0dh.blob.core.windows.net/vhds/gwacldisk79vobmh.vhd</MediaLink>
        <SourceImageName>b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu_DAILY_BUILD-precise-12_04_2-LTS-amd64-server-20130624-en-us-30GB</SourceImageName>
        <OS>Linux</OS>
      </OSVirtualHardDisk>
      <RoleSize>ExtraSmall</RoleSize>
    </Role>
  </RoleList>
  <SdkVersion/>
  <Locked>false</Locked>
  <RollbackAllowed>false</RollbackAllowed>
  <CreatedTime>2013-06-25T14:35:22Z</CreatedTime>
  <LastModifiedTime>2013-06-25T14:48:54Z</LastModifiedTime>
  <ExtendedProperties/>
  <PersistentVMDowntime>
    <StartTime>2013-05-08T22:00:00Z</StartTime>
    <EndTime>2013-05-10T06:00:00Z</EndTime>
    <Status>PersistentVMUpdateCompleted</Status>
  </PersistentVMDowntime>
  <VirtualIPs>
    <VirtualIP>
      <Address>137.117.72.69</Address>
      <IsDnsProgrammed>true</IsDnsProgrammed>
      <Name>__PseudoBackEndContractVip</Name>
    </VirtualIP>
  </VirtualIPs>
</Deployment>
`

func assertGetDeploymentRequest(c *C, api *ManagementAPI, request *GetDeploymentRequest, httpRequest *X509Request) {
    expectedURL := fmt.Sprintf(
        "%s%s/services/hostedservices/%s/deployments/%s", defaultManagement,
        api.session.subscriptionId, request.ServiceName, request.DeploymentName)
    checkRequest(c, httpRequest, expectedURL, "2013-10-01", nil, "GET")
}

func (suite *managementBaseAPISuite) TestGetDeployment(c *C) {
    api := makeAPI(c)
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(getDeploymentResponse),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    serviceName := "serviceName"
    deploymentName := "gwaclmachinekjn8minr"

    request := &GetDeploymentRequest{ServiceName: serviceName, DeploymentName: deploymentName}
    deployment, err := api.GetDeployment(request)
    c.Assert(err, IsNil)
    c.Assert(recordedRequests, HasLen, 1)
    assertGetDeploymentRequest(c, api, request, recordedRequests[0])
    c.Check(deployment.Name, Equals, deploymentName)
}

func (suite *managementBaseAPISuite) TestAddStorageAccount(c *C) {
    api := makeAPI(c)
    header := http.Header{}
    header.Set("X-Ms-Request-Id", "operationID")
    fixedResponse := x509Response{
        StatusCode: http.StatusAccepted,
        Header:     header,
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)
    cssi := NewCreateStorageServiceInputWithLocation("name", "label", "East US", "false")

    err := api.AddStorageAccount(cssi)
    c.Assert(err, IsNil)

    expectedURL := defaultManagement + api.session.subscriptionId + "/services/storageservices"
    expectedPayload, err := marshalXML(cssi)
    c.Assert(err, IsNil)
    checkOneRequest(c, &recordedRequests, expectedURL, "2013-10-01", expectedPayload, "POST")
}

func (suite *managementBaseAPISuite) TestDeleteStorageAccount(c *C) {
    const accountName = "myaccount"
    api := makeAPI(c)
    accountURL := api.session.composeURL("services/storageservices/" + accountName)
    recordedRequests := setUpDispatcher("operationID")

    err := api.DeleteStorageAccount(accountName)
    c.Assert(err, IsNil)

    checkOneRequest(c, recordedRequests, accountURL, "2011-06-01", nil, "DELETE")
}

func (suite *managementBaseAPISuite) TestDeleteStorageAccountWhenAccountDoesNotExist(c *C) {
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusNotFound})
    err := makeAPI(c).DeleteStorageAccount("account-name")
    c.Assert(err, IsNil)
}

func (suite *managementBaseAPISuite) TestGetStorageAccountKeys(c *C) {
    const (
        accountName  = "accountname"
        primaryKey   = "primarykey"
        secondaryKey = "secondarykey"
    )
    api := makeAPI(c)
    url := api.session.composeURL("services/storageservices/" + accountName)
    body := fmt.Sprintf(
        `<StorageService>
            <Url>%s</Url>
            <StorageServiceKeys>
                <Primary>%s</Primary>
                <Secondary>%s</Secondary>
            </StorageServiceKeys>
        </StorageService>`,
        url, primaryKey, secondaryKey)
    rigFixedResponseDispatcher(&x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(body),
    })

    keys, err := api.GetStorageAccountKeys("account")
    c.Assert(err, IsNil)

    c.Check(keys.Primary, Equals, primaryKey)
    c.Check(keys.Secondary, Equals, secondaryKey)
    c.Check(keys.URL, Equals, url)
}

func assertDeleteDiskRequest(c *C, api *ManagementAPI, diskName string, httpRequest *X509Request, deleteBlob bool) {
    expectedURL := fmt.Sprintf("%s%s/services/disks/%s", defaultManagement,
        api.session.subscriptionId, diskName)
    if deleteBlob {
        expectedURL += "?comp=media"
    }
    checkRequest(c, httpRequest, expectedURL, "2012-08-01", nil, "DELETE")
}

func (suite *managementBaseAPISuite) TestDeleteDisk(c *C) {
    // The current implementation of DeleteDisk() works around a bug in
    // Windows Azure by polling the server.  See the documentation in the file
    // deletedisk.go for details.
    // Change the polling interval to speed up the tests:
    deleteDiskInterval = time.Nanosecond
    api := makeAPI(c)
    diskName := "diskName"
    for _, deleteBlob := range [...]bool{false, true} {
        recordedRequests := setUpDispatcher("operationID")
        err := api.DeleteDisk(&DeleteDiskRequest{
            DiskName:   diskName,
            DeleteBlob: deleteBlob,
        })
        c.Assert(err, IsNil)
        c.Assert(*recordedRequests, HasLen, 1)
        assertDeleteDiskRequest(c, api, diskName, (*recordedRequests)[0], deleteBlob)
    }
}

func (suite *managementBaseAPISuite) TestDeleteDiskWhenDiskDoesNotExist(c *C) {
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusNotFound})
    err := makeAPI(c).DeleteDisk(&DeleteDiskRequest{DiskName: "disk-name"})
    c.Assert(err, IsNil)
}

func (suite *managementBaseAPISuite) TestDeleteDiskWithDeleteBlob(c *C) {
    // Setting deleteBlob=true should append comp=media as a url query value.
    deleteDiskInterval = time.Nanosecond
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    diskName := "diskName"

    err := api.DeleteDisk(&DeleteDiskRequest{
        DiskName: diskName, DeleteBlob: true})

    c.Assert(err, IsNil)
    originalURL := (*recordedRequests)[0].URL
    parsedURL, err := url.Parse(originalURL)
    c.Assert(err, IsNil)
    values := parsedURL.Query()
    c.Assert(err, IsNil)
    c.Check(values["comp"], DeepEquals, []string{"media"})
}

func (suite *managementBaseAPISuite) TestPerformNodeOperation(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    serviceName := "serviceName"
    deploymentName := "deploymentName"
    roleName := "roleName"
    operation := newRoleOperation("RandomOperation")
    version := "test-version"
    err := api.performRoleOperation(serviceName, deploymentName, roleName, version, operation)

    c.Assert(err, IsNil)
    expectedURL := defaultManagement + api.session.subscriptionId + "/services/hostedservices/" + serviceName + "/deployments/" + deploymentName + "/roleinstances/" + roleName + "/Operations"
    expectedPayload, err := marshalXML(operation)
    c.Assert(err, IsNil)
    checkOneRequest(c, recordedRequests, expectedURL, version, expectedPayload, "POST")
}

func (suite *managementBaseAPISuite) TestStartRole(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    request := &StartRoleRequest{"serviceName", "deploymentName", "roleName"}
    err := api.StartRole(request)
    c.Assert(err, IsNil)
    expectedURL := (defaultManagement + api.session.subscriptionId + "/services/hostedservices/" +
        request.ServiceName + "/deployments/" + request.DeploymentName + "/roleinstances/" +
        request.RoleName + "/Operations")
    expectedPayload, err := marshalXML(startRoleOperation)
    c.Assert(err, IsNil)
    checkOneRequest(c, recordedRequests, expectedURL, "2013-10-01", expectedPayload, "POST")
}

func (suite *managementBaseAPISuite) TestRestartRole(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    request := &RestartRoleRequest{"serviceName", "deploymentName", "roleName"}
    err := api.RestartRole(request)
    c.Assert(err, IsNil)
    expectedURL := (defaultManagement + api.session.subscriptionId + "/services/hostedservices/" +
        request.ServiceName + "/deployments/" + request.DeploymentName + "/roleinstances/" +
        request.RoleName + "/Operations")
    expectedPayload, err := marshalXML(restartRoleOperation)
    c.Assert(err, IsNil)
    checkOneRequest(c, recordedRequests, expectedURL, "2013-10-01", expectedPayload, "POST")
}

func assertShutdownRoleRequest(c *C, api *ManagementAPI, request *ShutdownRoleRequest, httpRequest *X509Request) {
    expectedURL := fmt.Sprintf(
        "%s%s/services/hostedservices/%s/deployments/%s/roleinstances/%s/Operations",
        defaultManagement, api.session.subscriptionId, request.ServiceName,
        request.DeploymentName, request.RoleName)
    expectedPayload, err := marshalXML(shutdownRoleOperation)
    c.Assert(err, IsNil)
    checkRequest(c, httpRequest, expectedURL, "2013-10-01", expectedPayload, "POST")
}

func (suite *managementBaseAPISuite) TestShutdownRole(c *C) {
    api := makeAPI(c)
    recordedRequests := setUpDispatcher("operationID")
    request := &ShutdownRoleRequest{"serviceName", "deploymentName", "roleName"}
    err := api.ShutdownRole(request)
    c.Assert(err, IsNil)
    c.Assert(*recordedRequests, HasLen, 1)
    assertShutdownRoleRequest(c, api, request, (*recordedRequests)[0])
}

func assertGetRoleRequest(c *C, api *ManagementAPI, httpRequest *X509Request, serviceName, deploymentName, roleName string) {
    expectedURL := (defaultManagement + api.session.subscriptionId +
        "/services/hostedservices/" +
        serviceName + "/deployments/" + deploymentName + "/roles/" + roleName)
    checkRequest(c, httpRequest, expectedURL, "2013-10-01", nil, "GET")
}

func (suite *managementBaseAPISuite) TestGetRole(c *C) {
    api := makeAPI(c)
    request := &GetRoleRequest{"serviceName", "deploymentName", "roleName"}

    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(makePersistentVMRole("rolename")),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    role, err := api.GetRole(request)
    c.Assert(err, IsNil)

    assertGetRoleRequest(
        c, api, recordedRequests[0], request.ServiceName,
        request.DeploymentName, request.RoleName)
    c.Check(role.RoleName, Equals, "rolename")
}

func assertUpdateRoleRequest(c *C, api *ManagementAPI, httpRequest *X509Request, serviceName, deploymentName, roleName, expectedXML string) {
    expectedURL := (defaultManagement + api.session.subscriptionId +
        "/services/hostedservices/" +
        serviceName + "/deployments/" + deploymentName + "/roles/" + roleName)
    checkRequest(
        c, httpRequest, expectedURL, "2013-10-01", []byte(expectedXML), "PUT")
    c.Assert(httpRequest.ContentType, Equals, "application/xml")
}

func (suite *managementBaseAPISuite) TestUpdateRole(c *C) {
    api := makeAPI(c)
    request := &UpdateRoleRequest{
        ServiceName:    "serviceName",
        DeploymentName: "deploymentName",
        RoleName:       "roleName",
        PersistentVMRole: &PersistentVMRole{
            RoleName: "newRoleNamePerhaps",
        },
    }
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusOK})
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    err := api.UpdateRole(request)
    c.Assert(err, IsNil)

    expectedXML, err := request.PersistentVMRole.Serialize()
    c.Assert(err, IsNil)
    assertUpdateRoleRequest(
        c, api, recordedRequests[0], request.ServiceName,
        request.DeploymentName, request.RoleName, expectedXML)
}

func (suite *managementBaseAPISuite) TestUpdateRoleBlocksUntilComplete(c *C) {
    api := makeAPI(c)
    api.PollerInterval = time.Nanosecond
    request := &UpdateRoleRequest{
        ServiceName:    "serviceName",
        DeploymentName: "deploymentName",
        RoleName:       "roleName",
        PersistentVMRole: &PersistentVMRole{
            RoleName: "newRoleNamePerhaps",
        },
    }
    responses := []DispatcherResponse{
        // First response is 202 with an X-MS-Request-ID header.
        {makeX509ResponseWithOperationHeader("foobar"), nil},
        // Second response is XML to say that the request above has completed.
        {
            &x509Response{
                Body:       []byte(fmt.Sprintf(operationXMLTemplate, "Succeeded")),
                StatusCode: http.StatusOK,
            },
            nil,
        },
    }
    recordedRequests := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&recordedRequests, responses)

    err := api.UpdateRole(request)
    c.Assert(err, IsNil)

    c.Assert(recordedRequests, HasLen, 2)
    expectedXML, err := request.PersistentVMRole.Serialize()
    c.Assert(err, IsNil)
    assertUpdateRoleRequest(
        c, api, recordedRequests[0], request.ServiceName,
        request.DeploymentName, request.RoleName, expectedXML)
    // Second request is to get status of operation "foobar".
    c.Check(recordedRequests[1].Method, Equals, "GET")
    c.Check(recordedRequests[1].URL, Matches, ".*/operations/foobar")
}

func (suite *managementBaseAPISuite) TestCreateAffinityGroup(c *C) {
    api := makeAPI(c)
    cag := NewCreateAffinityGroup(
        "name", "label", "description", "location")
    request := CreateAffinityGroupRequest{
        CreateAffinityGroup: cag}
    fixedResponse := x509Response{
        StatusCode: http.StatusCreated}
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    err := api.CreateAffinityGroup(&request)
    c.Assert(err, IsNil)

    expectedURL := defaultManagement + api.session.subscriptionId + "/affinitygroups"
    expectedBody, _ := cag.Serialize()
    checkOneRequest(c, &recordedRequests, expectedURL, "2013-10-01", []byte(expectedBody), "POST")
}

func (suite *managementBaseAPISuite) TestUpdateAffinityGroup(c *C) {
    api := makeAPI(c)
    uag := NewUpdateAffinityGroup("label", "description")
    request := UpdateAffinityGroupRequest{
        Name:                "groupname",
        UpdateAffinityGroup: uag}
    fixedResponse := x509Response{
        StatusCode: http.StatusCreated}
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    err := api.UpdateAffinityGroup(&request)
    c.Assert(err, IsNil)

    expectedURL := (defaultManagement + api.session.subscriptionId +
        "/affinitygroups/" + request.Name)
    expectedBody, _ := uag.Serialize()
    checkOneRequest(c, &recordedRequests, expectedURL, "2011-02-25", []byte(expectedBody), "PUT")
}

func (suite *managementBaseAPISuite) TestDeleteAffinityGroup(c *C) {
    api := makeAPI(c)
    request := DeleteAffinityGroupRequest{
        Name: "groupname"}
    fixedResponse := x509Response{
        StatusCode: http.StatusCreated}
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    err := api.DeleteAffinityGroup(&request)
    c.Assert(err, IsNil)

    expectedURL := (defaultManagement + api.session.subscriptionId +
        "/affinitygroups/" + request.Name)
    checkOneRequest(c, &recordedRequests, expectedURL, "2011-02-25", nil, "DELETE")
}

func (suite *managementBaseAPISuite) TestDeleteAffinityGroupWhenGroupDoesNotExist(c *C) {
    rigFixedResponseDispatcher(&x509Response{StatusCode: http.StatusNotFound})
    request := DeleteAffinityGroupRequest{Name: "groupname"}
    err := makeAPI(c).DeleteAffinityGroup(&request)
    c.Assert(err, IsNil)
}

func makeNetworkConfiguration() *NetworkConfiguration {
    return &NetworkConfiguration{
        XMLNS: XMLNS_NC,
        DNS: &[]VirtualNetDnsServer{
            {
                Name:      "dns-server-name",
                IPAddress: "IPV4-address-of-the-server",
            },
        },
        LocalNetworkSites: &[]LocalNetworkSite{
            {
                Name: "local-site-name",
                AddressSpacePrefixes: []string{
                    "CIDR-identifier",
                },
                VPNGatewayAddress: "IPV4-address-of-the-vpn-gateway",
            },
        },
        VirtualNetworkSites: &[]VirtualNetworkSite{
            {
                Name:          "virtual-network-name",
                AffinityGroup: "affinity-group-name",
                AddressSpacePrefixes: []string{
                    "CIDR-identifier",
                },
                Subnets: &[]Subnet{
                    {
                        Name:          "subnet-name",
                        AddressPrefix: "CIDR-identifier",
                    },
                },
                DnsServersRef: &[]DnsServerRef{
                    {
                        Name: "primary-DNS-name",
                    },
                },
                Gateway: &Gateway{
                    Profile: "Small",
                    VPNClientAddressPoolPrefixes: []string{
                        "CIDR-identifier",
                    },
                    LocalNetworkSiteRef: LocalNetworkSiteRef{
                        Name: "local-site-name",
                        Connection: LocalNetworkSiteRefConnection{
                            Type: "connection-type",
                        },
                    },
                },
            },
        },
    }
}

func assertGetNetworkConfigurationRequest(c *C, api *ManagementAPI, httpRequest *X509Request) {
    expectedURL := fmt.Sprintf(
        "%s%s/services/networking/media", defaultManagement,
        api.session.subscriptionId)
    checkRequest(c, httpRequest, expectedURL, "2013-10-01", nil, "GET")
}

func (suite *managementBaseAPISuite) TestGetNetworkConfiguration(c *C) {
    expected := makeNetworkConfiguration()
    expectedXML, err := expected.Serialize()
    c.Assert(err, IsNil)

    rigFixedResponseDispatcher(&x509Response{
        StatusCode: http.StatusOK, Body: []byte(expectedXML)})
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    api := makeAPI(c)
    observed, err := api.GetNetworkConfiguration()
    c.Assert(err, IsNil)
    c.Assert(recordedRequests, HasLen, 1)
    assertGetNetworkConfigurationRequest(c, api, recordedRequests[0])
    c.Assert(observed, DeepEquals, expected)
}

func (suite *managementBaseAPISuite) TestGetNetworkConfigurationNotFound(c *C) {
    rigFixedResponseDispatcher(&x509Response{
        StatusCode: http.StatusNotFound})
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    api := makeAPI(c)
    observed, err := api.GetNetworkConfiguration()
    c.Assert(observed, IsNil)
    c.Assert(err, IsNil)
    c.Assert(recordedRequests, HasLen, 1)
    assertGetNetworkConfigurationRequest(c, api, recordedRequests[0])
}

func assertSetNetworkConfigurationRequest(c *C, api *ManagementAPI, body []byte, httpRequest *X509Request) {
    expectedURL := fmt.Sprintf(
        "%s%s/services/networking/media", defaultManagement,
        api.session.subscriptionId)
    checkRequest(c, httpRequest, expectedURL, "2013-10-01", body, "PUT")
    // Azure chokes when the content type is text/xml or similar.
    c.Assert(httpRequest.ContentType, Equals, "application/octet-stream")
}

func (suite *managementBaseAPISuite) TestSetNetworkConfiguration(c *C) {
    api := makeAPI(c)
    fixedResponse := x509Response{StatusCode: http.StatusOK}
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    request := makeNetworkConfiguration()
    requestXML, err := request.Serialize()
    c.Assert(err, IsNil)
    requestPayload := []byte(requestXML)
    err = api.SetNetworkConfiguration(request)

    c.Assert(err, IsNil)
    c.Assert(recordedRequests, HasLen, 1)
    assertSetNetworkConfigurationRequest(c, api, requestPayload, recordedRequests[0])
}
