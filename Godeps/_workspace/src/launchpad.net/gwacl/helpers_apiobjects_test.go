// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).
//
// Test helpers to fake objects that go into, or come out of, the Azure API.

package gwacl

func populateEndpoint(endpoint *InputEndpoint) *InputEndpoint {
    if endpoint.LoadBalancedEndpointSetName == "" {
        endpoint.LoadBalancedEndpointSetName = MakeRandomString(10)
    }
    if endpoint.LocalPort == 0 {
        endpoint.LocalPort = int(MakeRandomPort())
    }
    if endpoint.Name == "" {
        endpoint.Name = MakeRandomString(10)
    }
    if endpoint.Port == 0 {
        endpoint.Port = int(MakeRandomPort())
    }
    if endpoint.LoadBalancerProbe == nil {
        endpoint.LoadBalancerProbe = &LoadBalancerProbe{}
    }
    if endpoint.LoadBalancerProbe.Path == "" {
        endpoint.LoadBalancerProbe.Path = MakeRandomString(10)
    }
    if endpoint.LoadBalancerProbe.Port == 0 {
        endpoint.LoadBalancerProbe.Port = int(MakeRandomPort())
    }
    if endpoint.LoadBalancerProbe.Protocol == "" {
        endpoint.LoadBalancerProbe.Protocol = MakeRandomString(10)
    }
    if endpoint.Protocol == "" {
        endpoint.Protocol = MakeRandomString(10)
    }
    if endpoint.VIP == "" {
        endpoint.VIP = MakeRandomString(10)
    }
    return endpoint
}

func makeLinuxProvisioningConfiguration() *ConfigurationSet {
    hostname := MakeRandomString(10)
    username := MakeRandomString(10)
    password := MakeRandomString(10)
    customdata := MakeRandomString(10)
    disableSSH := BoolToString(MakeRandomBool())
    return NewLinuxProvisioningConfigurationSet(hostname, username, password, customdata, disableSSH)
}

func makeOSVirtualHardDisk() *OSVirtualHardDisk {
    HostCaching := BoolToString(MakeRandomBool())
    DiskLabel := MakeRandomString(10)
    DiskName := MakeRandomString(10)
    MediaLink := MakeRandomString(10)
    SourceImageName := MakeRandomString(10)

    return &OSVirtualHardDisk{
        HostCaching:     HostCaching,
        DiskLabel:       DiskLabel,
        DiskName:        DiskName,
        MediaLink:       MediaLink,
        SourceImageName: SourceImageName}
}

func makeRole() *Role {
    RoleSize := "ExtraSmall"
    RoleName := MakeRandomString(10)
    RoleType := "PersistentVMRole"
    config := makeLinuxProvisioningConfiguration()
    configset := []ConfigurationSet{*config}

    return &Role{
        RoleSize:          RoleSize,
        RoleName:          RoleName,
        RoleType:          RoleType,
        ConfigurationSets: configset}
}

func makeDnsServer() *DnsServer {
    name := MakeRandomString(10)
    address := MakeRandomString(10)

    return &DnsServer{
        Name:    name,
        Address: address}
}

func makeDeployment() *Deployment {
    Name := MakeRandomString(10)
    DeploymentSlot := "Staging"
    Label := MakeRandomString(10)
    VirtualNetworkName := MakeRandomString(10)
    role := makeRole()
    RoleList := []Role{*role}
    Dns := []DnsServer{*makeDnsServer()}

    return &Deployment{
        XMLNS:              XMLNS,
        XMLNS_I:            XMLNS_I,
        Name:               Name,
        DeploymentSlot:     DeploymentSlot,
        Label:              Label,
        RoleList:           RoleList,
        VirtualNetworkName: VirtualNetworkName,
        DNS:                Dns,
    }
}

func makeCreateStorageServiceInput() *CreateStorageServiceInput {
    ServiceName := MakeRandomString(10)
    Description := MakeRandomString(10)
    Label := MakeRandomString(10)
    AffinityGroup := MakeRandomString(10)
    Location := MakeRandomString(10)
    GeoReplicationEnabled := BoolToString(MakeRandomBool())
    ExtendedProperties := []ExtendedProperty{{
        Name:  MakeRandomString(10),
        Value: MakeRandomString(10)}}

    return &CreateStorageServiceInput{
        XMLNS:                 XMLNS,
        ServiceName:           ServiceName,
        Description:           Description,
        Label:                 Label,
        AffinityGroup:         AffinityGroup,
        Location:              Location,
        GeoReplicationEnabled: GeoReplicationEnabled,
        ExtendedProperties:    ExtendedProperties}
}
