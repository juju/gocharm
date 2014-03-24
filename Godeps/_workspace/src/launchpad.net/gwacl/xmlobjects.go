// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/base64"
    "encoding/xml"
    "fmt"
    "net/url"
    "regexp"
    "sort"
    "strings"
    "time"
)

// It's impossible to have any kind of common method inherited into all the
// various serializable objects because the receiver of the method is the
// wrong class which confuses the xml marshaller.  Hence, this mess.
type AzureObject interface {
    Serialize() (string, error)
}

// marshalXML is a wrapper for serializing objects to XML in the visual layout
// that gwacl prefers.
func marshalXML(obj interface{}) ([]byte, error) {
    return xml.MarshalIndent(obj, "", "  ")
}

func toxml(obj AzureObject) (string, error) {
    out, err := marshalXML(obj)
    if err != nil {
        return "", err
    }
    return string(out), nil
}

//
// ConfigurationSet bits
//

const (
    CONFIG_SET_LINUX_PROVISIONING = "LinuxProvisioningConfiguration"
    CONFIG_SET_NETWORK            = "NetworkConfiguration"
)

// A ConfigurationSet object can be different things depending on its 'type'.
// The types we currently support are:
// - LinuxProvisioningConfigurationSet: configuration of a Linux VM
// - NetworkConfiguration: configuration of the network of a VM
type ConfigurationSet struct {
    ConfigurationSetType string `xml:"ConfigurationSetType"` // "ConfigurationSet"

    // LinuxProvisioningConfiguration fields.
    Hostname                         string `xml:"HostName,omitempty"`
    Username                         string `xml:"UserName,omitempty"`
    Password                         string `xml:"UserPassword,omitempty"`
    CustomData                       string `xml:"CustomData,omitempty"`
    DisableSSHPasswordAuthentication string `xml:"DisableSshPasswordAuthentication,omitempty"`

    // NetworkConfiguration fields.
    // We use slice pointers to work around a Go bug:
    // https://code.google.com/p/go/issues/detail?id=4168
    // We need the whole 'InputEndpoints' and 'SubnetNames' element to be omitted
    // when no InputEndpoint objects are present (this happens when the
    // ConfigurationSet object has a LinuxProvisioningConfiguration type for
    // instance).
    InputEndpoints *[]InputEndpoint `xml:"InputEndpoints>InputEndpoint,omitempty"`
    SubnetNames    *[]string        `xml:"SubnetNames>SubnetName,omitempty"`
}

func (c *ConfigurationSet) inputEndpoints() []InputEndpoint {
    return *c.InputEndpoints
}

func (c *ConfigurationSet) Serialize() (string, error) {
    return toxml(c)
}

// NewLinuxProvisioningConfiguration creates and returns a ConfigurationSet of
// type "LinuxProvisioningConfiguration" which is used when deploying a Linux
// VM instance.  Note that CustomData is passed to Azure *as-is* which also
// stores it as passed, so consider base64 encoding it.
func NewLinuxProvisioningConfigurationSet(
    Hostname, Username, Password, CustomData string,
    DisableSSHPasswordAuthentication string) *ConfigurationSet {
    return &ConfigurationSet{
        ConfigurationSetType:             CONFIG_SET_LINUX_PROVISIONING,
        Hostname:                         Hostname,
        Username:                         Username,
        Password:                         Password,
        CustomData:                       CustomData,
        DisableSSHPasswordAuthentication: DisableSSHPasswordAuthentication,
    }
}

// NewNetworkConfiguration creates a ConfigurationSet of type "NetworkConfiguration".
func NewNetworkConfigurationSet(
    inputEndpoints []InputEndpoint, subnetNames []string) *ConfigurationSet {
    return &ConfigurationSet{
        ConfigurationSetType: CONFIG_SET_NETWORK,
        InputEndpoints:       &inputEndpoints,
        SubnetNames:          &subnetNames,
    }
}

//
// InputEndpoint bits
//

type LoadBalancerProbe struct {
    Path     string `xml:"Path"`
    Port     int    `xml:"Port"` // Not uint16; see https://bugs.launchpad.net/juju-core/+bug/1201880
    Protocol string `xml:"Protocol"`
}

type InputEndpoint struct {
    LoadBalancedEndpointSetName string             `xml:"LoadBalancedEndpointSetName,omitempty"`
    LocalPort                   int                `xml:"LocalPort"` // Not uint16; see https://bugs.launchpad.net/juju-core/+bug/1201880
    Name                        string             `xml:"Name"`
    Port                        int                `xml:"Port"` // Not uint16; see https://bugs.launchpad.net/juju-core/+bug/1201880
    LoadBalancerProbe           *LoadBalancerProbe `xml:"LoadBalancerProbe,omitempty"`
    Protocol                    string             `xml:"Protocol"` // TCP or UDP
    VIP                         string             `xml:"Vip,omitempty"`
}

