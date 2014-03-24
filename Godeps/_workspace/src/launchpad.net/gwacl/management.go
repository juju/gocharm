// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    "sort"
    "strings"
)

type ListInstancesRequest struct {
    ServiceName string
}

// ListInstances returns a slice of all instances for all deployments for the
// given hosted service name.
func (api *ManagementAPI) ListInstances(request *ListInstancesRequest) ([]RoleInstance, error) {
    instances := []RoleInstance{}
    properties, err := api.GetHostedServiceProperties(request.ServiceName, true)
    if err != nil {
        return nil, err
    }
    for _, deployment := range properties.Deployments {
        instances = append(instances, deployment.RoleInstanceList...)
    }
    return instances, nil
}

// ListAllDeploymentsRequest is a parameter object for ListAllDeployments.
type ListAllDeploymentsRequest struct {
    // ServiceName restricts the listing to the given service.
    ServiceName string
}

// ListAllDeployments returns a slice containing all deployments that match
// the request.
func (api *ManagementAPI) ListAllDeployments(request *ListAllDeploymentsRequest) ([]Deployment, error) {
    properties, err := api.GetHostedServiceProperties(request.ServiceName, true)
    if err != nil {
        return nil, err
    }
    return properties.Deployments, nil
}

// ListDeploymentsRequest is a parameter object for ListDeployments.
type ListDeploymentsRequest struct {
    // ServiceName restricts the listing to the given service.
    ServiceName string
    // DeploymentNames is a set (its value type is ignored) that restricts the
    // listing to those deployments which it contains.
    DeploymentNames []string
}

// ListDeployments returns a slice containing specific deployments, insofar
// as they match the request.
func (api *ManagementAPI) ListDeployments(request *ListDeploymentsRequest) ([]Deployment, error) {
    properties, err := api.GetHostedServiceProperties(request.ServiceName, true)
    if err != nil {
        return nil, err
    }
    // Filter the deployment list according to the given names.
    filter := make(map[string]bool)
    for _, name := range request.DeploymentNames {
        filter[name] = true
    }
    deployments := []Deployment{}
    for _, deployment := range properties.Deployments {
        if _, ok := filter[deployment.Name]; ok {
            deployments = append(deployments, deployment)
        }
    }
    return deployments, nil
}

type ListSpecificHostedServicesRequest struct {
    ServiceNames []string
}

// ListSpecificHostedServices returns a slice containing specific
// HostedServiceDescriptor objects, insofar as they match the request.
func (api *ManagementAPI) ListSpecificHostedServices(request *ListSpecificHostedServicesRequest) ([]HostedServiceDescriptor, error) {
    allServices, err := api.ListHostedServices()
    if err != nil {
        return nil, err
    }
    // Filter the service list according to the given names.
    filter := make(map[string]bool)
    for _, name := range request.ServiceNames {
        filter[name] = true
    }
    services := []HostedServiceDescriptor{}
    for _, service := range allServices {
        if _, ok := filter[service.ServiceName]; ok {
            services = append(services, service)
        }
    }
    return services, nil
}

type ListPrefixedHostedServicesRequest struct {
    ServiceNamePrefix string
}

// ListPrefixedHostedServices returns a slice containing specific
// HostedServiceDescriptor objects, insofar as they match the request.
func (api *ManagementAPI) ListPrefixedHostedServices(request *ListPrefixedHostedServicesRequest) ([]HostedServiceDescriptor, error) {
    services, err := api.ListHostedServices()
    if err != nil {
        return nil, err
    }
    resServices := []HostedServiceDescriptor{}
    for _, service := range services {
        if strings.HasPrefix(service.ServiceName, request.ServiceNamePrefix) {
            resServices = append(resServices, service)
        }
    }
    services = resServices
    return services, nil
}

type DestroyDeploymentRequest struct {
    ServiceName    string
    DeploymentName string
}

// DestroyDeployment brings down all resources within a deployment - running
// instances, disks, etc. - and deletes the deployment itself.
func (api *ManagementAPI) DestroyDeployment(request *DestroyDeploymentRequest) error {
    deployment, err := api.GetDeployment(&GetDeploymentRequest{
        ServiceName:    request.ServiceName,
        DeploymentName: request.DeploymentName,
    })
    if err != nil {
        if IsNotFoundError(err) {
            return nil
        }
        return err
    }
    // 1. Get the list of the VM disks.
    diskNameMap := make(map[string]bool)
    for _, role := range deployment.RoleList {
        for _, osVHD := range role.OSVirtualHardDisk {
            diskNameMap[osVHD.DiskName] = true
        }
    }
    // 2. Delete deployment.  This will delete all the role instances inside
    // this deployment as a side effect.
    err = api.DeleteDeployment(request.ServiceName, request.DeploymentName)
    if err != nil && !IsNotFoundError(err) {
        return err
    }
    // Sort the disk names to aid testing.
    diskNames := []string{}
    for diskName := range diskNameMap {
        diskNames = append(diskNames, diskName)
    }
    sort.Strings(diskNames)
    // 3. Delete the disks.
    for _, diskName := range diskNames {
        err = api.DeleteDisk(&DeleteDiskRequest{
            DiskName:   diskName,
            DeleteBlob: true})
        if err != nil && !IsNotFoundError(err) {
            return err
        }
    }
    // Done.
    return nil
}

