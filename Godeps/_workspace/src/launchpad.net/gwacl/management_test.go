// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    . "launchpad.net/gocheck"
    "net/http"
)

type managementAPISuite struct{}

var _ = Suite(&managementAPISuite{})

// makeNamedRoleInstances creates an array of RoleInstance objects, each with
// the respective given name.
func makeNamedRoleInstances(names ...string) []RoleInstance {
    instances := make([]RoleInstance, 0)
    for _, name := range names {
        instances = append(instances, RoleInstance{RoleName: name})
    }
    return instances
}

// makeNamedDeployments creates an array of Deployment objects, each with
// the respective given name.
func makeNamedDeployments(names ...string) []Deployment {
    deployments := make([]Deployment, 0)
    for _, name := range names {
        deployments = append(deployments, Deployment{Name: name})
    }
    return deployments
}

// makeHostedService creates a HostedService with the given deployments.
func makeHostedService(deployments []Deployment) HostedService {
    desc := HostedServiceDescriptor{ServiceName: "S1"}
    return HostedService{
        HostedServiceDescriptor: desc,
        Deployments:             deployments,
    }
}

// makeOKXMLResponse creates a DispatcherResponse with status code OK, and
// an XML-serialized version of the given object.
// The response is wrapped in a slice because that's slightly easier for
// the callers.
func makeOKXMLResponse(c *C, bodyObject AzureObject) []DispatcherResponse {
    body, err := bodyObject.Serialize()
    c.Assert(err, IsNil)
    return []DispatcherResponse{
        {
            response: &x509Response{
                StatusCode: http.StatusOK,
                Body:       []byte(body),
            },
        },
    }
}

// TestListInstances goes through the happy path for ListInstances.
func (suite *managementAPISuite) TestListInstances(c *C) {
    service := makeHostedService(
        []Deployment{
            {RoleInstanceList: makeNamedRoleInstances("one", "two")},
            {RoleInstanceList: makeNamedRoleInstances("three", "four")},
        })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, makeOKXMLResponse(c, service))

    // Exercise ListInstances.
    api := makeAPI(c)
    request := &ListInstancesRequest{ServiceName: service.ServiceName}
    instances, err := api.ListInstances(request)
    c.Assert(err, IsNil)

    // We get the expected instances back.
    c.Check(instances, DeepEquals, []RoleInstance{
        service.Deployments[0].RoleInstanceList[0],
        service.Deployments[0].RoleInstanceList[1],
        service.Deployments[1].RoleInstanceList[0],
        service.Deployments[1].RoleInstanceList[1],
    })

    // The only request is for the service's properties
    c.Assert(record, Not(HasLen), 0)
    expectedURL := fmt.Sprintf(
        "%ssubscriptionId/services/hostedservices/%s?embed-detail=true",
        defaultManagement, service.ServiceName)
    c.Check(record[0].URL, Equals, expectedURL)
    c.Check(record[0].Method, Equals, "GET")
}

func (suite *managementAPISuite) TestListInstancesFailsGettingDetails(c *C) {
    rigPreparedResponseDispatcher(
        []DispatcherResponse{{response: &x509Response{StatusCode: http.StatusNotFound}}},
    )
    api := makeAPI(c)
    request := &ListInstancesRequest{ServiceName: "SomeService"}
    instances, err := api.ListInstances(request)
    c.Check(instances, IsNil)
    c.Assert(err, NotNil)
    c.Assert(err.Error(), Equals, "GET request failed (404: Not Found)")
}

// TestListAllDeployments goes through the happy path for ListAllDeployments.
func (suite *managementAPISuite) TestListAllDeployments(c *C) {
    service := makeHostedService(makeNamedDeployments("one", "two"))
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, makeOKXMLResponse(c, service))

    // Exercise ListDeployments.
    api := makeAPI(c)
    request := &ListAllDeploymentsRequest{ServiceName: service.ServiceName}
    deployments, err := api.ListAllDeployments(request)
    c.Assert(err, IsNil)

    // We get the complete set of deployments back.
    c.Check(deployments, DeepEquals, service.Deployments)

    // Only one request to the API is made.
    c.Assert(record, HasLen, 1)
}

// TestListDeployments tests ListDeployments, including filtering by name.
func (suite *managementAPISuite) TestListDeployments(c *C) {
    service := makeHostedService(makeNamedDeployments("Arthur", "Bobby"))
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, makeOKXMLResponse(c, service))

    // Exercise ListDeployments.
    api := makeAPI(c)
    request := &ListDeploymentsRequest{
        ServiceName:     service.ServiceName,
        DeploymentNames: []string{"Arthur"},
    }
    deployments, err := api.ListDeployments(request)
    c.Assert(err, IsNil)

    // Only the first deployment - named "Arthur" - is returned.
    c.Check(deployments, DeepEquals, []Deployment{service.Deployments[0]})
    // Only one request to the API is made.
    c.Assert(record, HasLen, 1)
}