func (c *InputEndpoint) Serialize() (string, error) {
    return toxml(c)
}

//
// Images bits
//

// Images is a series of OSImages.
type Images struct {
    Images []OSImage `xml:"OSImage"`
}

func (i *Images) Deserialize(data []byte) error {
    return xml.Unmarshal(data, i)
}

var canonicalPublisherName = "Canonical"
var imageFamilyFormatRegexp = "^Ubuntu Server %s.*$"

func (images *Images) Len() int {
    return len(images.Images)
}

func (images *Images) Swap(i, j int) {
    images.Images[i], images.Images[j] = images.Images[j], images.Images[i]
}

// Less returns true if the image at index i is newer than the one at index j, comparing by
// PublishedDate.
// This function is used by sort.Sort().
func (images *Images) Less(i, j int) bool {
    // We need to implement the sort interface so Less cannot return an error.  We panic if
    // one of the dates cannot be parse and the calling method will recover this.
    dateStringI := images.Images[i].PublishedDate
    dateI, err := time.Parse(time.RFC3339, dateStringI)
    if err != nil {
        panic(fmt.Errorf("Failed to parse image's 'PublishedDate': %s", dateStringI))
    }
    dateStringJ := images.Images[j].PublishedDate
    dateJ, err := time.Parse(time.RFC3339, dateStringJ)
    if err != nil {
        panic(fmt.Errorf("Failed to parse image's 'PublishedDate': %s", dateStringJ))
    }
    return dateI.After(dateJ)
}

// GetLatestUbuntuImage returns the most recent released available OSImage,
// for the given release name and location.  The 'releaseName' parameter is
// the Ubuntu version number present in the 'ImageFamily' tag present in
// Azure's representation of an OS Image (e.g. '12.04', '12.10').
func (images *Images) GetLatestUbuntuImage(releaseName string, location string) (image *OSImage, err error) {
    // The Less method defined above can panic if one of the published dates cannot be parsed,
    // this code recovers from that and transforms that into an error.
    defer func() {
        if recoveredErr := recover(); recoveredErr != nil {
            image = nil
            err = recoveredErr.(error)
        }
    }()
    matcherRegexp := regexp.MustCompile(fmt.Sprintf(imageFamilyFormatRegexp, releaseName))
    matchingImages := Images{}
    for _, image := range images.Images {
        if image.PublisherName == canonicalPublisherName &&
            matcherRegexp.MatchString(image.ImageFamily) &&
            image.hasLocation(location) &&
            !image.isDailyBuild() {
            matchingImages.Images = append(matchingImages.Images, image)
        }
    }
    if matchingImages.Len() == 0 {
        return nil, fmt.Errorf("No matching images found")
    }
    sort.Sort(&matchingImages)
    return &matchingImages.Images[0], nil
}

//
// OSImage bits
//

// OSImage represents a disk image containing an operating system.
// Confusingly, the Azure API documentation also calls it a VM image.
type OSImage struct {
    AffinityGroup     string  `xml:"AffinityGroup,omitempty"`
    Category          string  `xml:"Category"`
    Label             string  `xml:"Label"`
    Location          string  `xml:"Location"`
    LogicalSizeInGB   float32 `xml:"LogicalSizeInGB"`
    MediaLink         string  `xml:"MediaLink"`
    Name              string  `xml:"Name"`
    OS                string  `xml:"OS"`
    EULA              string  `xml:"Eula,omitempty"`
    Description       string  `xml:"Description,omitempty"`
    ImageFamily       string  `xml:"ImageFamily,omitempty"`
    PublishedDate     string  `xml:"PublishedDate,omitempty"`
    IsPremium         string  `xml:"IsPremium,omitempty"`
    PrivacyURI        string  `xml:"PrivacyUri,omitempty"`
    PricingDetailLink string  `xml:"PricingDetailLink,omitempty"`
    IconURI           string  `xml:"IconUri,omitempty"`
    RecommendedVMSize string  `xml:"RecommendedVMSize,omitempty"`
    PublisherName     string  `xml:"PublisherName"`
    ShowInGUI         string  `xml:"ShowInGui"`
    SmallIconURI      string  `xml:"SmallIconUri,omitempty"`
    Language          string  `xml:"Language"`
}

func (image *OSImage) hasLocation(location string) bool {
    locations := strings.Split(image.Location, ";")
    for _, loc := range locations {
        if loc == location {
            return true
        }
    }
    return false
}

// isDailyBuild returns whether this image is a daily build.
func (image *OSImage) isDailyBuild() bool {
    return strings.Contains(image.Label, "DAILY")
}

func (i *OSImage) Deserialize(data []byte) error {
    return xml.Unmarshal(data, i)
}

//
// DataVirtualHardDisk
//

