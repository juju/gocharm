// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/xml"
    "fmt"
    "net/http"
    "strings"
    "time"
)

// Note: each API call is required to include a version string in the request header.
// These may often be the same string, but need to be kept as strings rather than being
// pulled out and replaced with a constant, each API call may be individually changed.

type ManagementAPI struct {
    session *x509Session
    // The interval used when polling the server.
    // Set this to 0 to prevent polling from happening.  When polling is
    // disabled, the API methods return immediately after the request has
    // been issued so the caller's code will have to manually deal with the
    // possibility that the triggered asynchronous operation might still
    // not be completed.
    PollerInterval time.Duration

    // The duration after which the polling is terminated.
    PollerTimeout time.Duration
}

// The default interval used when polling the server to get the status of a
// running operation.
const DefaultPollerInterval = 10 * time.Second

// The default duration after which the polling is terminated.
const DefaultPollerTimeout = 20 * time.Minute

// NewManagementAPIWithRetryPolicy creates an object used to interact with
// Windows Azure's API.
// http://msdn.microsoft.com/en-us/library/windowsazure/ff800682.aspx
func NewManagementAPIWithRetryPolicy(subscriptionId, certFile, location string, policy RetryPolicy) (*ManagementAPI, error) {
    session, err := newX509Session(subscriptionId, certFile, location, policy)
    if err != nil {
        return nil, err
    }
    api := ManagementAPI{session, DefaultPollerInterval, DefaultPollerTimeout}
    return &api, nil
}

// NewManagementAPI creates an object used to interact with Windows Azure's API.
// http://msdn.microsoft.com/en-us/library/windowsazure/ff800682.aspx
func NewManagementAPI(subscriptionId, certFile, location string) (*ManagementAPI, error) {
    return NewManagementAPIWithRetryPolicy(subscriptionId, certFile, location, NoRetryPolicy)
}

var operationIDHeaderName = http.CanonicalHeaderKey("x-ms-request-id")

// getOperationID extracts the Windows Azure operation ID from the headers
// of the given x509Response.
func getOperationID(response *x509Response) (string, error) {
    header := response.Header[operationIDHeaderName]
    if header != nil && len(header) != 0 {
        return header[0], nil
    }
    err := fmt.Errorf("no operation header (%v) found in response", operationIDHeaderName)
    return "", err
}

func (api *ManagementAPI) GetRetryPolicy() RetryPolicy {
    return api.session.retryPolicy
}

// blockUntilCompleted blocks and polls for completion of an Azure operation.
// The "response" parameter is the result of the request that started the
// operation.  If the response says that the operation is running
// asynchronously, this function will block and poll until the operation is
// finished.  On the other hand, if the response was a failure or a synchronous
// result, the function returns immediately.
func (api *ManagementAPI) blockUntilCompleted(response *x509Response) error {
    switch response.StatusCode {
    case http.StatusAccepted:
        // Asynchronous.  Fall out of the switch and start blocking.
    case http.StatusOK, http.StatusCreated, http.StatusNoContent:
        // Simple success.  Sometimes it happens; enjoy it.
        return nil
    default:
        // Request failed, synchronously.
        return newHTTPError(response.StatusCode, response.Body, "request failed")
    }

    if api.PollerInterval == 0 {
        // Polling has been disabled for test purposes.  Return immediately.
        return nil
    }
    operationID, err := getOperationID(response)
    if err != nil {
        return fmt.Errorf("could not interpret asynchronous response: %v", err)
    }
    poller := newOperationPoller(api, operationID)
    operation, err := performOperationPolling(poller, api.PollerInterval, api.PollerTimeout)
    if err != nil {
        return err
    }
    if operation.Status != SucceededOperationStatus {
        return newAzureErrorFromOperation(operation)
    }
    return nil
}