func (suite *managementAPISuite) TestListDeploymentsWithoutNamesReturnsNothing(c *C) {
    service := makeHostedService(makeNamedDeployments("One", "Two"))
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, makeOKXMLResponse(c, service))
    // Exercise ListDeployments.
    api := makeAPI(c)
    request := &ListDeploymentsRequest{
        ServiceName:     service.ServiceName,
        DeploymentNames: []string{},
    }
    deployments, err := api.ListDeployments(request)
    c.Assert(err, IsNil)

    // No deployments are returned.
    c.Check(deployments, HasLen, 0)
}

func makeHostedServiceDescriptor() *HostedServiceDescriptor {
    url := MakeRandomString(10)
    serviceName := MakeRandomString(10)
    return &HostedServiceDescriptor{ServiceName: serviceName, URL: url}
}

func (suite *managementAPISuite) TestListSpecificHostedServices(c *C) {
    service1 := makeHostedServiceDescriptor()
    service2 := makeHostedServiceDescriptor()
    list := HostedServiceDescriptorList{HostedServices: []HostedServiceDescriptor{*service1, *service2}}
    XML, err := list.Serialize()
    c.Assert(err, IsNil)
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(XML),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    api := makeAPI(c)
    request := &ListSpecificHostedServicesRequest{
        ServiceNames: []string{service1.ServiceName},
    }
    descriptors, err := api.ListSpecificHostedServices(request)

    // Only the first service is returned.
    c.Check(descriptors, DeepEquals, []HostedServiceDescriptor{*service1})
    // Only one request to the API is made.
    c.Assert(recordedRequests, HasLen, 1)
}

func (suite *managementAPISuite) TestListPrefixedHostedServices(c *C) {
    prefix := "prefix"
    service1 := &HostedServiceDescriptor{ServiceName: prefix + "service1"}
    service2 := makeHostedServiceDescriptor()
    list := HostedServiceDescriptorList{HostedServices: []HostedServiceDescriptor{*service1, *service2}}
    XML, err := list.Serialize()
    c.Assert(err, IsNil)
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(XML),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    api := makeAPI(c)
    request := &ListPrefixedHostedServicesRequest{
        ServiceNamePrefix: prefix,
    }
    descriptors, err := api.ListPrefixedHostedServices(request)

    // Only the first service is returned.
    c.Check(descriptors, DeepEquals, []HostedServiceDescriptor{*service1})
    // Only one request to the API is made.
    c.Assert(recordedRequests, HasLen, 1)
}

func (suite *managementAPISuite) TestListSpecificHostedServicesWithoutNamesReturnsNothing(c *C) {
    service1 := makeHostedServiceDescriptor()
    service2 := makeHostedServiceDescriptor()
    list := HostedServiceDescriptorList{HostedServices: []HostedServiceDescriptor{*service1, *service2}}
    XML, err := list.Serialize()
    c.Assert(err, IsNil)
    fixedResponse := x509Response{
        StatusCode: http.StatusOK,
        Body:       []byte(XML),
    }
    rigFixedResponseDispatcher(&fixedResponse)
    recordedRequests := make([]*X509Request, 0)
    rigRecordingDispatcher(&recordedRequests)

    api := makeAPI(c)
    request := &ListSpecificHostedServicesRequest{
        ServiceNames: []string{},
    }
    descriptors, err := api.ListSpecificHostedServices(request)

    c.Check(descriptors, DeepEquals, []HostedServiceDescriptor{})
    // Only one request to the API is made.
    c.Assert(recordedRequests, HasLen, 1)
}

var exampleOkayResponse = DispatcherResponse{
    response: &x509Response{StatusCode: http.StatusOK},
}

var exampleFailResponse = DispatcherResponse{
    response: &x509Response{StatusCode: http.StatusInternalServerError},
}

var exampleNotFoundResponse = DispatcherResponse{
    response: &x509Response{StatusCode: http.StatusNotFound},
}

type suiteDestroyDeployment struct{}

var _ = Suite(&suiteDestroyDeployment{})

func (suite *suiteDestroyDeployment) makeExampleDeployment() *Deployment {
    return &Deployment{
        RoleInstanceList: makeNamedRoleInstances("one", "two"),
        RoleList: []Role{
            {OSVirtualHardDisk: []OSVirtualHardDisk{
                {DiskName: "disk1"}, {DiskName: "disk2"}}},
            {OSVirtualHardDisk: []OSVirtualHardDisk{
                {DiskName: "disk1"}, {DiskName: "disk3"}}},
        },
    }
}

func (suite *suiteDestroyDeployment) TestHappyPath(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleDeployment := suite.makeExampleDeployment()
    responses = append(responses, makeOKXMLResponse(c, exampleDeployment)...)
    // For deleting the deployment.
    responses = append(responses, exampleOkayResponse)
    // For deleting disks.
    responses = append(responses, exampleOkayResponse, exampleOkayResponse, exampleOkayResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyDeployment.
    api := makeAPI(c)
    request := &DestroyDeploymentRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
    }
    err := api.DestroyDeployment(request)
    c.Assert(err, IsNil)
    c.Check(record, HasLen, 5)
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, request.DeploymentName}, record[0])
    assertDeleteDeploymentRequest(c, api, request.ServiceName,
        request.DeploymentName, record[1])
    assertDeleteDiskRequest(c, api, "disk1", record[2], true)
    assertDeleteDiskRequest(c, api, "disk2", record[3], true)
    assertDeleteDiskRequest(c, api, "disk3", record[4], true)
}