type DataVirtualHardDisk struct {
    HostCaching         string `xml:"HostCaching"`
    DiskName            string `xml:"DiskName"`
    LUN                 string `xml:"Lun"`
    LogicalDiskSizeInGB string `xml:"LogicalDiskSizeInGB"`
    MediaLink           string `xml:"MediaLink"`
}

//
// OSVirtualHardDisk bits
//

type HostCachingType string

const (
    HostCachingRO HostCachingType = "ReadOnly"
    HostCachingRW HostCachingType = "ReadWrite"
)

type OSVirtualHardDisk struct {
    HostCaching     string `xml:"HostCaching,omitempty"`
    DiskLabel       string `xml:"DiskLabel,omitempty"`
    DiskName        string `xml:"DiskName,omitempty"`
    MediaLink       string `xml:"MediaLink,omitempty"`
    SourceImageName string `xml:"SourceImageName,omitempty"`
    OS              string `xml:"OS,omitempty"`
}

func (c *OSVirtualHardDisk) Serialize() (string, error) {
    return toxml(c)
}

func NewOSVirtualHardDisk(
    HostCaching HostCachingType, DiskLabel, DiskName, MediaLink,
    SourceImageName, OS string) *OSVirtualHardDisk {
    return &OSVirtualHardDisk{
        HostCaching:     string(HostCaching),
        DiskLabel:       DiskLabel,
        DiskName:        DiskName,
        SourceImageName: SourceImageName,
        MediaLink:       MediaLink,
        OS:              OS,
    }
}

// CreateVirtualHardDiskMediaLink creates a media link string used to specify
// the location of a physical blob in the given Windows Azure storage account.
// Example: http://example.blob.core.windows.net/disks/mydatadisk.vhd
func CreateVirtualHardDiskMediaLink(StorageName, StoragePath string) string {
    pathComponents := strings.Split(StoragePath, "/")
    components := append(pathComponents, StorageName)
    checkPathComponents(components...)
    return fmt.Sprintf("http://%s.blob.core.windows.net/%s", StorageName, StoragePath)
}

type Role struct {
    RoleName          string              `xml:"RoleName"`
    RoleType          string              `xml:"RoleType"` // Always "PersistentVMRole"
    ConfigurationSets []ConfigurationSet  `xml:"ConfigurationSets>ConfigurationSet"`
    OSVirtualHardDisk []OSVirtualHardDisk `xml:"OSVirtualHardDisk"`
    RoleSize          string              `xml:"RoleSize"`
}

//
// Role bits
//

func (c *Role) Serialize() (string, error) {
    return toxml(c)
}

func NewRole(RoleSize string, RoleName string,
    ConfigurationSets []ConfigurationSet, vhds []OSVirtualHardDisk) *Role {
    return &Role{
        RoleSize:          RoleSize,
        RoleName:          RoleName,
        RoleType:          "PersistentVMRole",
        ConfigurationSets: ConfigurationSets,
        OSVirtualHardDisk: vhds,
    }
}

//
// DnsServer bits
//

type DnsServer struct {
    Name    string `xml:"Name"`
    Address string `xml:"Address"`
}

func (c *DnsServer) Serialize() (string, error) {
    return toxml(c)
}

//
// Hosted service bits
//

// HostedService represents a cloud service in Azure.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460806.aspx
type HostedService struct {
    HostedServiceDescriptor
    XMLNS       string       `xml:"xmlns,attr"`
    Deployments []Deployment `xml:"Deployments>Deployment"`
}

func (c HostedService) Serialize() (string, error) {
    return toxml(c)
}

func (c *HostedService) Deserialize(data []byte) error {
    return xml.Unmarshal(data, c)
}

type HostedServiceDescriptorList struct {
    XMLName        xml.Name                  `xml:"HostedServices"`
    XMLNS          string                    `xml:"xmlns,attr"`
    HostedServices []HostedServiceDescriptor `xml:"HostedService"`
}

func (c *HostedServiceDescriptorList) Serialize() (string, error) {
    return toxml(c)
}

func (c *HostedServiceDescriptorList) Deserialize(data []byte) error {
    return xml.Unmarshal(data, c)
}

// HostedServiceDescriptor contains a subset of the details in HostedService,
// and is used when describing a list of HostedServices.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460781.aspx
type HostedServiceDescriptor struct {
    URL                string             `xml:"Url"`
    ServiceName        string             `xml:"ServiceName"`
    Description        string             `xml:"HostedServiceProperties>Description"`
    AffinityGroup      string             `xml:"HostedServiceProperties>AffinityGroup"`
    Location           string             `xml:"HostedServiceProperties>Location"`
    Label              string             `xml:"HostedServiceProperties>Label"`
    Status             string             `xml:"HostedServiceProperties>Status"`
    DateCreated        string             `xml:"HostedServiceProperties>DateCreated"`
    DateLastModified   string             `xml:"HostedServiceProperties>DateLastModified"`
    ExtendedProperties []ExtendedProperty `xml:"HostedServiceProperties>ExtendedProperties>ExtendedProperty"`
}