type DestroyHostedServiceRequest struct {
    ServiceName string
}

// DestroyHostedService destroys all of the hosted service's contained
// deployments then deletes the hosted service itself.
func (api *ManagementAPI) DestroyHostedService(request *DestroyHostedServiceRequest) error {
    // 1. Get properties.
    properties, err := api.GetHostedServiceProperties(request.ServiceName, true)
    if err != nil {
        if IsNotFoundError(err) {
            return nil
        }
        return err
    }
    // 2. Delete deployments.
    for _, deployment := range properties.Deployments {
        err := api.DestroyDeployment(&DestroyDeploymentRequest{
            ServiceName:    request.ServiceName,
            DeploymentName: deployment.Name,
        })
        if err != nil {
            return err
        }
    }
    // 3. Delete service.
    err = api.DeleteHostedService(request.ServiceName)
    if err != nil && !IsNotFoundError(err) {
        return err
    }
    // Done.
    return nil
}

func (api *ManagementAPI) AddVirtualNetworkSite(site *VirtualNetworkSite) error {
    // Obtain the current network config, which we will then modify.
    networkConfig, err := api.GetNetworkConfiguration()
    if err != nil {
        return err
    }
    if networkConfig == nil {
        // There's no config yet.
        networkConfig = &NetworkConfiguration{XMLNS: XMLNS_NC}
    }
    if networkConfig.VirtualNetworkSites == nil {
        networkConfig.VirtualNetworkSites = &[]VirtualNetworkSite{}
    }
    // Check to see if this network already exists.
    for _, existingSite := range *networkConfig.VirtualNetworkSites {
        if existingSite.Name == site.Name {
            // Network already defined.
            return fmt.Errorf("could not add virtual network: %q already exists", site.Name)
        }
    }
    // Add the network to the configuration.
    virtualNetworkSites := append(*networkConfig.VirtualNetworkSites, *site)
    networkConfig.VirtualNetworkSites = &virtualNetworkSites
    // Put it back up to Azure. There's a race here...
    return api.SetNetworkConfiguration(networkConfig)
}

func (api *ManagementAPI) RemoveVirtualNetworkSite(siteName string) error {
    // Obtain the current network config, which we will then modify.
    networkConfig, err := api.GetNetworkConfiguration()
    if err != nil {
        return err
    }
    if networkConfig == nil || networkConfig.VirtualNetworkSites == nil {
        // There's no config, nothing to do.
        return nil
    }
    // Remove all references to the specified virtual network site name.
    virtualNetworkSites := []VirtualNetworkSite{}
    for _, existingSite := range *networkConfig.VirtualNetworkSites {
        if existingSite.Name != siteName {
            virtualNetworkSites = append(virtualNetworkSites, existingSite)
        }
    }
    if len(virtualNetworkSites) < len(*networkConfig.VirtualNetworkSites) {
        // Put it back up to Azure. There's a race here...
        networkConfig.VirtualNetworkSites = &virtualNetworkSites
        return api.SetNetworkConfiguration(networkConfig)
    }
    return nil
}

type ListRoleEndpointsRequest struct {
    ServiceName    string
    DeploymentName string
    RoleName       string
}

// ListRoleEndpoints lists the open endpoints for the named service/deployment/role name.
func (api *ManagementAPI) ListRoleEndpoints(request *ListRoleEndpointsRequest) ([]InputEndpoint, error) {
    var err error
    vmRole, err := api.GetRole(&GetRoleRequest{
        ServiceName:    request.ServiceName,
        DeploymentName: request.DeploymentName,
        RoleName:       request.RoleName})

    if err != nil {
        return nil, err
    }

    for i, configSet := range vmRole.ConfigurationSets {
        if configSet.ConfigurationSetType == CONFIG_SET_NETWORK {
            endpointsP := vmRole.ConfigurationSets[i].InputEndpoints
            if endpointsP != nil {
                return *endpointsP, nil
            }
        }
    }
    return []InputEndpoint{}, nil
}

type AddRoleEndpointsRequest struct {
    ServiceName    string
    DeploymentName string
    RoleName       string
    InputEndpoints []InputEndpoint
}