func (suite *suiteDestroyDeployment) TestOkayWhenDeploymentNotFound(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    responses = []DispatcherResponse{exampleNotFoundResponse}
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyDeployment.
    api := makeAPI(c)
    request := &DestroyDeploymentRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
    }
    err := api.DestroyDeployment(request)
    c.Assert(err, IsNil)
    c.Check(record, HasLen, 1)
}

func (suite *suiteDestroyDeployment) TestOkayWhenAssetsNotFound(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleDeployment := suite.makeExampleDeployment()
    responses = append(responses, makeOKXMLResponse(c, exampleDeployment)...)
    // For deleting the deployment.
    responses = append(responses, exampleNotFoundResponse)
    // For deleting the disks.
    responses = append(responses, exampleNotFoundResponse, exampleNotFoundResponse, exampleNotFoundResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyDeployment.
    api := makeAPI(c)
    request := &DestroyDeploymentRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
    }
    err := api.DestroyDeployment(request)
    c.Assert(err, IsNil)
    c.Check(record, HasLen, 5)
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, request.DeploymentName}, record[0])
    assertDeleteDeploymentRequest(c, api, request.ServiceName,
        request.DeploymentName, record[1])
    assertDeleteDiskRequest(c, api, "disk1", record[2], true)
    assertDeleteDiskRequest(c, api, "disk2", record[3], true)
    assertDeleteDiskRequest(c, api, "disk3", record[4], true)
}

func (suite *suiteDestroyDeployment) TestFailsGettingDeployment(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    responses = []DispatcherResponse{exampleFailResponse}
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyDeployment.
    api := makeAPI(c)
    request := &DestroyDeploymentRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
    }
    err := api.DestroyDeployment(request)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 1)
}

func (suite *suiteDestroyDeployment) TestFailsDeletingDisk(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleDeployment := suite.makeExampleDeployment()
    responses = append(responses, makeOKXMLResponse(c, exampleDeployment)...)
    // For deleting disks.
    responses = append(responses, exampleFailResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyDeployment.
    api := makeAPI(c)
    request := &DestroyDeploymentRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
    }
    err := api.DestroyDeployment(request)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "DELETE request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 2)
}

func (suite *suiteDestroyDeployment) TestFailsDeletingDeployment(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleDeployment := suite.makeExampleDeployment()
    responses = append(responses, makeOKXMLResponse(c, exampleDeployment)...)
    // For deleting disks.
    responses = append(responses, exampleOkayResponse, exampleOkayResponse, exampleOkayResponse)
    // For other requests.
    responses = append(responses, exampleFailResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyDeployment.
    api := makeAPI(c)
    request := &DestroyDeploymentRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
    }
    err := api.DestroyDeployment(request)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "DELETE request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 5)
}

type suiteDestroyHostedService struct{}

var _ = Suite(&suiteDestroyHostedService{})

func (suite *suiteDestroyHostedService) makeExampleHostedService(deploymentNames ...string) *HostedService {
    var exampleHostedService = &HostedService{}
    for _, deploymentName := range deploymentNames {
        exampleHostedService.Deployments = append(
            exampleHostedService.Deployments,
            Deployment{Name: deploymentName})
    }
    return exampleHostedService
}

func (suite *suiteDestroyHostedService) TestHappyPath(c *C) {
    var responses []DispatcherResponse
    // DestroyHostedService first gets the hosted service proerties.
    exampleHostedService := suite.makeExampleHostedService("one", "two")
    responses = append(responses, makeOKXMLResponse(c, exampleHostedService)...)
    // It calls DestroyDeployment, which first gets the deployment's
    // properties, deletes any assets contained therein (none in this case)
    // then deletes the deployment.
    for _, deployment := range exampleHostedService.Deployments {
        responses = append(responses, makeOKXMLResponse(c, &deployment)...)
        responses = append(responses, exampleOkayResponse)
    }
    // For deleting the service itself.
    responses = append(responses, exampleOkayResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyHostedService.
    api := makeAPI(c)
    request := &DestroyHostedServiceRequest{
        ServiceName: "service-name",
    }
    err := api.DestroyHostedService(request)
    c.Assert(err, IsNil)
    c.Check(record, HasLen, 6)
    // The first request is for the hosted service.
    assertGetHostedServicePropertiesRequest(c, api, request.ServiceName, true, record[0])
    // The second and third requests fetch then delete deployment "one"; see
    // DestroyDeployment for an explanation of this behaviour.
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, "one"}, record[1])
    assertDeleteDeploymentRequest(c, api, request.ServiceName, "one",
        record[2])
    // The fourth and fifth requests are a repaat of the previous two, but for
    // deployment "two".
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, "two"}, record[3])
    assertDeleteDeploymentRequest(c, api, request.ServiceName, "two",
        record[4])
    // The last request deletes the hosted service.
    assertDeleteHostedServiceRequest(c, api, request.ServiceName, record[5])
}