func (c HostedServiceDescriptor) Serialize() (string, error) {
    return toxml(c)
}

func (service *HostedServiceDescriptor) GetLabel() (string, error) {
    label, err := base64.StdEncoding.DecodeString(service.Label)
    if err != nil {
        return "", err
    }
    return string(label), nil
}

type CreateHostedService struct {
    XMLNS              string             `xml:"xmlns,attr"`
    ServiceName        string             `xml:"ServiceName"`
    Label              string             `xml:"Label"` // base64-encoded
    Description        string             `xml:"Description"`
    Location           string             `xml:"Location,omitempty"`
    AffinityGroup      string             `xml:"AffinityGroup,omitempty"`
    ExtendedProperties []ExtendedProperty `xml:"ExtendedProperties>ExtendedProperty"`
}

func NewCreateHostedServiceWithLocation(serviceName, label, location string) *CreateHostedService {
    base64label := base64.StdEncoding.EncodeToString([]byte(label))
    return &CreateHostedService{
        XMLNS:       XMLNS,
        ServiceName: serviceName,
        Label:       base64label,
        Location:    location,
    }
}

func (s *CreateHostedService) Deserialize(data []byte) error {
    return xml.Unmarshal(data, s)
}

// AvailabilityResponse is the reply from a Check Hosted Service Name
// Availability operation.
type AvailabilityResponse struct {
    XMLNS  string `xml:"xmlns,attr"`
    Result string `xml:"Result"`
    Reason string `xml:"Reason"`
}

func (a *AvailabilityResponse) Deserialize(data []byte) error {
    return xml.Unmarshal(data, a)
}

// UpdateHostedService contains the details necessary to call the
// UpdateHostedService management API call.
// See http://msdn.microsoft.com/en-us/library/windowsazure/gg441303.aspx
type UpdateHostedService struct {
    XMLNS              string             `xml:"xmlns,attr"`
    Label              string             `xml:"Label,omitempty"` // base64-encoded
    Description        string             `xml:"Description,omitempty"`
    ExtendedProperties []ExtendedProperty `xml:"ExtendedProperties>ExtendedProperty,omitempty"`
}

func (u *UpdateHostedService) Serialize() (string, error) {
    return toxml(u)
}

func NewUpdateHostedService(label, description string, properties []ExtendedProperty) *UpdateHostedService {
    base64label := base64.StdEncoding.EncodeToString([]byte(label))
    return &UpdateHostedService{
        XMLNS:              XMLNS,
        Label:              base64label,
        Description:        description,
        ExtendedProperties: properties,
    }
}

//
// Deployment bits
//

// Deployment is used both as input for the "Create Virtual Machine Deployment"
// call, and as a return value for "Get Deployment."
type Deployment struct {
    XMLNS   string `xml:"xmlns,attr"`
    XMLNS_I string `xml:"xmlns:i,attr"`
    Name    string `xml:"Name"`
    // DeploymentSlot is either "Production" or "Staging".
    DeploymentSlot     string             `xml:"DeploymentSlot"`
    PrivateID          string             `xml:"PrivateID,omitempty"` // Only used for "Get Deployment."
    Status             string             `xml:"Status,omitempty"`    // Only used for "Get Deployment."
    Label              string             `xml:"Label"`
    URL                string             `xml:"Url,omitempty"`           // Only used for "Get Deployment."
    Configuration      string             `xml:"Configuration,omitempty"` // Only used for "Get Deployment."
    RoleInstanceList   []RoleInstance     `xml:"RoleInstanceList>RoleInstance"`
    UpgradeDomainCount string             `xml:"UpgradeDomainCount,omitempty"` // Only used for "Get Deployment."
    RoleList           []Role             `xml:"RoleList>Role"`
    SDKVersion         string             `xml:"SdkVersion,omitempty"`      // Only used for "Get Deployment."
    Locked             string             `xml:"Locked,omitempty"`          // Only used for "Get Deployment."
    RollbackAllowed    string             `xml:"RollbackAllowed,omitempty"` // Only used for "Get Deployment."
    VirtualNetworkName string             `xml:VirtualNetworkName,omitempty"`
    DNS                []DnsServer        `xml:"Dns>DnsServers>DnsServer",omitempty`
    ExtendedProperties []ExtendedProperty `xml:"ExtendedProperties>ExtendedProperty,omitempty"` // Only used for "Get Deployment."
}

func (deployment *Deployment) GetFQDN() (string, error) {
    if deployment.URL == "" {
        return "", fmt.Errorf("Deployment's URL field is empty")
    }
    parsedURL, err := url.Parse(deployment.URL)
    if err != nil {
        return "", err
    }
    return parsedURL.Host, nil
}

func (s *Deployment) Deserialize(data []byte) error {
    return xml.Unmarshal(data, s)
}