// AddRoleEndpoints appends the supplied endpoints to the existing endpoints
// for the named service/deployment/role name.  Note that the Azure API
// leaves this open to a race condition between fetching and updating the role.
func (api *ManagementAPI) AddRoleEndpoints(request *AddRoleEndpointsRequest) error {
    var err error
    vmRole, err := api.GetRole(&GetRoleRequest{
        ServiceName:    request.ServiceName,
        DeploymentName: request.DeploymentName,
        RoleName:       request.RoleName})

    if err != nil {
        return err
    }

    for i, configSet := range vmRole.ConfigurationSets {
        // TODO: Is NetworkConfiguration always present?
        if configSet.ConfigurationSetType == CONFIG_SET_NETWORK {
            endpointsP := vmRole.ConfigurationSets[i].InputEndpoints
            if endpointsP == nil {
                // No endpoints set at all, initialise it to be empty.
                vmRole.ConfigurationSets[i].InputEndpoints = &[]InputEndpoint{}
            }
            // Append to existing endpoints.
            // TODO: Check clashing endpoint. LocalPort/Name/Port unique?
            endpoints := append(
                *vmRole.ConfigurationSets[i].InputEndpoints,
                request.InputEndpoints...)
            vmRole.ConfigurationSets[i].InputEndpoints = &endpoints

            break // Only one NetworkConfiguration so exit loop now.
        }
    }

    // Enjoy this race condition.
    err = api.UpdateRole(&UpdateRoleRequest{
        ServiceName:      request.ServiceName,
        DeploymentName:   request.DeploymentName,
        RoleName:         request.RoleName,
        PersistentVMRole: vmRole})
    if err != nil {
        return err
    }
    return nil
}

// CompareInputEndpoints attempts to compare two InputEndpoint objects in a
// way that's congruent with how Windows's Azure considers them. The name is
// always ignored, as is LoadBalancerProbe. When LoadBalancedEndpointSetName
// is set (not the empty string), all the fields - LocalPort, Port, Protocol,
// VIP and LoadBalancedEndpointSetName - are used for the comparison. When
// LoadBalancedEndpointSetName is the empty string, all except LocalPort -
// effectively Port, Protocol and VIP - are used for comparison.
func CompareInputEndpoints(a, b *InputEndpoint) bool {
    if a.LoadBalancedEndpointSetName == "" {
        return a.Port == b.Port && a.Protocol == b.Protocol && a.VIP == b.VIP
    } else {
        return (a.LoadBalancedEndpointSetName == b.LoadBalancedEndpointSetName &&
            a.LocalPort == b.LocalPort && a.Port == b.Port &&
            a.Protocol == b.Protocol && a.VIP == b.VIP)
    }
}

type RemoveRoleEndpointsRequest struct {
    ServiceName    string
    DeploymentName string
    RoleName       string
    InputEndpoints []InputEndpoint
}

// RemoveRoleEndpoints attempts to remove the given endpoints from the
// specified role. It uses `CompareInputEndpoints` to determine when there's a
// match between the given endpoint and one already configured.
func (api *ManagementAPI) RemoveRoleEndpoints(request *RemoveRoleEndpointsRequest) error {
    filterRequest := filterRoleEndpointsRequest{
        ServiceName:    request.ServiceName,
        DeploymentName: request.DeploymentName,
        RoleName:       request.RoleName,
        Filter: func(a *InputEndpoint) bool {
            for _, b := range request.InputEndpoints {
                if CompareInputEndpoints(a, &b) {
                    return false
                }
            }
            return true
        },
    }
    return api.filterRoleEndpoints(&filterRequest)
}

// Returns true to keep the endpoint defined in the role's configuration,
// false to remove it. It is also welcome to mutate endpoints; they are passed
// by reference.
type inputEndpointFilter func(*InputEndpoint) bool

type filterRoleEndpointsRequest struct {
    ServiceName    string
    DeploymentName string
    RoleName       string
    Filter         inputEndpointFilter
}

// filterRoleEndpoints is a general role endpoint filtering function. It is
// private because it is only a support function for RemoveRoleEndpoints, and
// is not tested directly.
func (api *ManagementAPI) filterRoleEndpoints(request *filterRoleEndpointsRequest) error {
    role, err := api.GetRole(&GetRoleRequest{
        ServiceName:    request.ServiceName,
        DeploymentName: request.DeploymentName,
        RoleName:       request.RoleName,
    })
    if err != nil {
        return err
    }
    for index, configSet := range role.ConfigurationSets {
        if configSet.ConfigurationSetType == CONFIG_SET_NETWORK {
            if configSet.InputEndpoints != nil {
                endpoints := []InputEndpoint{}
                for _, existingEndpoint := range *configSet.InputEndpoints {
                    if request.Filter(&existingEndpoint) {
                        endpoints = append(endpoints, existingEndpoint)
                    }
                }
                if len(endpoints) == 0 {
                    configSet.InputEndpoints = nil
                } else {
                    configSet.InputEndpoints = &endpoints
                }
            }
        }
        // Update the role; implicit copying is a nuisance.
        role.ConfigurationSets[index] = configSet
    }
    return api.UpdateRole(&UpdateRoleRequest{
        ServiceName:      request.ServiceName,
        DeploymentName:   request.DeploymentName,
        RoleName:         request.RoleName,
        PersistentVMRole: role,
    })
}