func (suite *suiteDestroyHostedService) TestOkayWhenHostedServiceNotFound(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    responses = []DispatcherResponse{exampleNotFoundResponse}
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyHostedService.
    api := makeAPI(c)
    request := &DestroyHostedServiceRequest{ServiceName: "service-name"}
    err := api.DestroyHostedService(request)
    c.Assert(err, IsNil)
    c.Check(record, HasLen, 1)
}

func (suite *suiteDestroyHostedService) TestOkayWhenDeploymentsNotFound(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleHostedService := suite.makeExampleHostedService("one", "two")
    responses = append(responses, makeOKXMLResponse(c, exampleHostedService)...)
    // Someone else has destroyed the deployments in the meantime.
    responses = append(responses, exampleNotFoundResponse, exampleNotFoundResponse)
    // Success deleting the hosted service.
    responses = append(responses, exampleOkayResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyHostedService.
    api := makeAPI(c)
    request := &DestroyHostedServiceRequest{ServiceName: "service-name"}
    err := api.DestroyHostedService(request)
    c.Assert(err, IsNil)
    c.Check(record, HasLen, 4)
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, "one"}, record[1])
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, "two"}, record[2])
    assertDeleteHostedServiceRequest(c, api, request.ServiceName, record[3])
}

func (suite *suiteDestroyHostedService) TestOkayWhenHostedServiceNotFoundWhenDeleting(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleHostedService := suite.makeExampleHostedService("one", "two")
    responses = append(responses, makeOKXMLResponse(c, exampleHostedService)...)
    // Someone else has destroyed the deployments in the meantime.
    responses = append(responses, exampleNotFoundResponse, exampleNotFoundResponse)
    // Someone else has destroyed the hosted service in the meantime.
    responses = append(responses, exampleNotFoundResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyHostedService.
    api := makeAPI(c)
    request := &DestroyHostedServiceRequest{ServiceName: "service-name"}
    err := api.DestroyHostedService(request)
    c.Assert(err, IsNil)
    c.Check(record, HasLen, 4)
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, "one"}, record[1])
    assertGetDeploymentRequest(c, api, &GetDeploymentRequest{
        request.ServiceName, "two"}, record[2])
    assertDeleteHostedServiceRequest(c, api, request.ServiceName, record[3])
}

func (suite *suiteDestroyHostedService) TestFailsGettingHostedService(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    responses = []DispatcherResponse{exampleFailResponse}
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyHostedService.
    api := makeAPI(c)
    request := &DestroyHostedServiceRequest{ServiceName: "service-name"}
    err := api.DestroyHostedService(request)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 1)
}

func (suite *suiteDestroyHostedService) TestFailsDestroyingDeployment(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleHostedService := suite.makeExampleHostedService("one", "two")
    responses = append(responses, makeOKXMLResponse(c, exampleHostedService)...)
    responses = append(responses, exampleFailResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyHostedService.
    api := makeAPI(c)
    request := &DestroyHostedServiceRequest{ServiceName: "service-name"}
    err := api.DestroyHostedService(request)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 2)
}

func (suite *suiteDestroyHostedService) TestFailsDeletingHostedService(c *C) {
    var responses []DispatcherResponse
    // Prepare.
    exampleHostedService := suite.makeExampleHostedService("one", "two")
    responses = append(responses, makeOKXMLResponse(c, exampleHostedService)...)
    // Deployments not found, but that's okay.
    responses = append(responses, exampleNotFoundResponse, exampleNotFoundResponse)
    // When deleting hosted service.
    responses = append(responses, exampleFailResponse)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    // Exercise DestroyHostedService.
    api := makeAPI(c)
    request := &DestroyHostedServiceRequest{ServiceName: "service-name"}
    err := api.DestroyHostedService(request)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "DELETE request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 4)
}

type suiteAddVirtualNetworkSite struct{}

var _ = Suite(&suiteAddVirtualNetworkSite{})

func (suite *suiteAddVirtualNetworkSite) TestWhenConfigCannotBeFetched(c *C) {
    responses := []DispatcherResponse{
        {response: &x509Response{StatusCode: http.StatusInternalServerError}},
    }
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    err := api.AddVirtualNetworkSite(nil)

    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 1)
    assertGetNetworkConfigurationRequest(c, api, record[0])
}

func (suite *suiteAddVirtualNetworkSite) TestWhenConfigDoesNotExist(c *C) {
    responses := []DispatcherResponse{
        // No configuration found.
        {response: &x509Response{StatusCode: http.StatusNotFound}},
        // Accept upload of new configuration.
        {response: &x509Response{StatusCode: http.StatusOK}},
    }
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    virtualNetwork := &VirtualNetworkSite{Name: MakeRandomVirtualNetworkName("test-")}

    err := api.AddVirtualNetworkSite(virtualNetwork)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    assertGetNetworkConfigurationRequest(c, api, record[0])
    expected := &NetworkConfiguration{
        XMLNS:               XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{*virtualNetwork},
    }
    expectedBody, err := expected.Serialize()
    c.Assert(err, IsNil)
    assertSetNetworkConfigurationRequest(c, api, []byte(expectedBody), record[1])
}