func (c *Deployment) Serialize() (string, error) {
    return toxml(c)
}

// RoleInstance is a component of a Deployment.
type RoleInstance struct {
    RoleName                          string             `xml:"RoleName"`
    InstanceName                      string             `xml:"InstanceName"`
    InstanceStatus                    string             `xml:"InstanceStatus"`
    InstanceUpgradeDomain             string             `xml:"InstanceUpgradeDomain"`
    InstanceFaultDomain               string             `xml:"InstanceFaultDomain"`
    InstanceSize                      string             `xml:"InstanceSize"`
    InstanceStateDetails              string             `xml:"InstanceStateDetails"`
    InstanceErrorCode                 string             `xml:"InstanceErrorCode"`
    IPAddress                         string             `xml:"IpAddress"`
    InstanceEndpoints                 []InstanceEndpoint `xml:"InstanceEndpoints>InstanceEndpoint"`
    PowerState                        string             `xml:"PowerState"`
    HostName                          string             `xml:"HostName"`
    RemoteAccessCertificateThumbprint string             `xml:"RemoteAccessCertificateThumbprint"`
}

// InstanceEndpoint is a component of a RoleInstance.
type InstanceEndpoint struct {
    Name       string `xml:"Name"`
    VIP        string `xml:"Vip"`
    PublicPort int    `xml:"PublicPort"` // Not uint16; see https://bugs.launchpad.net/juju-core/+bug/1201880
    LocalPort  int    `xml:"LocalPort"`  // Not uint16; see https://bugs.launchpad.net/juju-core/+bug/1201880
    Protocol   string `xml:"Protocol"`
}

// newDeploymentForCreateVMDeployment creates a Deployment object for the
// purpose of passing it to "Create Virtual Machine Deployment."
// You may still want to set the optional DNS attribute.
func NewDeploymentForCreateVMDeployment(name, deploymentSlot, label string, roles []Role, virtualNetworkName string) *Deployment {
    deployment := Deployment{
        XMLNS:              XMLNS,
        XMLNS_I:            XMLNS_I,
        Name:               name,
        DeploymentSlot:     deploymentSlot,
        Label:              base64.StdEncoding.EncodeToString([]byte(label)),
        RoleList:           roles,
        VirtualNetworkName: virtualNetworkName,
    }
    return &deployment
}

const XMLNS = "http://schemas.microsoft.com/windowsazure"
const XMLNS_I = "http://www.w3.org/2001/XMLSchema-instance"
const XMLNS_NC = "http://schemas.microsoft.com/ServiceHosting/2011/07/NetworkConfiguration"

//
// Role Operations bits
//

type RoleOperation struct {
    XMLName       xml.Name
    XMLNS         string `xml:"xmlns,attr"`
    XMLNS_I       string `xml:"xmlns:i,attr"`
    OperationType string `xml:"OperationType"`
}

func newRoleOperation(operationType string) *RoleOperation {
    operation := RoleOperation{
        XMLNS:         XMLNS,
        XMLNS_I:       XMLNS_I,
        OperationType: operationType,
    }
    operation.XMLName.Local = operationType
    return &operation
}

// The Start Role operation starts a virtual machine.
// http://msdn.microsoft.com/en-us/library/windowsazure/jj157189.aspx
var startRoleOperation = newRoleOperation("StartRoleOperation")

// The Shutdown Role operation shuts down a virtual machine.
// http://msdn.microsoft.com/en-us/library/windowsazure/jj157195.aspx
var shutdownRoleOperation = newRoleOperation("ShutdownRoleOperation")

// The Restart role operation restarts a virtual machine.
// http://msdn.microsoft.com/en-us/library/windowsazure/jj157197.aspx
var restartRoleOperation = newRoleOperation("RestartRoleOperation")

//
// PersistentVMRole, as used by GetRole, UpdateRole, etc.
//
type PersistentVMRole struct {
    XMLNS                             string                 `xml:"xmlns,attr"`
    RoleName                          string                 `xml:"RoleName"`
    OsVersion                         string                 `xml:"OsVersion"`
    RoleType                          string                 `xml:"RoleType"` // Always PersistentVMRole
    ConfigurationSets                 []ConfigurationSet     `xml:"ConfigurationSets>ConfigurationSet"`
    AvailabilitySetName               string                 `xml:"AvailabilitySetName"`
    DataVirtualHardDisks              *[]DataVirtualHardDisk `xml:"DataVirtualHardDisks>DataVirtualHardDisk,omitempty"`
    OSVirtualHardDisk                 OSVirtualHardDisk      `xml:"OSVirtualHardDisk"`
    RoleSize                          string                 `xml:"RoleSize"`
    DefaultWinRmCertificateThumbprint string                 `xml:"DefaultWinRmCertificateThumbprint"`
}

func (role *PersistentVMRole) Deserialize(data []byte) error {
    return xml.Unmarshal(data, role)
}