// ListOSImages retrieves the list of available operating system disk images
// from the Azure management API.
// Images are returned in the order in which Azure lists them.
// http://msdn.microsoft.com/en-us/library/windowsazure/jj157191.aspx
func (api *ManagementAPI) ListOSImages() (*Images, error) {
    response, err := api.session.get("services/images", "2013-03-01")
    if err != nil {
        return nil, err
    }
    images := Images{}
    err = xml.Unmarshal(response.Body, &images)
    return &images, err
}

// ListHostedServices loads a list of HostedServiceDescriptor objects from the
// Azure management API.
// HostedServiceDescriptor objects contains a small subset of the fields present in
// HostedService objects.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460781.aspx
func (api *ManagementAPI) ListHostedServices() ([]HostedServiceDescriptor, error) {
    res, err := api.session.get("services/hostedservices", "2013-10-01")
    if err != nil {
        return nil, err
    }
    hostedServices := HostedServiceDescriptorList{}
    err = xml.Unmarshal(res.Body, &hostedServices)
    return hostedServices.HostedServices, err
}

// UpdateHostedService updates the provided values on the named service.
// Use NewUpdateHostedService() to create an UpdateHostedService params object.
// See http://msdn.microsoft.com/en-us/library/windowsazure/gg441303.aspx
func (api *ManagementAPI) UpdateHostedService(serviceName string, params *UpdateHostedService) error {
    var err error
    checkPathComponents(serviceName)
    URI := "services/hostedservices/" + serviceName
    body, err := params.Serialize()
    if err != nil {
        return err
    }
    response, err := api.session.put(URI, "2013-10-01", []byte(body), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

// GetHostedServiceProperties loads a HostedService object from the Azure
// management API.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460806.aspx
func (api *ManagementAPI) GetHostedServiceProperties(
    serviceName string, embedDetail bool) (*HostedService, error) {
    checkPathComponents(serviceName)
    URI := "services/hostedservices/" + serviceName + "?embed-detail="
    switch embedDetail {
    case true:
        URI += "true"
    case false:
        URI += "false"
    }
    res, err := api.session.get(URI, "2013-10-01")
    if err != nil {
        return nil, err
    }
    hostedService := HostedService{}
    err = xml.Unmarshal(res.Body, &hostedService)
    return &hostedService, err
}

// AddHostedService adds a hosted service.
// This is an asynchronous operation on Azure, but this call blocks until the
// operation is completed.
// This is actually called CreateHostedService in the Azure documentation.
// See http://msdn.microsoft.com/en-us/library/windowsazure/gg441304.aspx
func (api *ManagementAPI) AddHostedService(definition *CreateHostedService) error {
    URI := "services/hostedservices"
    body, err := marshalXML(definition)
    if err != nil {
        return err
    }
    response, err := api.session.post(URI, "2013-10-01", []byte(body), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

// CheckHostedServiceNameAvailability looks to see if the supplied name is
// acceptable to use as a cloud service name. It returns nil if it is available
// or an error containing the reason if it is not.  Names may not be acceptable
// based on reserved words, trademarks and profanity.
// See http://msdn.microsoft.com/en-us/library/windowsazure/jj154116.aspx
func (api *ManagementAPI) CheckHostedServiceNameAvailability(name string) error {
    var err error
    response, err := api.session.get(
        "services/hostedservices/operations/isavailable/"+name, "2013-10-01")
    if err != nil {
        return err
    }

    availability := &AvailabilityResponse{}
    err = availability.Deserialize(response.Body)
    if err != nil {
        return err
    }
    if strings.ToLower(availability.Result) == "true" {
        return nil
    }
    return fmt.Errorf(availability.Reason)
}

// DeleteHostedService deletes the named hosted service.
// See http://msdn.microsoft.com/en-us/library/windowsazure/gg441305.aspx
func (api *ManagementAPI) DeleteHostedService(serviceName string) error {
    response, err := api.session.delete("services/hostedservices/"+serviceName, "2010-10-28")
    if err != nil {
        if IsNotFoundError(err) {
            return nil
        }
        return err
    }
    return api.blockUntilCompleted(response)
}

// AddDeployment adds a virtual machine deployment.
// This is an asynchronous operation on Azure, but this call blocks until the
// operation is completed.
// This is actually called CreateDeployment in the Azure documentation.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460813.aspx
func (api *ManagementAPI) AddDeployment(definition *Deployment, serviceName string) error {
    checkPathComponents(serviceName)
    URI := "services/hostedservices/" + serviceName + "/deployments"
    body, err := marshalXML(definition)
    if err != nil {
        return err
    }
    response, err := api.session.post(URI, "2013-10-01", []byte(body), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

// DeleteDeployment deletes the named deployment from the named hosted service.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460815.aspx
func (api *ManagementAPI) DeleteDeployment(serviceName string, deploymentName string) error {
    path := "services/hostedservices/" + serviceName + "/deployments/" + deploymentName
    response, err := api.session.delete(path, "2013-10-01")
    if err != nil {
        if IsNotFoundError(err) {
            return nil
        }
        return err
    }
    return api.blockUntilCompleted(response)
}

type GetDeploymentRequest struct {
    ServiceName    string
    DeploymentName string
}

// GetDeployment returns a Deployment object for the named hosted service and
// deployment name.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460804.aspx
func (api *ManagementAPI) GetDeployment(request *GetDeploymentRequest) (*Deployment, error) {
    checkPathComponents(request.ServiceName)
    checkPathComponents(request.DeploymentName)
    path := "services/hostedservices/" + request.ServiceName + "/deployments/" + request.DeploymentName
    response, err := api.session.get(path, "2013-10-01")
    if err != nil {
        return nil, err
    }
    deployment := Deployment{}
    err = deployment.Deserialize(response.Body)
    if err != nil {
        return nil, err
    }
    return &deployment, nil
}

// AddStorageAccount starts the creation of a storage account.  This is
// called a storage service in the Azure API, but nomenclature seems to
// have changed.
// This is an asynchronous operation on Azure, but the call blocks until the
// operation is completed.
// This is actually called CreateStorageAccount in the Azure documentation.
// See http://msdn.microsoft.com/en-us/library/windowsazure/hh264518.aspx
func (api *ManagementAPI) AddStorageAccount(definition *CreateStorageServiceInput) error {
    uri := "services/storageservices"
    body, err := marshalXML(definition)
    if err != nil {
        return err
    }
    response, err := api.session.post(uri, "2013-10-01", []byte(body), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

// DeleteStorageAccount deletes a storage account.
// See http://msdn.microsoft.com/en-us/library/windowsazure/hh264517.aspx
func (api *ManagementAPI) DeleteStorageAccount(storageAccountName string) error {
    response, err := api.session.delete("services/storageservices/"+storageAccountName, "2011-06-01")
    if err != nil {
        if IsNotFoundError(err) {
            return nil
        }
        return err
    }
    return api.blockUntilCompleted(response)
}

// GetStorageAccountKeys retrieves a storage account's primary and secondary
// access keys from the Azure service.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460785.aspx
func (api *ManagementAPI) GetStorageAccountKeys(accountName string) (*StorageAccountKeys, error) {
    url := "services/storageservices/" + accountName + "/keys"
    res, err := api.session.get(url, "2009-10-01")
    if err != nil {
        return nil, err
    }
    keys := StorageAccountKeys{}
    err = keys.Deserialize(res.Body)
    if err != nil {
        return nil, err
    }
    return &keys, nil
}

type DeleteDiskRequest struct {
    DiskName   string // Name of the disk to delete.
    DeleteBlob bool   // Whether to delete the associated blob storage.
}

// DeleteDisk deletes the named OS/data disk.
// See http://msdn.microsoft.com/en-us/library/windowsazure/jj157200.aspx
func (api *ManagementAPI) DeleteDisk(request *DeleteDiskRequest) error {
    // Use the disk deletion poller to work around a bug in Windows Azure.
    // See the documentation in the file deletedisk.go for details.
    poller := diskDeletePoller{api, request.DiskName, request.DeleteBlob}
    _, err := performPolling(poller, deleteDiskInterval, deleteDiskTimeout)
    return err
}

func (api *ManagementAPI) _DeleteDisk(diskName string, deleteBlob bool) error {
    url := "services/disks/" + diskName
    if deleteBlob {
        url = addURLQueryParams(url, "comp", "media")
    }
    response, err := api.session.delete(url, "2012-08-01")
    if err != nil {
        if IsNotFoundError(err) {
            return nil
        }
        return err
    }
    return api.blockUntilCompleted(response)
}

// Perform an operation on the specified role (as defined by serviceName,
// deploymentName and roleName) This is an asynchronous operation on Azure, but
// the call blocks until the operation is completed.
func (api *ManagementAPI) performRoleOperation(serviceName, deploymentName, roleName, apiVersion string, operation *RoleOperation) error {
    checkPathComponents(serviceName, deploymentName, roleName)
    URI := "services/hostedservices/" + serviceName + "/deployments/" + deploymentName + "/roleinstances/" + roleName + "/Operations"
    body, err := marshalXML(operation)
    if err != nil {
        return err
    }
    response, err := api.session.post(URI, apiVersion, []byte(body), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

type performRoleOperationRequest struct {
    ServiceName    string
    DeploymentName string
    RoleName       string
}

type StartRoleRequest performRoleOperationRequest

// StartRole starts the named Role.
// See http://msdn.microsoft.com/en-us/library/windowsazure/jj157189.aspx
func (api *ManagementAPI) StartRole(request *StartRoleRequest) error {
    return api.performRoleOperation(
        request.ServiceName, request.DeploymentName, request.RoleName,
        "2013-10-01", startRoleOperation)
}

type RestartRoleRequest performRoleOperationRequest

// RestartRole restarts the named Role.
// See http://msdn.microsoft.com/en-us/library/windowsazure/jj157197.aspx
func (api *ManagementAPI) RestartRole(request *RestartRoleRequest) error {
    return api.performRoleOperation(
        request.ServiceName, request.DeploymentName, request.RoleName,
        "2013-10-01", restartRoleOperation)
}

type ShutdownRoleRequest performRoleOperationRequest

// ShutdownRole shuts down the named Role.
// See http://msdn.microsoft.com/en-us/library/windowsazure/jj157195.aspx
func (api *ManagementAPI) ShutdownRole(request *ShutdownRoleRequest) error {
    return api.performRoleOperation(
        request.ServiceName, request.DeploymentName, request.RoleName,
        "2013-10-01", shutdownRoleOperation)
}

type GetRoleRequest performRoleOperationRequest

// GetRole requests the role data for the specified role.
// See http://msdn.microsoft.com/en-us/library/windowsazure/jj157193.aspx
func (api *ManagementAPI) GetRole(request *GetRoleRequest) (*PersistentVMRole, error) {
    checkPathComponents(
        request.ServiceName, request.DeploymentName, request.RoleName)
    url := ("services/hostedservices/" + request.ServiceName +
        "/deployments/" + request.DeploymentName + "/roles/" +
        request.RoleName)
    response, err := api.session.get(url, "2013-10-01")
    if err != nil {
        return nil, err
    }
    role := PersistentVMRole{}
    err = role.Deserialize(response.Body)
    if err != nil {
        return nil, err
    }
    return &role, nil
}

type UpdateRoleRequest struct {
    // It would be nice to inherit ServiceName, DeploymentName and RoleName
    // from performRoleOperationRequest... alas, struct embedding is too
    // clunky, so copy-n-paste it is. My kingdom for a macro!
    ServiceName      string
    DeploymentName   string
    RoleName         string
    PersistentVMRole *PersistentVMRole
}

// UpdateRole pushes a PersistentVMRole document back up to Azure for the
// specified role. See
// http://msdn.microsoft.com/en-us/library/windowsazure/jj157187.aspx
func (api *ManagementAPI) UpdateRole(request *UpdateRoleRequest) error {
    checkPathComponents(
        request.ServiceName, request.DeploymentName, request.RoleName)
    url := ("services/hostedservices/" + request.ServiceName +
        "/deployments/" + request.DeploymentName + "/roles/" +
        request.RoleName)
    role, err := request.PersistentVMRole.Serialize()
    if err != nil {
        return err
    }
    response, err := api.session.put(url, "2013-10-01", []byte(role), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

type CreateAffinityGroupRequest struct {
    CreateAffinityGroup *CreateAffinityGroup
}

// CreateAffinityGroup sends a request to make a new affinity group.
// See http://msdn.microsoft.com/en-us/library/windowsazure/gg715317.aspx
func (api *ManagementAPI) CreateAffinityGroup(request *CreateAffinityGroupRequest) error {
    var err error
    url := "affinitygroups"
    body, err := request.CreateAffinityGroup.Serialize()
    if err != nil {
        return err
    }
    response, err := api.session.post(url, "2013-10-01", []byte(body), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

type UpdateAffinityGroupRequest struct {
    Name                string
    UpdateAffinityGroup *UpdateAffinityGroup
}

// UpdateAffinityGroup sends a request to update the named affinity group.
// See http://msdn.microsoft.com/en-us/library/windowsazure/gg715316.aspx
func (api *ManagementAPI) UpdateAffinityGroup(request *UpdateAffinityGroupRequest) error {
    var err error
    checkPathComponents(request.Name)
    url := "affinitygroups/" + request.Name
    body, err := request.UpdateAffinityGroup.Serialize()
    if err != nil {
        return err
    }
    response, err := api.session.put(url, "2011-02-25", []byte(body), "application/xml")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}

type DeleteAffinityGroupRequest struct {
    Name string
}

// DeleteAffinityGroup requests a deletion of the named affinity group.
// See http://msdn.microsoft.com/en-us/library/windowsazure/gg715314.aspx
func (api *ManagementAPI) DeleteAffinityGroup(request *DeleteAffinityGroupRequest) error {
    checkPathComponents(request.Name)
    url := "affinitygroups/" + request.Name
    response, err := api.session.delete(url, "2011-02-25")
    if err != nil {
        if IsNotFoundError(err) {
            return nil
        }
        return err
    }
    return api.blockUntilCompleted(response)
}

// GetNetworkConfiguration gets the network configuration for this
// subscription. If there is no network configuration the configuration will
// be nil.
// See http://msdn.microsoft.com/en-us/library/windowsazure/jj157196.aspx
func (api *ManagementAPI) GetNetworkConfiguration() (*NetworkConfiguration, error) {
    response, err := api.session.get("services/networking/media", "2013-10-01")
    if err != nil {
        if IsNotFoundError(err) {
            return nil, nil
        }
        return nil, err
    }
    networkConfig := NetworkConfiguration{}
    err = networkConfig.Deserialize(response.Body)
    if err != nil {
        return nil, err
    }
    return &networkConfig, nil
}

// SetNetworkConfiguration sets the network configuration for this
// subscription. See:
// http://msdn.microsoft.com/en-us/library/windowsazure/jj157181.aspx
func (api *ManagementAPI) SetNetworkConfiguration(cfg *NetworkConfiguration) error {
    var err error
    body, err := cfg.Serialize()
    if err != nil {
        return err
    }
    response, err := api.session.put(
        "services/networking/media", "2013-10-01", []byte(body),
        "application/octet-stream")
    if err != nil {
        return err
    }
    return api.blockUntilCompleted(response)
}