func (suite *suiteAddVirtualNetworkSite) TestWhenNoPreexistingVirtualNetworkSites(c *C) {
    // Prepare a basic, empty, configuration.
    existingConfig := &NetworkConfiguration{XMLNS: XMLNS_NC}
    responses := makeOKXMLResponse(c, existingConfig)
    responses = append(responses, DispatcherResponse{
        // Accept upload of new configuration.
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    virtualNetwork := &VirtualNetworkSite{Name: MakeRandomVirtualNetworkName("test-")}

    err := api.AddVirtualNetworkSite(virtualNetwork)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    assertGetNetworkConfigurationRequest(c, api, record[0])
    expected := &NetworkConfiguration{
        XMLNS:               XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{*virtualNetwork},
    }
    expectedBody, err := expected.Serialize()
    c.Assert(err, IsNil)
    assertSetNetworkConfigurationRequest(c, api, []byte(expectedBody), record[1])
}

func (suite *suiteAddVirtualNetworkSite) TestWhenPreexistingVirtualNetworkSites(c *C) {
    // Prepare a configuration with a single virtual network.
    existingConfig := &NetworkConfiguration{
        XMLNS: XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{
            {Name: MakeRandomVirtualNetworkName("test-")},
        },
    }
    responses := makeOKXMLResponse(c, existingConfig)
    responses = append(responses, DispatcherResponse{
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    virtualNetwork := &VirtualNetworkSite{Name: MakeRandomVirtualNetworkName("test-")}

    err := api.AddVirtualNetworkSite(virtualNetwork)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    assertGetNetworkConfigurationRequest(c, api, record[0])
    expectedSites := append(
        *existingConfig.VirtualNetworkSites, *virtualNetwork)
    expected := &NetworkConfiguration{
        XMLNS:               XMLNS_NC,
        VirtualNetworkSites: &expectedSites,
    }
    expectedBody, err := expected.Serialize()
    c.Assert(err, IsNil)
    assertSetNetworkConfigurationRequest(c, api, []byte(expectedBody), record[1])
}

func (suite *suiteAddVirtualNetworkSite) TestWhenPreexistingVirtualNetworkSiteWithSameName(c *C) {
    // Prepare a configuration with a single virtual network.
    existingConfig := &NetworkConfiguration{
        XMLNS: XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{
            {Name: "virtual-network-bob"},
        },
    }
    responses := makeOKXMLResponse(c, existingConfig)
    rigPreparedResponseDispatcher(responses)
    api := makeAPI(c)
    virtualNetwork := &VirtualNetworkSite{Name: "virtual-network-bob"}

    err := api.AddVirtualNetworkSite(virtualNetwork)

    c.Check(err, ErrorMatches, "could not add virtual network: \"virtual-network-bob\" already exists")
}

func (suite *suiteAddVirtualNetworkSite) TestWhenConfigCannotBePushed(c *C) {
    responses := []DispatcherResponse{
        // No configuration found.
        {response: &x509Response{StatusCode: http.StatusNotFound}},
        // Cannot accept upload of new configuration.
        {response: &x509Response{StatusCode: http.StatusInternalServerError}},
    }
    rigPreparedResponseDispatcher(responses)
    virtualNetwork := &VirtualNetworkSite{Name: MakeRandomVirtualNetworkName("test-")}

    err := makeAPI(c).AddVirtualNetworkSite(virtualNetwork)

    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
}

type suiteRemoveVirtualNetworkSite struct{}

var _ = Suite(&suiteRemoveVirtualNetworkSite{})

func (suite *suiteRemoveVirtualNetworkSite) TestWhenConfigCannotBeFetched(c *C) {
    responses := []DispatcherResponse{
        {response: &x509Response{StatusCode: http.StatusInternalServerError}},
    }
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    err := api.RemoveVirtualNetworkSite("virtual-network-o-doom")

    c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 1)
    assertGetNetworkConfigurationRequest(c, api, record[0])
}

func (suite *suiteRemoveVirtualNetworkSite) TestWhenConfigDoesNotExist(c *C) {
    responses := []DispatcherResponse{
        {response: &x509Response{StatusCode: http.StatusNotFound}},
    }
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    err := api.RemoveVirtualNetworkSite("virtual-network-in-my-eyes")

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 1)
    assertGetNetworkConfigurationRequest(c, api, record[0])
}

func (suite *suiteRemoveVirtualNetworkSite) TestWhenNoPreexistingVirtualNetworkSites(c *C) {
    // Prepare a basic, empty, configuration.
    existingConfig := &NetworkConfiguration{XMLNS: XMLNS_NC}
    responses := makeOKXMLResponse(c, existingConfig)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    err := api.RemoveVirtualNetworkSite("virtual-network-on-my-shoes")

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 1)
    assertGetNetworkConfigurationRequest(c, api, record[0])
}

func (suite *suiteRemoveVirtualNetworkSite) TestWhenPreexistingVirtualNetworkSites(c *C) {
    // Prepare a configuration with a single virtual network.
    existingConfig := &NetworkConfiguration{
        XMLNS: XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{
            {Name: MakeRandomVirtualNetworkName("test-")},
        },
    }
    responses := makeOKXMLResponse(c, existingConfig)
    responses = append(responses, DispatcherResponse{
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    virtualNetwork := &VirtualNetworkSite{Name: MakeRandomVirtualNetworkName("test-")}

    err := api.RemoveVirtualNetworkSite(virtualNetwork.Name)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 1)
    assertGetNetworkConfigurationRequest(c, api, record[0])
    // It didn't do anything, so no upload.
}

func (suite *suiteRemoveVirtualNetworkSite) TestWhenPreexistingVirtualNetworkSiteWithSameName(c *C) {
    // Prepare a configuration with a single virtual network.
    existingConfig := &NetworkConfiguration{
        XMLNS: XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{
            {Name: "virtual-network-bob"},
        },
    }
    responses := makeOKXMLResponse(c, existingConfig)
    responses = append(responses, DispatcherResponse{
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    virtualNetwork := &VirtualNetworkSite{Name: "virtual-network-bob"}

    err := api.RemoveVirtualNetworkSite(virtualNetwork.Name)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    assertGetNetworkConfigurationRequest(c, api, record[0])
    expected := &NetworkConfiguration{
        XMLNS:               XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{},
    }
    expectedBody, err := expected.Serialize()
    c.Assert(err, IsNil)
    assertSetNetworkConfigurationRequest(c, api, []byte(expectedBody), record[1])
}

func (suite *suiteRemoveVirtualNetworkSite) TestWhenConfigCannotBePushed(c *C) {
    existingConfig := &NetworkConfiguration{
        XMLNS: XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{
            {Name: "virtual-network-all-over"},
        },
    }
    responses := makeOKXMLResponse(c, existingConfig)
    responses = append(responses, DispatcherResponse{
        response: &x509Response{StatusCode: http.StatusInternalServerError},
    })
    rigPreparedResponseDispatcher(responses)

    err := makeAPI(c).RemoveVirtualNetworkSite("virtual-network-all-over")

    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
}

type suiteListRoleEndpoints struct{}

var _ = Suite(&suiteListRoleEndpoints{})

func (suite *suiteListRoleEndpoints) TestWhenNoExistingEndpoints(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &ListRoleEndpointsRequest{
        ServiceName:    "foo",
        DeploymentName: "foo",
        RoleName:       "foo"}
    endpoints, err := api.ListRoleEndpoints(request)

    c.Assert(err, IsNil)
    c.Assert(endpoints, DeepEquals, []InputEndpoint{})
    c.Check(record, HasLen, 1)
    // Check GetRole was performed.
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
}

func (suite *suiteListRoleEndpoints) TestWhenExistingEndpoints(c *C) {
    var err error
    endpoints := &[]InputEndpoint{
        {
            LocalPort: 123,
            Name:      "test123",
            Port:      1123,
        },
        {
            LocalPort: 456,
            Name:      "test456",
            Port:      4456,
        }}

    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                InputEndpoints:       endpoints,
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &ListRoleEndpointsRequest{
        ServiceName:    "foo",
        DeploymentName: "foo",
        RoleName:       "foo"}
    listedEndpoints, err := api.ListRoleEndpoints(request)

    c.Assert(err, IsNil)
    c.Assert(listedEndpoints, DeepEquals, *endpoints)
    c.Check(record, HasLen, 1)
    // Check GetRole was performed.
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
}

func (suite *suiteListRoleEndpoints) TestWhenGetRoleFails(c *C) {
    responses := []DispatcherResponse{
        // No role found.
        {response: &x509Response{StatusCode: http.StatusNotFound}},
    }
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &ListRoleEndpointsRequest{
        ServiceName:    "foo",
        DeploymentName: "foo",
        RoleName:       "foo"}
    _, err := api.ListRoleEndpoints(request)

    c.Check(err, ErrorMatches, "GET request failed [(]404: Not Found[)]")
    c.Check(record, HasLen, 1)
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
}

type suiteAddRoleEndpoints struct{}

var _ = Suite(&suiteAddRoleEndpoints{})

func (suite *suiteAddRoleEndpoints) TestWhenNoPreexistingEndpoints(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    responses = append(responses, DispatcherResponse{
        // Accept upload of new endpoints
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    endpoints := []InputEndpoint{
        {
            LocalPort: 999,
            Name:      "test999",
            Port:      1999,
        },
    }

    request := &AddRoleEndpointsRequest{
        ServiceName:    "foo",
        DeploymentName: "foo",
        RoleName:       "foo",
        InputEndpoints: endpoints}
    err = api.AddRoleEndpoints(request)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    // Check GetRole was performed.
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
    // Check UpdateRole was performed.
    existingRole.ConfigurationSets[0].InputEndpoints = &endpoints
    expectedXML, err := existingRole.Serialize()
    c.Assert(err, IsNil)
    assertUpdateRoleRequest(
        c, api, record[1], request.ServiceName, request.DeploymentName,
        request.RoleName, expectedXML)
}

func (suite *suiteAddRoleEndpoints) TestWhenPreexistingEndpoints(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                InputEndpoints: &[]InputEndpoint{
                    {
                        LocalPort: 123,
                        Name:      "test123",
                        Port:      1123,
                    },
                },
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    responses = append(responses, DispatcherResponse{
        // Accept upload of new endpoints
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    endpoints := []InputEndpoint{
        {
            LocalPort: 999,
            Name:      "test999",
            Port:      1999,
        },
    }

    request := &AddRoleEndpointsRequest{
        ServiceName:    "foo",
        DeploymentName: "foo",
        RoleName:       "foo",
        InputEndpoints: endpoints}
    err = api.AddRoleEndpoints(request)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    // Check GetRole was performed.
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
    // Check UpdateRole was performed.
    allEndpoints := append(
        *existingRole.ConfigurationSets[0].InputEndpoints, endpoints...)
    existingRole.ConfigurationSets[0].InputEndpoints = &allEndpoints
    expectedXML, err := existingRole.Serialize()
    c.Assert(err, IsNil)
    assertUpdateRoleRequest(
        c, api, record[1], request.ServiceName, request.DeploymentName,
        request.RoleName, expectedXML)
}

func (suite *suiteAddRoleEndpoints) TestWhenGetRoleFails(c *C) {
    responses := []DispatcherResponse{
        // No role found.
        {response: &x509Response{StatusCode: http.StatusNotFound}},
    }
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &AddRoleEndpointsRequest{
        ServiceName:    "foo",
        DeploymentName: "foo",
        RoleName:       "foo"}
    err := api.AddRoleEndpoints(request)

    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "GET request failed [(]404: Not Found[)]")
    c.Check(record, HasLen, 1)
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
}

func (suite *suiteAddRoleEndpoints) TestWhenUpdateFails(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    responses = append(responses, DispatcherResponse{
        // Cannot accept upload of new role endpoint
        response: &x509Response{StatusCode: http.StatusInternalServerError},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    endpoints := []InputEndpoint{
        {
            LocalPort: 999,
            Name:      "test999",
            Port:      1999,
        },
    }

    request := &AddRoleEndpointsRequest{
        ServiceName:    "foo",
        DeploymentName: "foo",
        RoleName:       "foo",
        InputEndpoints: endpoints}
    err = api.AddRoleEndpoints(request)

    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 2)
}

type suiteRemoveRoleEndpoints struct{}

var _ = Suite(&suiteRemoveRoleEndpoints{})

func (suite *suiteRemoveRoleEndpoints) TestWhenNoPreexistingEndpoints(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    responses = append(responses, DispatcherResponse{
        // Accept upload of new endpoints
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)
    endpoints := []InputEndpoint{
        {
            LocalPort: 999,
            Name:      "test999",
            Port:      1999,
        },
    }

    request := &RemoveRoleEndpointsRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
        RoleName:       "role-name",
        InputEndpoints: endpoints,
    }
    err = api.RemoveRoleEndpoints(request)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    // Check GetRole was performed.
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
    // Check UpdateRole was performed.
    expectedXML, err := existingRole.Serialize()
    c.Assert(err, IsNil)
    assertUpdateRoleRequest(
        c, api, record[1], request.ServiceName, request.DeploymentName,
        request.RoleName, expectedXML)
}

func (suite *suiteRemoveRoleEndpoints) TestWhenEndpointIsDefined(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                InputEndpoints: &[]InputEndpoint{
                    {
                        LocalPort: 123,
                        Name:      "test123",
                        Port:      1123,
                    },
                    {
                        LocalPort: 456,
                        Name:      "test456",
                        Port:      4456,
                    },
                },
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    responses = append(responses, DispatcherResponse{
        // Accept upload of new endpoints
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &RemoveRoleEndpointsRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
        RoleName:       "role-name",
        // Remove the first of the existing endppints.
        InputEndpoints: (*existingRole.ConfigurationSets[0].InputEndpoints)[:1],
    }
    err = api.RemoveRoleEndpoints(request)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    // Check GetRole was performed.
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
    // Check UpdateRole was performed.
    expected := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                InputEndpoints: &[]InputEndpoint{
                    (*existingRole.ConfigurationSets[0].InputEndpoints)[1],
                },
            },
        },
    }
    expectedXML, err := expected.Serialize()
    c.Assert(err, IsNil)
    assertUpdateRoleRequest(
        c, api, record[1], request.ServiceName, request.DeploymentName,
        request.RoleName, expectedXML)
}

func (suite *suiteRemoveRoleEndpoints) TestWhenAllEndpointsAreRemoved(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                InputEndpoints: &[]InputEndpoint{
                    {
                        LocalPort: 123,
                        Name:      "test123",
                        Port:      1123,
                    },
                },
            },
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    responses = append(responses, DispatcherResponse{
        // Accept upload of new endpoints
        response: &x509Response{StatusCode: http.StatusOK},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &RemoveRoleEndpointsRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
        RoleName:       "role-name",
        // Remove the first of the existing endppints.
        InputEndpoints: *existingRole.ConfigurationSets[0].InputEndpoints,
    }
    err = api.RemoveRoleEndpoints(request)

    c.Assert(err, IsNil)
    c.Check(record, HasLen, 2)
    // Check GetRole was performed.
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
    // Check UpdateRole was performed.
    expected := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                // InputEndpoints is nil, not the empty slice.
                InputEndpoints: nil,
            },
        },
    }
    expectedXML, err := expected.Serialize()
    c.Assert(err, IsNil)
    assertUpdateRoleRequest(
        c, api, record[1], request.ServiceName, request.DeploymentName,
        request.RoleName, expectedXML)
}

func (suite *suiteRemoveRoleEndpoints) TestWhenGetRoleFails(c *C) {
    responses := []DispatcherResponse{
        // No role found.
        {response: &x509Response{StatusCode: http.StatusNotFound}},
    }
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &RemoveRoleEndpointsRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
        RoleName:       "role-name",
    }
    err := api.RemoveRoleEndpoints(request)

    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "GET request failed [(]404: Not Found[)]")
    c.Check(record, HasLen, 1)
    assertGetRoleRequest(
        c, api, record[0], request.ServiceName, request.DeploymentName,
        request.RoleName)
}

func (suite *suiteRemoveRoleEndpoints) TestWhenUpdateFails(c *C) {
    var err error
    existingRole := &PersistentVMRole{
        ConfigurationSets: []ConfigurationSet{
            {ConfigurationSetType: CONFIG_SET_NETWORK},
        },
    }
    responses := makeOKXMLResponse(c, existingRole)
    responses = append(responses, DispatcherResponse{
        // Cannot accept upload of new role endpoint
        response: &x509Response{StatusCode: http.StatusInternalServerError},
    })
    record := []*X509Request{}
    rigRecordingPreparedResponseDispatcher(&record, responses)
    api := makeAPI(c)

    request := &RemoveRoleEndpointsRequest{
        ServiceName:    "service-name",
        DeploymentName: "deployment-name",
        RoleName:       "role-name",
    }
    err = api.RemoveRoleEndpoints(request)

    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
    c.Check(record, HasLen, 2)
}

type suiteCompareInputEndpoints struct{}

var _ = Suite(&suiteCompareInputEndpoints{})

func (suite *suiteCompareInputEndpoints) TestEqualWhenEmpty(c *C) {
    a := &InputEndpoint{}
    b := &InputEndpoint{}
    c.Assert(CompareInputEndpoints(a, b), Equals, true)
}

func (suite *suiteCompareInputEndpoints) TestEquality(c *C) {
    checkComparison := func(a, b InputEndpoint, expected bool) {
        c.Check(CompareInputEndpoints(&a, &b), Equals, expected)
    }
    // Name has no influence on comparison.
    checkComparison(
        InputEndpoint{Name: "foo"},
        InputEndpoint{Name: "bar"},
        true)
    // LoadBalancerProbe has no influence on comparison.
    checkComparison(
        InputEndpoint{
            LoadBalancerProbe: &LoadBalancerProbe{Path: "foo"},
        },
        InputEndpoint{
            LoadBalancerProbe: &LoadBalancerProbe{Path: "bar"},
        },
        true,
    )
    // Port influences comparisons.
    checkComparison(
        InputEndpoint{Port: 1234},
        InputEndpoint{Port: 1234},
        true)
    checkComparison(
        InputEndpoint{Port: 1234},
        InputEndpoint{Port: 5678},
        false)
    // Protocol influences comparisons.
    checkComparison(
        InputEndpoint{Protocol: "TCP"},
        InputEndpoint{Protocol: "TCP"},
        true)
    checkComparison(
        InputEndpoint{Protocol: "TCP"},
        InputEndpoint{Protocol: "UDP"},
        false)
    // VIP influences comparisons.
    checkComparison(
        InputEndpoint{VIP: "1.2.3.4"},
        InputEndpoint{VIP: "1.2.3.4"},
        true)
    checkComparison(
        InputEndpoint{VIP: "1.2.3.4"},
        InputEndpoint{VIP: "5.6.7.8"},
        false)
    // LoadBalancedEndpointSetName influences comparisons.
    checkComparison(
        InputEndpoint{LoadBalancedEndpointSetName: "foo"},
        InputEndpoint{LoadBalancedEndpointSetName: "foo"},
        true)
    checkComparison(
        InputEndpoint{LoadBalancedEndpointSetName: "foo"},
        InputEndpoint{LoadBalancedEndpointSetName: "bar"},
        false)
    // LocalPort influences comparisons only when LoadBalancedEndpointSetName
    // is not the empty string.
    checkComparison(
        InputEndpoint{LocalPort: 1234},
        InputEndpoint{LocalPort: 5678},
        true)
    checkComparison(
        InputEndpoint{LoadBalancedEndpointSetName: "foo", LocalPort: 1234},
        InputEndpoint{LoadBalancedEndpointSetName: "foo", LocalPort: 5678},
        false)
}