func (role *PersistentVMRole) Serialize() (string, error) {
    return toxml(role)
}

//
// Virtual Networks
//

type VirtualNetDnsServer struct {
    XMLName   string `xml:"DnsServer"`
    Name      string `xml:"name,attr"`
    IPAddress string `xml:"IPAddress,attr"`
}

type LocalNetworkSite struct {
    XMLName              string   `xml:"LocalNetworkSite"`
    Name                 string   `xml:"name,attr"`
    AddressSpacePrefixes []string `xml:"AddressSpace>AddressPrefix"`
    VPNGatewayAddress    string   `xml:"VPNGatewayAddress"`
}

type Subnet struct {
    XMLName       string `xml:"Subnet"`
    Name          string `xml:"name,attr"`
    AddressPrefix string `xml:"AddressPrefix"`
}

type LocalNetworkSiteRefConnection struct {
    XMLName string `xml:"Connection"`
    Type    string `xml:"type,attr"`
}

type LocalNetworkSiteRef struct {
    XMLName    string                        `xml:"LocalNetworkSiteRef"`
    Name       string                        `xml:"name,attr"`
    Connection LocalNetworkSiteRefConnection `xml:"Connection"`
}

type Gateway struct {
    XMLName                      string              `xml:"Gateway"`
    Profile                      string              `xml:"profile,attr"`
    VPNClientAddressPoolPrefixes []string            `xml:"VPNClientAddressPool>AddressPrefix"`
    LocalNetworkSiteRef          LocalNetworkSiteRef `xml:"ConnectionsToLocalNetwork>LocalNetworkSiteRef"`
}

type DnsServerRef struct {
    XMLName string `xml:"DnsServerRef"`
    Name    string `xml:"name,attr"`
}

type VirtualNetworkSite struct {
    Name                 string          `xml:"name,attr"`
    AffinityGroup        string          `xml:"AffinityGroup,attr"`
    AddressSpacePrefixes []string        `xml:"AddressSpace>AddressPrefix"`
    Subnets              *[]Subnet       `xml:"Subnets>Subnet",omitempty`
    DnsServersRef        *[]DnsServerRef `xml:"DnsServersRef>DnsServerRef",omitempty`
    Gateway              *Gateway        `xml:"Gateway",omitempty`
}

type NetworkConfiguration struct {
    XMLNS               string                 `xml:"xmlns,attr"`
    DNS                 *[]VirtualNetDnsServer `xml:"VirtualNetworkConfiguration>Dns>DnsServers>DnsServer",omitempty`
    LocalNetworkSites   *[]LocalNetworkSite    `xml:"VirtualNetworkConfiguration>LocalNetworkSites>LocalNetworkSite",omitempty`
    VirtualNetworkSites *[]VirtualNetworkSite  `xml:"VirtualNetworkConfiguration>VirtualNetworkSites>VirtualNetworkSite",omitempty`
}

func (nc *NetworkConfiguration) Serialize() (string, error) {
    return toxml(nc)
}

func (nc *NetworkConfiguration) Deserialize(data []byte) error {
    return xml.Unmarshal(data, nc)
}

//
// Affinity Group
//

// See http://msdn.microsoft.com/en-us/library/windowsazure/gg715317.aspx
type CreateAffinityGroup struct {
    XMLNS       string `xml:"xmlns,attr"`
    Name        string `xml:"Name"`
    Label       string `xml:"Label"` // Must be base64 encoded.
    Description string `xml:"Description",omitempty`
    Location    string `xml:"Location"` // Value comes from ListLocations.
}

func (c *CreateAffinityGroup) Serialize() (string, error) {
    return toxml(c)
}

func NewCreateAffinityGroup(name, label, description, location string) *CreateAffinityGroup {
    base64label := base64.StdEncoding.EncodeToString([]byte(label))
    return &CreateAffinityGroup{
        XMLNS:       XMLNS,
        Name:        name,
        Label:       base64label,
        Description: description,
        Location:    location,
    }
}

// See http://msdn.microsoft.com/en-us/library/windowsazure/gg715316.aspx
type UpdateAffinityGroup struct {
    XMLNS       string `xml:"xmlns,attr"`
    Label       string `xml:"Label"` // Must be base64 encoded.
    Description string `xml:"Description",omitempty`
}

func (u *UpdateAffinityGroup) Serialize() (string, error) {
    return toxml(u)
}

func NewUpdateAffinityGroup(label, description string) *UpdateAffinityGroup {
    base64label := base64.StdEncoding.EncodeToString([]byte(label))
    return &UpdateAffinityGroup{
        XMLNS:       XMLNS,
        Label:       base64label,
        Description: description,
    }
}

//
// Storage Services bits
//

type ExtendedProperty struct {
    Name  string `xml:"Name"`
    Value string `xml:"Value"`
}

type StorageService struct {
    // List Storage Accounts.
    // See http://msdn.microsoft.com/en-us/library/windowsazure/ee460787.aspx
    URL                   string             `xml:"Url"`
    ServiceName           string             `xml:"ServiceName"`
    Description           string             `xml:"StorageServiceProperties>Description"`
    AffinityGroup         string             `xml:"StorageServiceProperties>AffinityGroup"`
    Label                 string             `xml:"StorageServiceProperties>Label"` // base64
    Status                string             `xml:"StorageServiceProperties>Status"`
    Endpoints             []string           `xml:"StorageServiceProperties>Endpoints>Endpoint"`
    GeoReplicationEnabled string             `xml:"StorageServiceProperties>GeoReplicationEnabled"`
    GeoPrimaryRegion      string             `xml:"StorageServiceProperties>GeoPrimaryRegion"`
    StatusOfPrimary       string             `xml:"StorageServiceProperties>StatusOfPrimary"`
    LastGeoFailoverTime   string             `xml:"StorageServiceProperties>LastGeoFailoverTime"`
    GeoSecondaryRegion    string             `xml:"StorageServiceProperties>GeoSecondaryRegion"`
    StatusOfSecondary     string             `xml:"StorageServiceProperties>StatusOfSecondary"`
    ExtendedProperties    []ExtendedProperty `xml:"StorageServiceProperties>ExtendedProperties>ExtendedProperty"`

    // TODO: Add accessors for non-string data encoded as strings.
}

type StorageServices struct {
    XMLNS           string           `xml:"xmlns,attr"`
    StorageServices []StorageService `xml:"StorageService"`
}

func (s *StorageServices) Deserialize(data []byte) error {
    return xml.Unmarshal(data, s)
}

// CreateStorageServiceInput is a request to create a storage account.
// (Azure's "storage services" seem to have been renamed to "storage accounts"
// but the old terminology is still evident in the API).
type CreateStorageServiceInput struct {
    // See http://msdn.microsoft.com/en-us/library/windowsazure/hh264518.aspx
    XMLNS                 string             `xml:"xmlns,attr"`
    ServiceName           string             `xml:"ServiceName"`
    Label                 string             `xml:"Label"`
    Description           string             `xml:"Description,omitempty"`
    Location              string             `xml:"Location"`
    AffinityGroup         string             `xml:"AffinityGroup,omitempty"`
    GeoReplicationEnabled string             `xml:"GeoReplicationEnabled,omitempty"`
    ExtendedProperties    []ExtendedProperty `xml:"ExtendedProperties>ExtendedProperty"`
}

func (c *CreateStorageServiceInput) Serialize() (string, error) {
    return toxml(c)
}

// NewCreateStorageServiceInputWithLocation creates a location-based
// CreateStorageServiceInput, with all required fields filled out.
func NewCreateStorageServiceInputWithLocation(name, label, location string, geoReplicationEnabled string) *CreateStorageServiceInput {
    return &CreateStorageServiceInput{
        XMLNS:                 XMLNS,
        ServiceName:           name,
        Label:                 base64.StdEncoding.EncodeToString([]byte(label)),
        Location:              location,
        GeoReplicationEnabled: geoReplicationEnabled,
    }
}

type MetadataItem struct {
    XMLName xml.Name
    Value   string `xml:",chardata"`
}

func (item *MetadataItem) Name() string {
    return item.XMLName.Local
}

type Metadata struct {
    Items []MetadataItem `xml:",any"`
}

type Blob struct {
    Name                  string   `xml:"Name"`
    Snapshot              string   `xml:"Snapshot"`
    URL                   string   `xml:"Url"`
    LastModified          string   `xml:"Properties>Last-Modified"`
    ETag                  string   `xml:"Properties>Etag"`
    ContentLength         string   `xml:"Properties>Content-Length"`
    ContentType           string   `xml:"Properties>Content-Type"`
    BlobSequenceNumber    string   `xml:"Properties>x-ms-blob-sequence-number"`
    BlobType              string   `xml:"Properties>BlobType"`
    LeaseStatus           string   `xml:"Properties>LeaseStatus"`
    LeaseState            string   `xml:"Properties>LeaseState"`
    LeaseDuration         string   `xml:"Properties>LeaseDuration"`
    CopyID                string   `xml:"Properties>CopyId"`
    CopyStatus            string   `xml:"Properties>CopyStatus"`
    CopySource            string   `xml:"Properties>CopySource"`
    CopyProgress          string   `xml:"Properties>CopyProgress"`
    CopyCompletionTime    string   `xml:"Properties>CopyCompletionTime"`
    CopyStatusDescription string   `xml:"Properties>CopyStatusDescription"`
    Metadata              Metadata `xml:"Metadata"`
}

type BlobEnumerationResults struct {
    // http://msdn.microsoft.com/en-us/library/windowsazure/dd135734.aspx
    XMLName       xml.Name `xml:"EnumerationResults"`
    ContainerName string   `xml:"ContainerName,attr"`
    Prefix        string   `xml:"Prefix"`
    Marker        string   `xml:"Marker"`
    MaxResults    string   `xml:"MaxResults"`
    Delimiter     string   `xml:"Delimiter"`
    Blobs         []Blob   `xml:"Blobs>Blob"`
    BlobPrefixes  []string `xml:"Blobs>BlobPrefix>Name"`
    NextMarker    string   `xml:"NextMarker"`
}

func (b *BlobEnumerationResults) Deserialize(data []byte) error {
    return xml.Unmarshal(data, b)
}

type StorageAccountKeys struct {
    // See http://msdn.microsoft.com/en-us/library/windowsazure/ee460785.aspx
    XMLName   xml.Name `xml:"StorageService"`
    URL       string   `xml:"Url"`
    Primary   string   `xml:"StorageServiceKeys>Primary"`
    Secondary string   `xml:"StorageServiceKeys>Secondary"`
}

func (s *StorageAccountKeys) Deserialize(data []byte) error {
    return xml.Unmarshal(data, s)
}

type ContainerEnumerationResults struct {
    // See http://msdn.microsoft.com/en-us/library/windowsazure/dd179352.aspx
    XMLName    xml.Name    `xml:"EnumerationResults"`
    Prefix     string      `xml:"Prefix"`
    Marker     string      `xml:"Marker"`
    MaxResults string      `xml:"MaxResults"`
    Containers []Container `xml:"Containers>Container"`
    NextMarker string      `xml:"NextMarker"`
}

func (s *ContainerEnumerationResults) Deserialize(data []byte) error {
    return xml.Unmarshal(data, s)
}

type Container struct {
    XMLName    xml.Name   `xml:"Container"`
    Name       string     `xml:"Name"`
    URL        string     `xml:"URL"`
    Properties Properties `xml:"Properties"`
    Metadata   Metadata   `xml:"Metadata"`
}

type Properties struct {
    LastModified  string `xml:"Last-Modified"`
    ETag          string `xml:"Etag"`
    LeaseStatus   string `xml:"LeaseStatus"`
    LeaseState    string `xml:"LeaseState"`
    LeaseDuration string `xml:"LeaseDuration"`
}

// An enumeration-lite type to define from which list (committed, uncommitted,
// latest) to get a block during a PutBlockList Storage API operation.
type BlockListType string

const (
    BlockListUncommitted BlockListType = "Uncommitted"
    BlockListCommitted   BlockListType = "Committed"
    BlockListLatest      BlockListType = "Latest"
)

// Payload for the PutBlockList operation.
type BlockList struct {
    XMLName xml.Name `xml:"BlockList"`
    Items   []*BlockListItem
}

func (s *BlockList) Serialize() ([]byte, error) {
    return marshalXML(s)
}

// Add a BlockListItem to a BlockList.
func (s *BlockList) Add(blockType BlockListType, blockID string) {
    base64ID := base64.StdEncoding.EncodeToString([]byte(blockID))
    item := NewBlockListItem(blockType, base64ID)
    s.Items = append(s.Items, item)
}

type BlockListItem struct {
    XMLName xml.Name
    BlockID string `xml:",chardata"`
}

// Create a new BlockListItem.
func NewBlockListItem(blockType BlockListType, blockID string) *BlockListItem {
    return &BlockListItem{
        XMLName: xml.Name{Local: string(blockType)},
        BlockID: blockID,
    }
}

func (item *BlockListItem) Type() BlockListType {
    switch BlockListType(item.XMLName.Local) {
    case BlockListUncommitted:
        return BlockListUncommitted
    case BlockListCommitted:
        return BlockListCommitted
    case BlockListLatest:
        return BlockListLatest
    }
    panic(fmt.Errorf("type not recognized: %s", item.XMLName.Local))
}

// GetBlockList result struct.
type Block struct {
    Name string `xml:"Name"`
    Size string `xml:"Size"`
}

type GetBlockList struct {
    XMLName           xml.Name `xml:"BlockList"`
    CommittedBlocks   []Block  `xml:"CommittedBlocks>Block"`
    UncommittedBlocks []Block  `xml:"UncommittedBlocks>Block"`
}

func (g *GetBlockList) Deserialize(data []byte) error {
    return xml.Unmarshal(data, g)
}

//
// Operation Services bits
//

const (
    InProgressOperationStatus = "InProgress"
    SucceededOperationStatus  = "Succeeded"
    FailedOperationStatus     = "Failed"
)

type Operation struct {
    ID             string `xml:"ID"`
    Status         string `xml:"Status"`
    HTTPStatusCode int    `xml:"HttpStatusCode"`
    ErrorCode      string `xml:"Error>Code"`
    ErrorMessage   string `xml:"Error>Message"`
}

func (o *Operation) Deserialize(data []byte) error {
    return xml.Unmarshal(data, o)
}
