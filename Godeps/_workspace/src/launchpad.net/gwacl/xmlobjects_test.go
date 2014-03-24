// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/base64"
    "encoding/xml"
    "fmt"
    . "launchpad.net/gocheck"
    "launchpad.net/gwacl/dedent"
    "sort"
    "strings"
)

type xmlSuite struct{}

var _ = Suite(&xmlSuite{})

//
// Tests for Marshallers
//

func (suite *xmlSuite) TestConfigurationSet(c *C) {
    config := makeLinuxProvisioningConfiguration()

    xml, err := config.Serialize()
    c.Assert(err, IsNil)
    template := dedent.Dedent(`
        <ConfigurationSet>
          <ConfigurationSetType>LinuxProvisioningConfiguration</ConfigurationSetType>
          <HostName>%s</HostName>
          <UserName>%s</UserName>
          <UserPassword>%s</UserPassword>
          <CustomData>%s</CustomData>
          <DisableSshPasswordAuthentication>%v</DisableSshPasswordAuthentication>
        </ConfigurationSet>`)
    expected := fmt.Sprintf(template, config.Hostname, config.Username,
        config.Password, config.CustomData,
        config.DisableSSHPasswordAuthentication)
    c.Check(strings.TrimSpace(xml), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestInputEndpoint(c *C) {
    endpoint := populateEndpoint(&InputEndpoint{})

    xml, err := endpoint.Serialize()
    c.Assert(err, IsNil)
    template := dedent.Dedent(`
        <InputEndpoint>
          <LoadBalancedEndpointSetName>%s</LoadBalancedEndpointSetName>
          <LocalPort>%v</LocalPort>
          <Name>%s</Name>
          <Port>%v</Port>
          <LoadBalancerProbe>
            <Path>%s</Path>
            <Port>%d</Port>
            <Protocol>%s</Protocol>
          </LoadBalancerProbe>
          <Protocol>%s</Protocol>
          <Vip>%s</Vip>
        </InputEndpoint>`)
    expected := fmt.Sprintf(template, endpoint.LoadBalancedEndpointSetName,
        endpoint.LocalPort, endpoint.Name, endpoint.Port,
        endpoint.LoadBalancerProbe.Path, endpoint.LoadBalancerProbe.Port,
        endpoint.LoadBalancerProbe.Protocol, endpoint.Protocol, endpoint.VIP)
    c.Check(strings.TrimSpace(xml), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestOSVirtualHardDisk(c *C) {
    disk := makeOSVirtualHardDisk()

    xml, err := disk.Serialize()
    c.Assert(err, IsNil)
    template := dedent.Dedent(`
        <OSVirtualHardDisk>
          <HostCaching>%s</HostCaching>
          <DiskLabel>%s</DiskLabel>
          <DiskName>%s</DiskName>
          <MediaLink>%s</MediaLink>
          <SourceImageName>%s</SourceImageName>
        </OSVirtualHardDisk>`)
    expected := fmt.Sprintf(template, disk.HostCaching, disk.DiskLabel,
        disk.DiskName, disk.MediaLink, disk.SourceImageName)
    c.Check(strings.TrimSpace(xml), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestConfigurationSetNetworkConfiguration(c *C) {
    endpoint1 := populateEndpoint(&InputEndpoint{})
    endpoint2 := populateEndpoint(&InputEndpoint{})
    endpoints := []InputEndpoint{*endpoint1, *endpoint2}
    subnet1 := MakeRandomString(10)
    subnet2 := MakeRandomString(10)
    config := NewNetworkConfigurationSet(endpoints, []string{subnet1, subnet2})
    xml, err := config.Serialize()
    c.Assert(err, IsNil)
    template := dedent.Dedent(`
        <ConfigurationSet>
          <ConfigurationSetType>NetworkConfiguration</ConfigurationSetType>
          <InputEndpoints>
            <InputEndpoint>
              <LoadBalancedEndpointSetName>%s</LoadBalancedEndpointSetName>
              <LocalPort>%v</LocalPort>
              <Name>%s</Name>
              <Port>%v</Port>
              <LoadBalancerProbe>
                <Path>%s</Path>
                <Port>%d</Port>
                <Protocol>%s</Protocol>
              </LoadBalancerProbe>
              <Protocol>%s</Protocol>
              <Vip>%s</Vip>
            </InputEndpoint>
            <InputEndpoint>
              <LoadBalancedEndpointSetName>%s</LoadBalancedEndpointSetName>
              <LocalPort>%v</LocalPort>
              <Name>%s</Name>
              <Port>%v</Port>
              <LoadBalancerProbe>
                <Path>%s</Path>
                <Port>%d</Port>
                <Protocol>%s</Protocol>
              </LoadBalancerProbe>
              <Protocol>%s</Protocol>
              <Vip>%s</Vip>
            </InputEndpoint>
          </InputEndpoints>
          <SubnetNames>
            <SubnetName>%s</SubnetName>
            <SubnetName>%s</SubnetName>
          </SubnetNames>
        </ConfigurationSet>`)
    expected := fmt.Sprintf(template,
        endpoint1.LoadBalancedEndpointSetName, endpoint1.LocalPort,
        endpoint1.Name, endpoint1.Port, endpoint1.LoadBalancerProbe.Path,
        endpoint1.LoadBalancerProbe.Port,
        endpoint1.LoadBalancerProbe.Protocol, endpoint1.Protocol,
        endpoint1.VIP,
        endpoint2.LoadBalancedEndpointSetName, endpoint2.LocalPort,
        endpoint2.Name, endpoint2.Port, endpoint2.LoadBalancerProbe.Path,
        endpoint2.LoadBalancerProbe.Port,
        endpoint2.LoadBalancerProbe.Protocol, endpoint2.Protocol,
        endpoint2.VIP,
        subnet1, subnet2)
    c.Check(strings.TrimSpace(xml), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestRole(c *C) {
    role := makeRole()
    config := role.ConfigurationSets[0]

    xml, err := role.Serialize()
    c.Assert(err, IsNil)
    template := dedent.Dedent(`
        <Role>
          <RoleName>%s</RoleName>
          <RoleType>PersistentVMRole</RoleType>
          <ConfigurationSets>
            <ConfigurationSet>
              <ConfigurationSetType>%s</ConfigurationSetType>
              <HostName>%s</HostName>
              <UserName>%s</UserName>
              <UserPassword>%s</UserPassword>
              <CustomData>%s</CustomData>
              <DisableSshPasswordAuthentication>%v</DisableSshPasswordAuthentication>
            </ConfigurationSet>
          </ConfigurationSets>
          <RoleSize>%s</RoleSize>
        </Role>`)
    expected := fmt.Sprintf(template, role.RoleName,
        config.ConfigurationSetType, config.Hostname, config.Username,
        config.Password, config.CustomData,
        config.DisableSSHPasswordAuthentication, role.RoleSize)
    c.Check(strings.TrimSpace(xml), Equals, strings.TrimSpace(expected))
}

func makePersistentVMRole(rolename string) string {
    // This template is from
    // http://msdn.microsoft.com/en-us/library/windowsazure/jj157193.aspx
    template := dedent.Dedent(`
        <PersistentVMRole xmlns="http://schemas.microsoft.com/windowsazure">
          <RoleName>%s</RoleName>
          <OsVersion>operating-system-version</OsVersion>
          <RoleType>PersistentVMRole</RoleType>
          <ConfigurationSets>
            <ConfigurationSet>
              <ConfigurationSetType>NetworkConfiguration</ConfigurationSetType>
              <InputEndpoints>
                <InputEndpoint>
                  <LoadBalancedEndpointSetName>name-of-load-balanced-endpoint-set</LoadBalancedEndpointSetName>
                  <LocalPort>1</LocalPort>
                  <Name>name-of-input-endpoint</Name>
                  <Port>1</Port>
                  <LoadBalancerProbe>
                    <Path>path-of-probe</Path>
                    <Port>1234</Port>
                    <Protocol>protocol-of-input-endpoint-1</Protocol>
                  </LoadBalancerProbe>
                  <Protocol>TCP|UDP</Protocol>
                  <Vip>virtual-ip-address-of-input-endpoint-1</Vip>
                </InputEndpoint>
                <InputEndpoint>
                  <LoadBalancedEndpointSetName>name-of-load-balanced-endpoint-set</LoadBalancedEndpointSetName>
                  <LocalPort>2</LocalPort>
                  <Name>name-of-input-endpoint</Name>
                  <Port>2</Port>
                  <LoadBalancerProbe>
                    <Path>path-of-probe</Path>
                    <Port>5678</Port>
                    <Protocol>protocol-of-input-endpoint-2</Protocol>
                  </LoadBalancerProbe>
                  <Protocol>TCP|UDP</Protocol>
                  <Vip>virtual-ip-address-of-input-endpoint-2</Vip>
                </InputEndpoint>
              </InputEndpoints>
              <SubnetNames>
                <SubnetName>name-of-subnet</SubnetName>
              </SubnetNames>
            </ConfigurationSet>
          </ConfigurationSets>
          <AvailabilitySetName>name-of-availability-set</AvailabilitySetName>
          <DataVirtualHardDisks>
            <DataVirtualHardDisk>
              <HostCaching>host-caching-mode-of-data-disk</HostCaching>
              <DiskName>new-or-existing-disk-name</DiskName>
              <Lun>logical-unit-number-of-data-disk</Lun>
              <LogicalDiskSizeInGB>size-of-data-disk</LogicalDiskSizeInGB>
              <MediaLink>path-to-vhd</MediaLink>
            </DataVirtualHardDisk>
          </DataVirtualHardDisks>
          <OSVirtualHardDisk>
            <HostCaching>host-caching-mode-of-os-disk</HostCaching>
            <DiskName>name-of-os-disk</DiskName>
            <MediaLink>path-to-vhd</MediaLink>
            <SourceImageName>image-used-to-create-os-disk</SourceImageName>
            <OS>operating-system-on-os-disk</OS>
          </OSVirtualHardDisk>
          <RoleSize>size-of-instance</RoleSize>
          <DefaultWinRmCertificateThumbprint>winrm-cert-thumbprint</DefaultWinRmCertificateThumbprint>
        </PersistentVMRole>`)
    return fmt.Sprintf(template, rolename)
}

func (suite *xmlSuite) TestPersistentVMRoleDeserialize(c *C) {
    expected := &PersistentVMRole{
        XMLNS:     XMLNS,
        RoleName:  "name-of-the-vm",
        OsVersion: "operating-system-version",
        RoleType:  "PersistentVMRole",
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                InputEndpoints: &[]InputEndpoint{
                    {
                        LoadBalancedEndpointSetName: "name-of-load-balanced-endpoint-set",
                        LocalPort:                   1,
                        Name:                        "name-of-input-endpoint",
                        Port:                        1,
                        LoadBalancerProbe: &LoadBalancerProbe{
                            Path:     "path-of-probe",
                            Port:     1234,
                            Protocol: "protocol-of-input-endpoint-1",
                        },
                        Protocol: "TCP|UDP",
                        VIP:      "virtual-ip-address-of-input-endpoint-1",
                    },
                    {
                        LoadBalancedEndpointSetName: "name-of-load-balanced-endpoint-set",
                        LocalPort:                   2,
                        Name:                        "name-of-input-endpoint",
                        Port:                        2,
                        LoadBalancerProbe: &LoadBalancerProbe{
                            Path:     "path-of-probe",
                            Port:     5678,
                            Protocol: "protocol-of-input-endpoint-2",
                        },
                        Protocol: "TCP|UDP",
                        VIP:      "virtual-ip-address-of-input-endpoint-2",
                    },
                },
                SubnetNames: &[]string{"name-of-subnet"},
            },
        },
        AvailabilitySetName: "name-of-availability-set",
        DataVirtualHardDisks: &[]DataVirtualHardDisk{
            {
                HostCaching:         "host-caching-mode-of-data-disk",
                DiskName:            "new-or-existing-disk-name",
                LUN:                 "logical-unit-number-of-data-disk",
                LogicalDiskSizeInGB: "size-of-data-disk",
                MediaLink:           "path-to-vhd",
            },
        },
        OSVirtualHardDisk: OSVirtualHardDisk{
            HostCaching:     "host-caching-mode-of-os-disk",
            DiskName:        "name-of-os-disk",
            MediaLink:       "path-to-vhd",
            SourceImageName: "image-used-to-create-os-disk",
            OS:              "operating-system-on-os-disk",
        },
        RoleSize: "size-of-instance",
        DefaultWinRmCertificateThumbprint: "winrm-cert-thumbprint",
    }

    template := makePersistentVMRole("name-of-the-vm")

    observed := &PersistentVMRole{}
    err := observed.Deserialize([]byte(template))
    c.Assert(err, IsNil)
    c.Assert(observed, DeepEquals, expected)
}

func (suite *xmlSuite) TestPersistentVMRoleSerialize(c *C) {
    role := &PersistentVMRole{
        XMLNS:     XMLNS,
        RoleName:  "name-of-the-vm",
        OsVersion: "operating-system-version",
        RoleType:  "PersistentVMRole",
        ConfigurationSets: []ConfigurationSet{
            {
                ConfigurationSetType: CONFIG_SET_NETWORK,
                InputEndpoints: &[]InputEndpoint{
                    {
                        LoadBalancedEndpointSetName: "name-of-load-balanced-endpoint-set",
                        LocalPort:                   1,
                        Name:                        "name-of-input-endpoint",
                        Port:                        1,
                        LoadBalancerProbe: &LoadBalancerProbe{
                            Path:     "path-of-probe",
                            Port:     1234,
                            Protocol: "protocol-of-input-endpoint-1",
                        },
                        Protocol: "TCP|UDP",
                        VIP:      "virtual-ip-address-of-input-endpoint-1",
                    },
                    {
                        LoadBalancedEndpointSetName: "name-of-load-balanced-endpoint-set",
                        LocalPort:                   2,
                        Name:                        "name-of-input-endpoint",
                        Port:                        2,
                        LoadBalancerProbe: &LoadBalancerProbe{
                            Path:     "path-of-probe",
                            Port:     5678,
                            Protocol: "protocol-of-input-endpoint-2",
                        },
                        Protocol: "TCP|UDP",
                        VIP:      "virtual-ip-address-of-input-endpoint-2",
                    },
                },
                SubnetNames: &[]string{"name-of-subnet"},
            },
        },
        AvailabilitySetName: "name-of-availability-set",
        DataVirtualHardDisks: &[]DataVirtualHardDisk{
            {
                HostCaching:         "host-caching-mode-of-data-disk",
                DiskName:            "new-or-existing-disk-name",
                LUN:                 "logical-unit-number-of-data-disk",
                LogicalDiskSizeInGB: "size-of-data-disk",
                MediaLink:           "path-to-vhd",
            },
        },
        OSVirtualHardDisk: OSVirtualHardDisk{
            HostCaching:     "host-caching-mode-of-os-disk",
            DiskName:        "name-of-os-disk",
            MediaLink:       "path-to-vhd",
            SourceImageName: "image-used-to-create-os-disk",
            OS:              "operating-system-on-os-disk",
        },
        RoleSize: "size-of-instance",
        DefaultWinRmCertificateThumbprint: "winrm-cert-thumbprint",
    }
    expected := makePersistentVMRole("name-of-the-vm")

    observed, err := role.Serialize()

    c.Assert(err, IsNil)
    c.Assert(strings.TrimSpace(observed), DeepEquals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestNetworkConfigurationSerialize(c *C) {
    // Template from
    // http://msdn.microsoft.com/en-us/library/windowsazure/jj157181.aspx
    expected := dedent.Dedent(`
        <NetworkConfiguration xmlns="http://schemas.microsoft.com/ServiceHosting/2011/07/NetworkConfiguration">
          <VirtualNetworkConfiguration>
            <Dns>
              <DnsServers>
                <DnsServer name="dns-server-name" IPAddress="IPV4-address-of-the-server"></DnsServer>
              </DnsServers>
            </Dns>
            <LocalNetworkSites>
              <LocalNetworkSite name="local-site-name">
                <AddressSpace>
                  <AddressPrefix>CIDR-identifier</AddressPrefix>
                </AddressSpace>
                <VPNGatewayAddress>IPV4-address-of-the-vpn-gateway</VPNGatewayAddress>
              </LocalNetworkSite>
            </LocalNetworkSites>
            <VirtualNetworkSites>
              <VirtualNetworkSite name="virtual-network-name" AffinityGroup="affinity-group-name">
                <AddressSpace>
                  <AddressPrefix>CIDR-identifier</AddressPrefix>
                </AddressSpace>
                <Subnets>
                  <Subnet name="subnet-name">
                    <AddressPrefix>CIDR-identifier</AddressPrefix>
                  </Subnet>
                </Subnets>
                <DnsServersRef>
                  <DnsServerRef name="primary-DNS-name"></DnsServerRef>
                </DnsServersRef>
                <Gateway profile="Small">
                  <VPNClientAddressPool>
                    <AddressPrefix>CIDR-identifier</AddressPrefix>
                  </VPNClientAddressPool>
                  <ConnectionsToLocalNetwork>
                    <LocalNetworkSiteRef name="local-site-name">
                      <Connection type="connection-type"></Connection>
                    </LocalNetworkSiteRef>
                  </ConnectionsToLocalNetwork>
                </Gateway>
              </VirtualNetworkSite>
            </VirtualNetworkSites>
          </VirtualNetworkConfiguration>
        </NetworkConfiguration>`)

    input := NetworkConfiguration{
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

    observed, err := input.Serialize()
    c.Assert(err, IsNil)
    c.Assert(strings.TrimSpace(observed), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestNetworkConfigurationSerializeMinimal(c *C) {
    expected := fmt.Sprintf(
        "<NetworkConfiguration xmlns=\"%s\"></NetworkConfiguration>",
        XMLNS_NC)
    input := NetworkConfiguration{XMLNS: XMLNS_NC}
    observed, err := input.Serialize()
    c.Assert(err, IsNil)
    c.Assert(strings.TrimSpace(observed), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestNetworkConfigurationSerializeSimpleVirtualNetworkSite(c *C) {
    expected := dedent.Dedent(`
        <NetworkConfiguration xmlns="http://schemas.microsoft.com/ServiceHosting/2011/07/NetworkConfiguration">
          <VirtualNetworkConfiguration>
            <VirtualNetworkSites>
              <VirtualNetworkSite name="virtual-network-name" AffinityGroup="affinity-group-name">
                <AddressSpace>
                  <AddressPrefix>CIDR-identifier</AddressPrefix>
                </AddressSpace>
              </VirtualNetworkSite>
            </VirtualNetworkSites>
          </VirtualNetworkConfiguration>
        </NetworkConfiguration>`)
    input := NetworkConfiguration{
        XMLNS: XMLNS_NC,
        VirtualNetworkSites: &[]VirtualNetworkSite{
            {
                Name:          "virtual-network-name",
                AffinityGroup: "affinity-group-name",
                AddressSpacePrefixes: []string{
                    "CIDR-identifier",
                },
            },
        },
    }
    observed, err := input.Serialize()
    c.Assert(err, IsNil)
    c.Assert(strings.TrimSpace(observed), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestCreateAffinityGroup(c *C) {
    expected := dedent.Dedent(`
        <CreateAffinityGroup xmlns="http://schemas.microsoft.com/windowsazure">
          <Name>affinity-group-name</Name>
          <Label>base64-encoded-affinity-group-label</Label>
          <Description>affinity-group-description</Description>
          <Location>location</Location>
        </CreateAffinityGroup>`)

    input := CreateAffinityGroup{
        XMLNS:       XMLNS,
        Name:        "affinity-group-name",
        Label:       "base64-encoded-affinity-group-label",
        Description: "affinity-group-description",
        Location:    "location"}

    observed, err := input.Serialize()
    c.Assert(err, IsNil)
    c.Assert(strings.TrimSpace(observed), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestNewCreateAffinityGroup(c *C) {
    name := "name"
    label := "label"
    description := "description"
    location := "location"
    ag := NewCreateAffinityGroup(name, label, description, location)
    base64label := base64.StdEncoding.EncodeToString([]byte(label))
    c.Check(ag.XMLNS, Equals, XMLNS)
    c.Check(ag.Name, Equals, name)
    c.Check(ag.Label, Equals, base64label)
    c.Check(ag.Description, Equals, description)
    c.Check(ag.Location, Equals, location)
}

func (suite *xmlSuite) TestUpdateAffinityGroup(c *C) {
    expected := dedent.Dedent(`
        <UpdateAffinityGroup xmlns="http://schemas.microsoft.com/windowsazure">
          <Label>base64-encoded-affinity-group-label</Label>
          <Description>affinity-group-description</Description>
        </UpdateAffinityGroup>`)
    input := UpdateAffinityGroup{
        XMLNS:       XMLNS,
        Label:       "base64-encoded-affinity-group-label",
        Description: "affinity-group-description"}

    observed, err := input.Serialize()
    c.Assert(err, IsNil)
    c.Assert(strings.TrimSpace(observed), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestNewUpdateAffinityGroup(c *C) {
    label := "label"
    description := "description"
    ag := NewUpdateAffinityGroup(label, description)
    base64label := base64.StdEncoding.EncodeToString([]byte(label))
    c.Check(ag.XMLNS, Equals, XMLNS)
    c.Check(ag.Label, Equals, base64label)
    c.Check(ag.Description, Equals, description)
}

func (suite *xmlSuite) TestNetworkConfigurationDeserialize(c *C) {
    // Template from
    // http://msdn.microsoft.com/en-us/library/windowsazure/jj157196.aspx
    input := `
        <NetworkConfiguration xmlns="http://schemas.microsoft.com/ServiceHosting/2011/07/NetworkConfiguration">
          <VirtualNetworkConfiguration>
            <Dns>
              <DnsServers>
                <DnsServer name="dns-server-name" IPAddress="IPV4-address-of-the-server"></DnsServer>
              </DnsServers>
            </Dns>
            <LocalNetworkSites>
              <LocalNetworkSite name="local-site-name">
                <AddressSpace>
                  <AddressPrefix>CIDR-identifier</AddressPrefix>
                </AddressSpace>
                <VPNGatewayAddress>IPV4-address-of-the-vpn-gateway</VPNGatewayAddress>
              </LocalNetworkSite>
            </LocalNetworkSites>
            <VirtualNetworkSites>
              <VirtualNetworkSite name="virtual-network-name" AffinityGroup="affinity-group-name">
                <Label>label-for-the-site</Label>
                <AddressSpace>
                  <AddressPrefix>CIDR-identifier</AddressPrefix>
                </AddressSpace>
                <Subnets>
                  <Subnet name="subnet-name">
                    <AddressPrefix>CIDR-identifier</AddressPrefix>
                  </Subnet>
                </Subnets>
                <DnsServersRef>
                  <DnsServerRef name="primary-DNS-name"></DnsServerRef>
                </DnsServersRef>
                <Gateway profile="Small">
                  <VPNClientAddressPool>
                    <AddressPrefix>CIDR-identifier</AddressPrefix>
                  </VPNClientAddressPool>
                  <ConnectionsToLocalNetwork>
                    <LocalNetworkSiteRef name="local-site-name">
                      <Connection type="connection-type"></Connection>
                    </LocalNetworkSiteRef>
                  </ConnectionsToLocalNetwork>
                </Gateway>
              </VirtualNetworkSite>
            </VirtualNetworkSites>
          </VirtualNetworkConfiguration>
        </NetworkConfiguration>`
    expected := &NetworkConfiguration{
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
    networkConfig := &NetworkConfiguration{}
    err := networkConfig.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    // Check sub-components of the overall structure.
    c.Check(networkConfig.DNS, DeepEquals, expected.DNS)
    c.Check(networkConfig.LocalNetworkSites, DeepEquals, expected.LocalNetworkSites)
    c.Check(networkConfig.VirtualNetworkSites, DeepEquals, expected.VirtualNetworkSites)
    // Check the whole thing.
    c.Check(networkConfig, DeepEquals, expected)
}

func (suite *xmlSuite) TestDeployment(c *C) {
    deployment := makeDeployment()
    dns := deployment.DNS[0]
    role := deployment.RoleList[0]
    config := role.ConfigurationSets[0]

    xml, err := deployment.Serialize()
    c.Assert(err, IsNil)
    template := dedent.Dedent(`
        <Deployment xmlns="http://schemas.microsoft.com/windowsazure" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
          <Name>%s</Name>
          <DeploymentSlot>%s</DeploymentSlot>
          <Label>%s</Label>
          <RoleInstanceList></RoleInstanceList>
          <RoleList>
            <Role>
              <RoleName>%s</RoleName>
              <RoleType>PersistentVMRole</RoleType>
              <ConfigurationSets>
                <ConfigurationSet>
                  <ConfigurationSetType>%s</ConfigurationSetType>
                  <HostName>%s</HostName>
                  <UserName>%s</UserName>
                  <UserPassword>%s</UserPassword>
                  <CustomData>%s</CustomData>
                  <DisableSshPasswordAuthentication>%v</DisableSshPasswordAuthentication>
                </ConfigurationSet>
              </ConfigurationSets>
              <RoleSize>%s</RoleSize>
            </Role>
          </RoleList>
          <VirtualNetworkName>%s</VirtualNetworkName>
          <Dns>
            <DnsServers>
              <DnsServer>
                <Name>%s</Name>
                <Address>%s</Address>
              </DnsServer>
            </DnsServers>
          </Dns>
          <ExtendedProperties></ExtendedProperties>
        </Deployment>`)
    expected := fmt.Sprintf(template, deployment.Name,
        deployment.DeploymentSlot, deployment.Label,
        role.RoleName, config.ConfigurationSetType, config.Hostname,
        config.Username, config.Password, config.CustomData,
        config.DisableSSHPasswordAuthentication, role.RoleSize,
        deployment.VirtualNetworkName, dns.Name, dns.Address)
    c.Check(strings.TrimSpace(xml), Equals, strings.TrimSpace(expected))
}

// From http://msdn.microsoft.com/en-us/library/windowsazure/ee460804.aspx
var deploymentXML = `
<?xml version="1.0" encoding="utf-8"?>
<Deployment xmlns="http://schemas.microsoft.com/windowsazure">
  <Name>name-of-deployment</Name>
  <DeploymentSlot>current-deployment-environment</DeploymentSlot>
  <PrivateID>deployment-id</PrivateID>
  <Status>status-of-deployment</Status>
  <Label>base64-encoded-name-of-deployment</Label>
  <Url>http://name-of-deployment.cloudapp.net</Url>
  <Configuration>base-64-encoded-configuration-file</Configuration>
  <RoleInstanceList>
    <RoleInstance>
      <RoleName>name-of-role</RoleName>
      <InstanceName>name-of-role-instance</InstanceName>
      <InstanceStatus>status-of-role-instance</InstanceStatus>
      <InstanceUpgradeDomain>update-domain-of-role-instance</InstanceUpgradeDomain>
      <InstanceFaultDomain>fault-domain-of-role-instance</InstanceFaultDomain>
      <InstanceSize>size-of-role-instance</InstanceSize>
      <InstanceStateDetails>state-of-role-instance</InstanceStateDetails>
      <InstanceErrorCode>error-code-returned-for-role-instance</InstanceErrorCode>
      <IpAddress>ip-address-of-role-instance</IpAddress>
      <InstanceEndpoints>
	<InstanceEndpoint>
	  <Name>name-of-endpoint</Name>
	  <Vip>virtual-ip-address-of-instance-endpoint</Vip>
	  <PublicPort>1234</PublicPort>
	  <LocalPort>5678</LocalPort>
	  <Protocol>protocol-of-instance-endpoint</Protocol>
	</InstanceEndpoint>
      </InstanceEndpoints>
      <PowerState>state-of-role-instance</PowerState>
      <HostName>dns-name-of-service</HostName>
      <RemoteAccessCertificateThumbprint>cert-thumbprint-for-remote-access</RemoteAccessCertificateThumbprint>
    </RoleInstance>
  </RoleInstanceList>
  <UpgradeStatus>
    <UpgradeType>auto|manual</UpgradeType>
    <CurrentUpgradeDomainState>before|during</CurrentUpgradeDomainState>
    <CurrentUpgradeDomain>n</CurrentUpgradeDomain>
  </UpgradeStatus>
  <UpgradeDomainCount>number-of-upgrade-domains-in-deployment</UpgradeDomainCount>
  <RoleList>
    <Role>
      <RoleName>name-of-role</RoleName>
      <OsVersion>operating-system-version</OsVersion>
      <ConfigurationSets>
	<ConfigurationSet>
	  <ConfigurationSetType>LinuxProvisioningConfiguration</ConfigurationSetType>
          <DisableSshPasswordAuthentication>false</DisableSshPasswordAuthentication>
	</ConfigurationSet>
      </ConfigurationSets>
    </Role>
    <Role>
      <RoleName>name-of-role</RoleName>
      <OsVersion>operating-system-version</OsVersion>
      <RoleType>PersistentVMRole</RoleType>
      <ConfigurationSets>
	<ConfigurationSet>
	  <ConfigurationSetType>NetworkConfiguration</ConfigurationSetType>
            <InputEndpoints>
	      <InputEndpoint>
	        <Port>2222</Port>
	        <LocalPort>111</LocalPort>
  	        <Protocol>TCP</Protocol>
  	        <Name>test-name</Name>
  	      </InputEndpoint>
	    </InputEndpoints>
	  <SubnetNames>
	    <SubnetName>name-of-subnet</SubnetName>
	  </SubnetNames>
	</ConfigurationSet>
      </ConfigurationSets>
      <AvailabilitySetName>name-of-availability-set</AvailabilitySetName>
      <DataVirtualHardDisks>
	<DataVirtualHardDisk>
	  <HostCaching>host-caching-mode-of-data-disk</HostCaching>
	  <DiskName>name-of-data-disk</DiskName>
	  <Lun>logical-unit-number-of-data-disk</Lun>
	  <LogicalDiskSizeInGB>size-of-data-disk</LogicalDiskSizeInGB>
	  <MediaLink>path-to-vhd</MediaLink>
	</DataVirtualHardDisk>
      </DataVirtualHardDisks>
      <OSVirtualHardDisk>
	<HostCaching>host-caching-mode-of-os-disk</HostCaching>
	<DiskName>name-of-os-disk</DiskName>
	<MediaLink>path-to-vhd</MediaLink>
	<SourceImageName>image-used-to-create-os-disk</SourceImageName>
	<OS>operating-system-on-os-disk</OS>
      </OSVirtualHardDisk>
      <RoleSize>size-of-instance</RoleSize>
    </Role>
  </RoleList>
  <SdkVersion>sdk-version-used-to-create-package</SdkVersion>
  <Locked>status-of-deployment-write-allowed</Locked>
  <RollbackAllowed>rollback-operation-allowed</RollbackAllowed>
  <VirtualNetworkName>name-of-virtual-network</VirtualNetworkName>
  <Dns>
    <DnsServers>
      <DnsServer>
	<Name>name-of-dns-server</Name>
	<Address>address-of-dns-server</Address>
      </DnsServer>
    </DnsServers>
  </Dns>
  <ExtendedProperties>
    <ExtendedProperty>
      <Name>name-of-property</Name>
      <Value>value-of-property</Value>
    </ExtendedProperty>
  </ExtendedProperties>
  <PersistentVMDowntime>
    <StartTime>start-of-downtime</StartTime>
    <EndTime>end-of-downtime</EndTime>
    <Status>status-of-downtime</Status>
  </PersistentVMDowntime>
  <VirtualIPs>
    <VirtualIP>
      <Address>virtual-ip-address-of-deployment</Address>
    </VirtualIP>
  </VirtualIPs>
  <ExtensionConfiguration>
    <AllRoles>
      <Extension>
	<Id>identifier-of-extension</Id>
      </Extension>
      ...
    </AllRoles>
    <NamedRoles>
      <Role>
	<RoleName>role_name1</RoleName>
	<Extensions>
	  <Extension>
	    <Id>identifier-of-extension</Id>
	  </Extension>
	  ...
	</Extensions>
      </Role>
    </NamedRoles>
  </ExtensionConfiguration>
</Deployment>
`

func (suite *xmlSuite) TestDeploymentWRTGetDeployment(c *C) {
    expected := &Deployment{
        XMLNS:          "http://schemas.microsoft.com/windowsazure",
        Name:           "name-of-deployment",
        DeploymentSlot: "current-deployment-environment",
        PrivateID:      "deployment-id",
        Status:         "status-of-deployment",
        Label:          "base64-encoded-name-of-deployment",
        URL:            "http://name-of-deployment.cloudapp.net",
        Configuration:  "base-64-encoded-configuration-file",
        RoleInstanceList: []RoleInstance{
            {
                RoleName:              "name-of-role",
                InstanceName:          "name-of-role-instance",
                InstanceStatus:        "status-of-role-instance",
                InstanceUpgradeDomain: "update-domain-of-role-instance",
                InstanceFaultDomain:   "fault-domain-of-role-instance",
                InstanceSize:          "size-of-role-instance",
                InstanceStateDetails:  "state-of-role-instance",
                InstanceErrorCode:     "error-code-returned-for-role-instance",
                IPAddress:             "ip-address-of-role-instance",
                InstanceEndpoints: []InstanceEndpoint{
                    {
                        Name:       "name-of-endpoint",
                        VIP:        "virtual-ip-address-of-instance-endpoint",
                        PublicPort: 1234,
                        LocalPort:  5678,
                        Protocol:   "protocol-of-instance-endpoint",
                    },
                },
                PowerState: "state-of-role-instance",
                HostName:   "dns-name-of-service",
                RemoteAccessCertificateThumbprint: "cert-thumbprint-for-remote-access",
            },
        },
        UpgradeDomainCount: "number-of-upgrade-domains-in-deployment",
        RoleList: []Role{
            {
                RoleName: "name-of-role",
                ConfigurationSets: []ConfigurationSet{
                    {
                        ConfigurationSetType:             "LinuxProvisioningConfiguration",
                        DisableSSHPasswordAuthentication: "false",
                    },
                },
            },
            {
                RoleName: "name-of-role",
                RoleType: "PersistentVMRole",
                ConfigurationSets: []ConfigurationSet{
                    {
                        ConfigurationSetType: CONFIG_SET_NETWORK,
                        InputEndpoints: &[]InputEndpoint{
                            {
                                Name:      "test-name",
                                Port:      2222,
                                LocalPort: 111,
                                Protocol:  "TCP",
                            },
                        },
                        SubnetNames: &[]string{"name-of-subnet"},
                    },
                },
                OSVirtualHardDisk: []OSVirtualHardDisk{
                    {
                        HostCaching:     "host-caching-mode-of-os-disk",
                        DiskName:        "name-of-os-disk",
                        MediaLink:       "path-to-vhd",
                        SourceImageName: "image-used-to-create-os-disk",
                        OS:              "operating-system-on-os-disk",
                    },
                },
                RoleSize: "size-of-instance",
            },
        },
        SDKVersion:         "sdk-version-used-to-create-package",
        Locked:             "status-of-deployment-write-allowed",
        RollbackAllowed:    "rollback-operation-allowed",
        VirtualNetworkName: "name-of-virtual-network",
        DNS: []DnsServer{
            {
                Name:    "name-of-dns-server",
                Address: "address-of-dns-server",
            },
        },
        ExtendedProperties: []ExtendedProperty{
            {
                Name:  "name-of-property",
                Value: "value-of-property",
            },
        },
    }
    observed := &Deployment{}
    err := observed.Deserialize([]byte(deploymentXML))
    c.Assert(err, IsNil)
    c.Assert(observed, DeepEquals, expected)
}

func (suite *xmlSuite) TestDeploymentGetFQDNExtractsFQDN(c *C) {
    deployment := &Deployment{}
    err := deployment.Deserialize([]byte(deploymentXML))
    c.Assert(err, IsNil)
    fqdn, err := deployment.GetFQDN()
    c.Assert(err, IsNil)
    c.Assert(fqdn, Equals, "name-of-deployment.cloudapp.net")
}

var deploymentXMLEmptyURL = `
<?xml version="1.0" encoding="utf-8"?>
<Deployment xmlns="http://schemas.microsoft.com/windowsazure">
  <Name>name-of-deployment</Name>
  <Label>base64-encoded-name-of-deployment</Label>
  <Url></Url>
</Deployment>
`

func (suite *xmlSuite) TestDeploymentGetFQDNErrorsIfURLIsEmpty(c *C) {
    deployment := &Deployment{}
    err := deployment.Deserialize([]byte(deploymentXMLEmptyURL))
    c.Assert(err, IsNil)
    _, err = deployment.GetFQDN()
    c.Check(err, ErrorMatches, ".*URL field is empty.*")
}

var deploymentXMLFaultyURL = `
<?xml version="1.0" encoding="utf-8"?>
<Deployment xmlns="http://schemas.microsoft.com/windowsazure">
  <Name>name-of-deployment</Name>
  <Label>base64-encoded-name-of-deployment</Label>
  <Url>%z</Url>
</Deployment>
`

func (suite *xmlSuite) TestDeploymentGetFQDNErrorsIfURLCannotBeParsed(c *C) {
    deployment := &Deployment{}
    err := deployment.Deserialize([]byte(deploymentXMLFaultyURL))
    c.Assert(err, IsNil)
    _, err = deployment.GetFQDN()
    c.Check(err, ErrorMatches, ".*invalid URL.*")
}

func (suite *xmlSuite) TestNewDeploymentForCreateVMDeployment(c *C) {
    name := "deploymentName"
    deploymentSlot := "staging"
    label := "deploymentLabel"
    vhd := NewOSVirtualHardDisk("hostCaching", "diskLabel", "diskName", "mediaLink", "sourceImageName", "os")
    roles := []Role{*NewRole("size", "name", []ConfigurationSet{}, []OSVirtualHardDisk{*vhd})}
    virtualNetworkName := "network"

    deployment := NewDeploymentForCreateVMDeployment(name, deploymentSlot, label, roles, virtualNetworkName)

    c.Check(deployment.XMLNS, Equals, XMLNS)
    c.Check(deployment.XMLNS_I, Equals, XMLNS_I)
    c.Check(deployment.Name, Equals, name)
    c.Check(deployment.DeploymentSlot, Equals, deploymentSlot)
    c.Check(deployment.RoleList, DeepEquals, roles)
    decodedLabel, err := base64.StdEncoding.DecodeString(deployment.Label)
    c.Assert(err, IsNil)
    c.Check(string(decodedLabel), Equals, label)
    c.Check(deployment.VirtualNetworkName, Equals, virtualNetworkName)
}

func (suite *xmlSuite) TestCreateVirtualHardDiskMediaLinkHappyPath(c *C) {
    mediaLink := CreateVirtualHardDiskMediaLink("storage-name", "storage/path")
    c.Check(mediaLink, Equals, "http://storage-name.blob.core.windows.net/storage/path")
}

func (suite *xmlSuite) TestCreateVirtualHardDiskMediaLinkChecksParams(c *C) {
    c.Check(
        func() { CreateVirtualHardDiskMediaLink("foo^bar", "valid") },
        PanicMatches, "'foo\\^bar' contains URI special characters")
    c.Check(
        func() { CreateVirtualHardDiskMediaLink("valid", "a/foo^bar/test") },
        PanicMatches, "'foo\\^bar' contains URI special characters")
}

func (suite *xmlSuite) TestCreateStorageServiceInput(c *C) {
    s := makeCreateStorageServiceInput()
    extProperty := s.ExtendedProperties[0]
    xml, err := s.Serialize()
    c.Assert(err, IsNil)
    template := dedent.Dedent(`
        <CreateStorageServiceInput xmlns="http://schemas.microsoft.com/windowsazure">
          <ServiceName>%s</ServiceName>
          <Label>%s</Label>
          <Description>%s</Description>
          <Location>%s</Location>
          <AffinityGroup>%s</AffinityGroup>
          <GeoReplicationEnabled>%s</GeoReplicationEnabled>
          <ExtendedProperties>
            <ExtendedProperty>
              <Name>%s</Name>
              <Value>%s</Value>
            </ExtendedProperty>
          </ExtendedProperties>
        </CreateStorageServiceInput>`)
    expected := fmt.Sprintf(template, s.ServiceName, s.Label, s.Description,
        s.Location, s.AffinityGroup, s.GeoReplicationEnabled, extProperty.Name,
        extProperty.Value)
    c.Assert(strings.TrimSpace(xml), Equals, strings.TrimSpace(expected))
}

//
// Tests for Unmarshallers
//

func (suite *xmlSuite) TestStorageServicesUnmarshal(c *C) {
    inputTemplate := `
        <?xml version="1.0" encoding="utf-8"?>
        <StorageServices xmlns="http://schemas.microsoft.com/windowsazure">
          <StorageService>
            <Url>%s</Url>
            <ServiceName>%s</ServiceName>
            <StorageServiceProperties>
              <Description>%s</Description>
              <AffinityGroup>%s</AffinityGroup>
              <Label>%s</Label>
              <Status>%s</Status>
              <Endpoints>
                <Endpoint>%s</Endpoint>
                <Endpoint>%s</Endpoint>
                <Endpoint>%s</Endpoint>
              </Endpoints>
              <GeoReplicationEnabled>%s</GeoReplicationEnabled>
              <GeoPrimaryRegion>%s</GeoPrimaryRegion>
              <StatusOfPrimary>%s</StatusOfPrimary>
              <LastGeoFailoverTime>%s</LastGeoFailoverTime>
              <GeoSecondaryRegion>%s</GeoSecondaryRegion>
              <StatusOfSecondary>%s</StatusOfSecondary>
              <ExtendedProperties>
                <ExtendedProperty>
                  <Name>%s</Name>
                  <Value>%s</Value>
                </ExtendedProperty>
                <ExtendedProperty>
                  <Name>%s</Name>
                  <Value>%s</Value>
                </ExtendedProperty>
              </ExtendedProperties>
            </StorageServiceProperties>
          </StorageService>
        </StorageServices>`
    url := MakeRandomString(10)
    servicename := MakeRandomString(10)
    desc := MakeRandomString(10)
    affinity := MakeRandomString(10)
    label := MakeRandomString(10)
    status := MakeRandomString(10)
    blobEndpoint := MakeRandomString(10)
    queueEndpoint := MakeRandomString(10)
    tableEndpoint := MakeRandomString(10)
    geoRepl := BoolToString(MakeRandomBool())
    geoRegion := MakeRandomString(10)
    statusPrimary := MakeRandomString(10)
    failoverTime := MakeRandomString(10)
    geoSecRegion := MakeRandomString(10)
    statusSec := MakeRandomString(10)
    p1Name := MakeRandomString(10)
    p1Val := MakeRandomString(10)
    p2Name := MakeRandomString(10)
    p2Val := MakeRandomString(10)

    input := fmt.Sprintf(inputTemplate, url, servicename, desc, affinity,
        label, status, blobEndpoint, queueEndpoint, tableEndpoint, geoRepl,
        geoRegion, statusPrimary, failoverTime, geoSecRegion, statusSec,
        p1Name, p1Val, p2Name, p2Val)
    data := []byte(input)

    services := &StorageServices{}
    err := services.Deserialize(data)
    c.Assert(err, IsNil)

    c.Check(len(services.StorageServices), Equals, 1)
    s := services.StorageServices[0]

    // Oh jeez, here we go....
    c.Check(s.URL, Equals, url)
    c.Check(s.ServiceName, Equals, servicename)
    c.Check(s.Description, Equals, desc)
    c.Check(s.AffinityGroup, Equals, affinity)
    c.Check(s.Label, Equals, label)
    c.Check(s.Status, Equals, status)
    c.Check(s.GeoReplicationEnabled, Equals, geoRepl)
    c.Check(s.GeoPrimaryRegion, Equals, geoRegion)
    c.Check(s.StatusOfPrimary, Equals, statusPrimary)
    c.Check(s.LastGeoFailoverTime, Equals, failoverTime)
    c.Check(s.GeoSecondaryRegion, Equals, geoSecRegion)
    c.Check(s.StatusOfSecondary, Equals, statusSec)

    endpoints := s.Endpoints
    c.Check(len(endpoints), Equals, 3)
    c.Check(endpoints[0], Equals, blobEndpoint)
    c.Check(endpoints[1], Equals, queueEndpoint)
    c.Check(endpoints[2], Equals, tableEndpoint)

    properties := s.ExtendedProperties
    c.Check(properties[0].Name, Equals, p1Name)
    c.Check(properties[0].Value, Equals, p1Val)
    c.Check(properties[1].Name, Equals, p2Name)
    c.Check(properties[1].Value, Equals, p2Val)
}

func (suite *xmlSuite) TestBlobEnumerationResuts(c *C) {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <EnumerationResults ContainerName="http://myaccount.blob.core.windows.net/mycontainer">
          <Prefix>prefix</Prefix>
          <Marker>marker</Marker>
          <MaxResults>maxresults</MaxResults>
          <Delimiter>delimiter</Delimiter>
          <Blobs>
            <Blob>
              <Name>blob-name</Name>
              <Snapshot>snapshot-date-time</Snapshot>
              <Url>blob-address</Url>
              <Properties>
                <Last-Modified>last-modified</Last-Modified>
                <Etag>etag</Etag>
                <Content-Length>size-in-bytes</Content-Length>
                <Content-Type>blob-content-type</Content-Type>
                <Content-Encoding />
                <Content-Language />
                <Content-MD5 />
                <Cache-Control />
                <x-ms-blob-sequence-number>sequence-number</x-ms-blob-sequence-number>
                <BlobType>blobtype</BlobType>
                <LeaseStatus>leasestatus</LeaseStatus>
                <LeaseState>leasestate</LeaseState>
                <LeaseDuration>leasesduration</LeaseDuration>
                <CopyId>id</CopyId>
                <CopyStatus>copystatus</CopyStatus>
                <CopySource>copysource</CopySource>
                <CopyProgress>copyprogress</CopyProgress>
                <CopyCompletionTime>copycompletiontime</CopyCompletionTime>
                <CopyStatusDescription>copydesc</CopyStatusDescription>
              </Properties>
              <Metadata>
                <MetaName1>metadataname1</MetaName1>
                <MetaName2>metadataname2</MetaName2>
              </Metadata>
            </Blob>
            <BlobPrefix>
              <Name>blob-prefix</Name>
            </BlobPrefix>
          </Blobs>
          <NextMarker />
        </EnumerationResults>`
    data := []byte(input)
    r := &BlobEnumerationResults{}
    err := r.Deserialize(data)
    c.Assert(err, IsNil)
    c.Check(r.ContainerName, Equals, "http://myaccount.blob.core.windows.net/mycontainer")
    c.Check(r.Prefix, Equals, "prefix")
    c.Check(r.Marker, Equals, "marker")
    c.Check(r.MaxResults, Equals, "maxresults")
    c.Check(r.Delimiter, Equals, "delimiter")
    c.Check(r.NextMarker, Equals, "")
    b := r.Blobs[0]
    c.Check(b.Name, Equals, "blob-name")
    c.Check(b.Snapshot, Equals, "snapshot-date-time")
    c.Check(b.URL, Equals, "blob-address")
    c.Check(b.LastModified, Equals, "last-modified")
    c.Check(b.ETag, Equals, "etag")
    c.Check(b.ContentLength, Equals, "size-in-bytes")
    c.Check(b.ContentType, Equals, "blob-content-type")
    c.Check(b.BlobSequenceNumber, Equals, "sequence-number")
    c.Check(b.BlobType, Equals, "blobtype")
    c.Check(b.LeaseStatus, Equals, "leasestatus")
    c.Check(b.LeaseState, Equals, "leasestate")
    c.Check(b.LeaseDuration, Equals, "leasesduration")
    c.Check(b.CopyID, Equals, "id")
    c.Check(b.CopyStatus, Equals, "copystatus")
    c.Check(b.CopySource, Equals, "copysource")
    c.Check(b.CopyProgress, Equals, "copyprogress")
    c.Check(b.CopyCompletionTime, Equals, "copycompletiontime")
    c.Check(b.CopyStatusDescription, Equals, "copydesc")
    m1 := b.Metadata.Items[0]
    m2 := b.Metadata.Items[1]
    c.Check(m1.Name(), Equals, "MetaName1")
    c.Check(m1.Value, Equals, "metadataname1")
    c.Check(m2.Name(), Equals, "MetaName2")
    c.Check(m2.Value, Equals, "metadataname2")
    prefix := r.BlobPrefixes[0]
    c.Check(prefix, Equals, "blob-prefix")
}

func (suite *xmlSuite) TestStorageAccountKeysUnmarshal(c *C) {
    template := `
        <?xml version="1.0" encoding="utf-8"?>
          <StorageService xmlns="http://schemas.microsoft.com/windowsazure">
            <Url>%s</Url>
            <StorageServiceKeys>
              <Primary>%s</Primary>
              <Secondary>%s</Secondary>
            </StorageServiceKeys>
          </StorageService>`
    url := MakeRandomString(10)
    key1 := MakeRandomString(10)
    key2 := MakeRandomString(10)
    input := fmt.Sprintf(template, url, key1, key2)
    data := []byte(input)

    keys := &StorageAccountKeys{}
    err := keys.Deserialize(data)
    c.Assert(err, IsNil)
    c.Check(keys.URL, Equals, url)
    c.Check(keys.Primary, Equals, key1)
    c.Check(keys.Secondary, Equals, key2)
}

// Tests for object factory functions.

func (suite *xmlSuite) TestNewRole(c *C) {
    rolesize := MakeRandomString(10)
    rolename := MakeRandomString(10)
    config := makeLinuxProvisioningConfiguration()
    configset := []ConfigurationSet{*config}
    vhd := NewOSVirtualHardDisk("hostCaching", "diskLabel", "diskName", "mediaLink", "sourceImageName", "os")

    role := NewRole(rolesize, rolename, configset, []OSVirtualHardDisk{*vhd})
    c.Check(role.RoleSize, Equals, rolesize)
    c.Check(role.RoleName, Equals, rolename)
    c.Check(role.ConfigurationSets, DeepEquals, configset)
    c.Check(role.RoleType, Equals, "PersistentVMRole")
}

func (suite *xmlSuite) TestNewLinuxProvisioningConfiguration(c *C) {
    hostname := MakeRandomString(10)
    username := MakeRandomString(10)
    password := MakeRandomString(10)
    disablessh := BoolToString(MakeRandomBool())
    customdata := MakeRandomString(10)

    config := NewLinuxProvisioningConfigurationSet(
        hostname, username, password, customdata, disablessh)
    c.Check(config.Hostname, Equals, hostname)
    c.Check(config.Username, Equals, username)
    c.Check(config.Password, Equals, password)
    c.Check(config.CustomData, Equals, customdata)
    c.Check(config.DisableSSHPasswordAuthentication, Equals, disablessh)
    c.Check(config.ConfigurationSetType, Equals, "LinuxProvisioningConfiguration")
}

func (suite *xmlSuite) TestNewNetworkConfiguration(c *C) {
    name := "name1"
    port := 242
    localPort := 922
    protocol := "TCP"
    bName := "bname1"
    inputendpoint := InputEndpoint{
        LoadBalancedEndpointSetName: bName, LocalPort: localPort, Name: name, Port: port, Protocol: protocol}

    config := NewNetworkConfigurationSet([]InputEndpoint{inputendpoint}, []string{"subnet-1", "subnet-2"})
    inputEndpoints := *config.InputEndpoints
    c.Check(inputEndpoints, HasLen, 1)
    inputEndpoint := inputEndpoints[0]
    c.Check(inputEndpoint.Name, Equals, name)
    c.Check(inputEndpoint.Port, Equals, port)
    c.Check(inputEndpoint.Protocol, Equals, protocol)
    c.Check(inputEndpoint.LoadBalancedEndpointSetName, Equals, bName)
    c.Check(inputEndpoint.LocalPort, Equals, localPort)
    c.Check(config.ConfigurationSetType, Equals, CONFIG_SET_NETWORK)
    c.Check(*config.SubnetNames, DeepEquals, []string{"subnet-1", "subnet-2"})
}

func (suite *xmlSuite) TestNewOSVirtualHardDisk(c *C) {
    var hostcaching HostCachingType = "ReadWrite"
    disklabel := MakeRandomString(10)
    diskname := MakeRandomString(10)
    MediaLink := MakeRandomString(10)
    SourceImageName := MakeRandomString(10)
    OS := MakeRandomString(10)

    disk := NewOSVirtualHardDisk(
        hostcaching, disklabel, diskname, MediaLink, SourceImageName, OS)
    c.Check(disk.HostCaching, Equals, string(hostcaching))
    c.Check(disk.DiskLabel, Equals, disklabel)
    c.Check(disk.DiskName, Equals, diskname)
    c.Check(disk.MediaLink, Equals, MediaLink)
    c.Check(disk.SourceImageName, Equals, SourceImageName)
    c.Check(disk.OS, Equals, OS)
}

// Properties XML subtree for ListContainers Storage API call.
func (suite *xmlSuite) TestProperties(c *C) {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <Properties>
          <Last-Modified>date/time-value</Last-Modified>
          <Etag>etag-value</Etag>
          <LeaseStatus>lease-status-value</LeaseStatus>
          <LeaseState>lease-state-value</LeaseState>
          <LeaseDuration>lease-duration-value</LeaseDuration>
        </Properties>`
    observed := &Properties{}
    err := xml.Unmarshal([]byte(input), observed)
    c.Assert(err, IsNil)

    expected := &Properties{
        LastModified:  "date/time-value",
        ETag:          "etag-value",
        LeaseStatus:   "lease-status-value",
        LeaseState:    "lease-state-value",
        LeaseDuration: "lease-duration-value",
    }

    c.Assert(observed, DeepEquals, expected)
}

// Metadata XML subtree for ListContainers Storage API call.
func (suite *xmlSuite) TestMetadata(c *C) {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <Metadata>
          <metadata-name>metadata-value</metadata-name>
        </Metadata>`
    observed := &Metadata{}
    err := xml.Unmarshal([]byte(input), observed)
    c.Assert(err, IsNil)

    expected := &Metadata{
        Items: []MetadataItem{
            {
                XMLName: xml.Name{Local: "metadata-name"},
                Value:   "metadata-value",
            },
        },
    }

    c.Assert(observed, DeepEquals, expected)
}

// Container XML subtree for ListContainers Storage API call.
func (suite *xmlSuite) TestContainer(c *C) {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <Container>
          <Name>name-value</Name>
          <URL>url-value</URL>
          <Properties>
            <Last-Modified>date/time-value</Last-Modified>
            <Etag>etag-value</Etag>
            <LeaseStatus>lease-status-value</LeaseStatus>
            <LeaseState>lease-state-value</LeaseState>
            <LeaseDuration>lease-duration-value</LeaseDuration>
          </Properties>
          <Metadata>
            <metadata-name>metadata-value</metadata-name>
          </Metadata>
        </Container>`
    observed := &Container{}
    err := xml.Unmarshal([]byte(input), observed)
    c.Assert(err, IsNil)

    expected := &Container{
        XMLName: xml.Name{Local: "Container"},
        Name:    "name-value",
        URL:     "url-value",
        Properties: Properties{
            LastModified:  "date/time-value",
            ETag:          "etag-value",
            LeaseStatus:   "lease-status-value",
            LeaseState:    "lease-state-value",
            LeaseDuration: "lease-duration-value",
        },
        Metadata: Metadata{
            Items: []MetadataItem{
                {
                    XMLName: xml.Name{Local: "metadata-name"},
                    Value:   "metadata-value",
                },
            },
        },
    }

    c.Assert(observed, DeepEquals, expected)
}

// EnumerationResults XML tree for ListContainers Storage API call.
func (suite *xmlSuite) TestContainerEnumerationResults(c *C) {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <EnumerationResults AccountName="http://myaccount.blob.core.windows.net">
          <Prefix>prefix-value</Prefix>
          <Marker>marker-value</Marker>
          <MaxResults>max-results-value</MaxResults>
          <Containers>
            <Container>
              <Name>name-value</Name>
              <URL>url-value</URL>
              <Properties>
                <Last-Modified>date/time-value</Last-Modified>
                <Etag>etag-value</Etag>
                <LeaseStatus>lease-status-value</LeaseStatus>
                <LeaseState>lease-state-value</LeaseState>
                <LeaseDuration>lease-duration-value</LeaseDuration>
              </Properties>
              <Metadata>
                <metadata-name>metadata-value</metadata-name>
              </Metadata>
            </Container>
          </Containers>
          <NextMarker>next-marker-value</NextMarker>
        </EnumerationResults>`
    observed := &ContainerEnumerationResults{}
    err := observed.Deserialize([]byte(input))
    c.Assert(err, IsNil)

    expected := &ContainerEnumerationResults{
        XMLName:    xml.Name{Local: "EnumerationResults"},
        Prefix:     "prefix-value",
        Marker:     "marker-value",
        MaxResults: "max-results-value",
        Containers: []Container{
            {
                XMLName: xml.Name{Local: "Container"},
                Name:    "name-value",
                URL:     "url-value",
                Properties: Properties{
                    LastModified:  "date/time-value",
                    ETag:          "etag-value",
                    LeaseStatus:   "lease-status-value",
                    LeaseState:    "lease-state-value",
                    LeaseDuration: "lease-duration-value",
                },
                Metadata: Metadata{
                    Items: []MetadataItem{
                        {
                            XMLName: xml.Name{Local: "metadata-name"},
                            Value:   "metadata-value",
                        },
                    },
                },
            },
        },
        NextMarker: "next-marker-value",
    }

    c.Assert(observed, DeepEquals, expected)
    c.Assert(observed.Containers[0].Metadata.Items[0].Name(), Equals, "metadata-name")
}

func (suite *xmlSuite) TestHostedService(c *C) {
    input := `
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
          <Deployments>
            <Deployment xmlns="http://schemas.microsoft.com/windowsazure">
              <Name>name-of-deployment</Name>
            </Deployment>
          </Deployments>
	</HostedService>
    `
    expected := &HostedService{
        XMLNS: "http://schemas.microsoft.com/windowsazure",
        HostedServiceDescriptor: HostedServiceDescriptor{
            URL:              "hosted-service-url",
            ServiceName:      "hosted-service-name",
            Description:      "description",
            AffinityGroup:    "name-of-affinity-group",
            Location:         "location-of-service",
            Label:            "base-64-encoded-name-of-service",
            Status:           "current-status-of-service",
            DateCreated:      "creation-date-of-service",
            DateLastModified: "last-modification-date-of-service",
            ExtendedProperties: []ExtendedProperty{
                {
                    Name:  "name-of-property",
                    Value: "value-of-property",
                },
            }},
        Deployments: []Deployment{{
            XMLNS: "http://schemas.microsoft.com/windowsazure",
            Name:  "name-of-deployment",
        }},
    }

    observed := &HostedService{}
    err := observed.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(observed, DeepEquals, expected)
}

func makeHostedServiceDescriptorList(url string) string {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
          <HostedServices xmlns="http://schemas.microsoft.com/windowsazure">
            <HostedService>
              <Url>%s</Url>
              <ServiceName>hosted-service-name</ServiceName>
              <HostedServiceProperties>
                <Description>description</Description>
                <AffinityGroup>affinity-group</AffinityGroup>
                <Location>service-location</Location>
                <Label>label</Label>
                <Status>status</Status>
                <DateCreated>date-created</DateCreated>
                <DateLastModified>date-modified</DateLastModified>
                <ExtendedProperties>
                  <ExtendedProperty>
                    <Name>property-name</Name>
                    <Value>property-value</Value>
                  </ExtendedProperty>
                </ExtendedProperties>
              </HostedServiceProperties>
            </HostedService>
          </HostedServices>
    `
    return fmt.Sprintf(input, url)
}

func (suite *xmlSuite) TestHostedServiceDescriptorList(c *C) {
    input := makeHostedServiceDescriptorList("hosted-service-address")
    expected := &HostedServiceDescriptorList{
        XMLName: xml.Name{
            Space: "http://schemas.microsoft.com/windowsazure",
            Local: "HostedServices"},
        XMLNS: "http://schemas.microsoft.com/windowsazure",
        HostedServices: []HostedServiceDescriptor{
            {
                URL:              "hosted-service-address",
                ServiceName:      "hosted-service-name",
                Description:      "description",
                AffinityGroup:    "affinity-group",
                Location:         "service-location",
                Label:            "label",
                Status:           "status",
                DateCreated:      "date-created",
                DateLastModified: "date-modified",
                ExtendedProperties: []ExtendedProperty{
                    {
                        Name:  "property-name",
                        Value: "property-value",
                    },
                },
            },
        },
    }

    observed := &HostedServiceDescriptorList{}
    err := observed.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(observed, DeepEquals, expected)
}

func (suite *xmlSuite) TestHostedServiceDescriptorGetLabel(c *C) {
    serviceDesc := HostedServiceDescriptor{Label: ""}
    label := MakeRandomString(10)
    base64Label := base64.StdEncoding.EncodeToString([]byte(label))
    serviceDesc.Label = base64Label
    decodedLabel, err := serviceDesc.GetLabel()
    c.Assert(err, IsNil)
    c.Check(decodedLabel, DeepEquals, label)
}

// TestCreateStorageService demonstrates that CreateHostedService is a
// suitable container for the CreateHostedService XML tree that are required
// for the Create Hosted Service API call.
func (suite *xmlSuite) TestCreateHostedService(c *C) {
    // From http://msdn.microsoft.com/en-us/library/windowsazure/gg441304.aspx
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <CreateHostedService xmlns="http://schemas.microsoft.com/windowsazure">
          <ServiceName>service-name</ServiceName>
          <Label>base64-encoded-service-label</Label>
          <Description>description</Description>
          <Location>location</Location>
          <AffinityGroup>affinity-group</AffinityGroup>
          <ExtendedProperties>
            <ExtendedProperty>
              <Name>property-name</Name>
              <Value>property-value</Value>
            </ExtendedProperty>
          </ExtendedProperties>
        </CreateHostedService>
        `
    expected := &CreateHostedService{
        XMLNS:         XMLNS,
        ServiceName:   "service-name",
        Label:         "base64-encoded-service-label",
        Description:   "description",
        Location:      "location",
        AffinityGroup: "affinity-group",
        ExtendedProperties: []ExtendedProperty{
            {
                Name:  "property-name",
                Value: "property-value",
            },
        },
    }
    observed := &CreateHostedService{}
    err := observed.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(observed, DeepEquals, expected)
}

func (suite *xmlSuite) TestNewCreateHostedServiceWithLocation(c *C) {
    serviceName := "serviceName"
    label := "label"
    location := "location"
    createdHostedService := NewCreateHostedServiceWithLocation(serviceName, label, location)
    base64label := base64.StdEncoding.EncodeToString([]byte(label))
    c.Check(createdHostedService.ServiceName, DeepEquals, serviceName)
    c.Check(createdHostedService.Label, DeepEquals, base64label)
    c.Check(createdHostedService.Location, DeepEquals, location)
}

func (suite *xmlSuite) TestNewCreateStorageServiceInputWithLocation(c *C) {
    cssi := NewCreateStorageServiceInputWithLocation("name", "label", "location", "false")
    c.Check(cssi.XMLNS, Equals, XMLNS)
    c.Check(cssi.ServiceName, Equals, "name")
    c.Check(cssi.Label, Equals, base64.StdEncoding.EncodeToString([]byte("label")))
    c.Check(cssi.Location, Equals, "location")
    c.Check(cssi.GeoReplicationEnabled, Equals, "false")
}

func (*xmlSuite) TestAvailabilityResponse(c *C) {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <AvailabilityResponse xmlns="http://schemas.microsoft.com/windowsazure">
          <Result>name-availability</Result>
          <Reason>reason</Reason>
        </AvailabilityResponse>`
    expected := &AvailabilityResponse{
        XMLNS:  XMLNS,
        Result: "name-availability",
        Reason: "reason",
    }
    observed := &AvailabilityResponse{}
    err := observed.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(observed, DeepEquals, expected)
}

func makeUpdateHostedService(label, description string, property ExtendedProperty) string {
    template := dedent.Dedent(`
        <UpdateHostedService xmlns="http://schemas.microsoft.com/windowsazure">
          <Label>%s</Label>
          <Description>%s</Description>
          <ExtendedProperties>
            <ExtendedProperty>
              <Name>%s</Name>
              <Value>%s</Value>
            </ExtendedProperty>
          </ExtendedProperties>
        </UpdateHostedService>`)
    return fmt.Sprintf(template, label, description, property.Name, property.Value)
}

func (suite *xmlSuite) TestUpdateHostedService(c *C) {
    label := MakeRandomString(10)
    description := MakeRandomString(10)
    property := ExtendedProperty{
        Name:  "property-name",
        Value: "property-value",
    }
    expected := makeUpdateHostedService(label, description, property)
    input := UpdateHostedService{
        XMLNS:       XMLNS,
        Label:       label,
        Description: description,
        ExtendedProperties: []ExtendedProperty{
            property,
        },
    }

    observed, err := input.Serialize()
    c.Assert(err, IsNil)
    c.Assert(strings.TrimSpace(observed), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestNewUpdateHostedService(c *C) {
    label := MakeRandomString(10)
    description := MakeRandomString(10)
    properties := []ExtendedProperty{
        {
            Name:  MakeRandomString(10),
            Value: MakeRandomString(10),
        },
    }
    updateHostedService := NewUpdateHostedService(
        label, description, properties)
    c.Check(
        updateHostedService.Label, Equals,
        base64.StdEncoding.EncodeToString([]byte(label)))
    c.Check(updateHostedService.Description, Equals, description)
    c.Check(updateHostedService.ExtendedProperties, DeepEquals, properties)
}

func (suite *xmlSuite) TestBlockListSerialize(c *C) {
    blockList := &BlockList{
        XMLName: xml.Name{Local: "BlockList"},
    }
    blockList.Add(BlockListCommitted, "first-base64-encoded-block-id")
    blockList.Add(BlockListUncommitted, "second-base64-encoded-block-id")
    blockList.Add(BlockListLatest, "third-base64-encoded-block-id")
    observed, err := blockList.Serialize()
    c.Assert(err, IsNil)
    expected := dedent.Dedent(`
        <BlockList>
          <Committed>Zmlyc3QtYmFzZTY0LWVuY29kZWQtYmxvY2staWQ=</Committed>
          <Uncommitted>c2Vjb25kLWJhc2U2NC1lbmNvZGVkLWJsb2NrLWlk</Uncommitted>
          <Latest>dGhpcmQtYmFzZTY0LWVuY29kZWQtYmxvY2staWQ=</Latest>
        </BlockList>`)
    c.Assert(strings.TrimSpace(string(observed)), Equals, strings.TrimSpace(expected))
}

func (suite *xmlSuite) TestGetBlockListDeserialize(c *C) {
    input := `
        <?xml version="1.0" encoding="utf-8"?>
        <BlockList>
          <CommittedBlocks>
            <Block>
              <Name>BlockId001</Name>
              <Size>4194304</Size>
            </Block>
          </CommittedBlocks>
          <UncommittedBlocks>
            <Block>
              <Name>BlockId002</Name>
              <Size>1024</Size>
            </Block>
          </UncommittedBlocks>
        </BlockList>`
    observed := GetBlockList{}
    err := observed.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    expected := GetBlockList{
        XMLName: xml.Name{Local: "BlockList"},
        CommittedBlocks: []Block{
            {
                Name: "BlockId001",
                Size: "4194304"},
        },
        UncommittedBlocks: []Block{
            {
                Name: "BlockId002",
                Size: "1024"},
        },
    }
    c.Check(observed, DeepEquals, expected)
}

func makeOperationXML(operationType string) string {
    XMLTemplate := dedent.Dedent(`
        <%s xmlns="http://schemas.microsoft.com/windowsazure" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
          <OperationType>%s</OperationType>
        </%s>`)
    return fmt.Sprintf(XMLTemplate, operationType, operationType, operationType)
}

func (suite *managementAPISuite) TestStartRoleOperation(c *C) {
    expectedXML := makeOperationXML("StartRoleOperation")
    xml, err := marshalXML(startRoleOperation)
    c.Assert(err, IsNil)
    c.Check(strings.TrimSpace(string(xml)), Equals, strings.TrimSpace(expectedXML))
}

func (suite *managementAPISuite) TestRestartRoleOperation(c *C) {
    expectedXML := makeOperationXML("RestartRoleOperation")
    xml, err := marshalXML(restartRoleOperation)
    c.Assert(err, IsNil)
    c.Check(strings.TrimSpace(string(xml)), Equals, strings.TrimSpace(expectedXML))
}

func (suite *managementAPISuite) TestShutdownRoleOperation(c *C) {
    expectedXML := makeOperationXML("ShutdownRoleOperation")
    xml, err := marshalXML(shutdownRoleOperation)
    c.Assert(err, IsNil)
    c.Check(strings.TrimSpace(string(xml)), Equals, strings.TrimSpace(expectedXML))
}

// TestOSImageWRTAddOSImage demonstrates the OSImage is a suitable container
// for the OSImage XML trees that are required for the Add OS Image API call.
func (suite *xmlSuite) TestOSImageWRTAddOSImage(c *C) {
    // From http://msdn.microsoft.com/en-us/library/windowsazure/jj157192.aspx
    input := `
        <OSImage xmlns="http://schemas.microsoft.com/windowsazure">
           <Label>image-label</Label>
           <MediaLink>uri-of-the-containing-blob</MediaLink>
           <Name>image-name</Name>
           <OS>Linux|Windows</OS>
           <Eula>image-eula</Eula>
           <Description>image-description</Description>
           <ImageFamily>image-family</ImageFamily>
           <PublishedDate>published-date</PublishedDate>
           <IsPremium>true/false</IsPremium>
           <ShowInGui>true/false</ShowInGui>
           <PrivacyUri>http://www.example.com/privacypolicy.html</PrivacyUri>
           <IconUri>http://www.example.com/favicon.png</IconUri>
           <RecommendedVMSize>Small/Large/Medium/ExtraLarge</RecommendedVMSize>
           <SmallIconUri>http://www.example.com/smallfavicon.png</SmallIconUri>
           <Language>language-of-image</Language>
        </OSImage>
        `
    expected := &OSImage{
        Label:             "image-label",
        MediaLink:         "uri-of-the-containing-blob",
        Name:              "image-name",
        OS:                "Linux|Windows",
        EULA:              "image-eula",
        Description:       "image-description",
        ImageFamily:       "image-family",
        PublishedDate:     "published-date",
        IsPremium:         "true/false",
        ShowInGUI:         "true/false",
        PrivacyURI:        "http://www.example.com/privacypolicy.html",
        IconURI:           "http://www.example.com/favicon.png",
        RecommendedVMSize: "Small/Large/Medium/ExtraLarge",
        SmallIconURI:      "http://www.example.com/smallfavicon.png",
        Language:          "language-of-image",
    }

    osimage := &OSImage{}
    err := osimage.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(osimage, DeepEquals, expected)
}

// TestOSImageWRTListOSImages demonstrates that OSImage is a suitable
// container for the OSImage XML subtrees that are returned from the List OS
// Images API call.
func (suite *xmlSuite) TestOSImageWRTListOSImages(c *C) {
    // From http://msdn.microsoft.com/en-us/library/windowsazure/jj157191.aspx
    input := `
        <OSImage xmlns="http://schemas.microsoft.com/windowsazure">
          <AffinityGroup>name-of-the-affinity-group</AffinityGroup>
          <Category>category-of-the-image</Category>
          <Label>image-description</Label>
          <Location>geo-location-of-the-stored-image</Location>
          <LogicalSizeInGB>123.456</LogicalSizeInGB>
          <MediaLink>url-of-the-containing-blob</MediaLink>
          <Name>image-name</Name>
          <OS>operating-system-of-the-image</OS>
          <Eula>image-eula</Eula>
          <Description>image-description</Description>
          <ImageFamily>image-family</ImageFamily>
          <ShowInGui>true|false</ShowInGui>
          <PublishedDate>published-date</PublishedDate>
          <IsPremium>true|false</IsPremium>
          <PrivacyUri>uri-of-privacy-policy</PrivacyUri>
          <RecommendedVMSize>size-of-the-virtual-machine</RecommendedVMSize>
          <PublisherName>publisher-identifier</PublisherName>
          <PricingDetailLink>pricing-details</PricingDetailLink>
          <SmallIconUri>uri-of-icon</SmallIconUri>
          <Language>language-of-image</Language>
        </OSImage>
        `
    expected := &OSImage{
        AffinityGroup:     "name-of-the-affinity-group",
        Category:          "category-of-the-image",
        Label:             "image-description",
        Location:          "geo-location-of-the-stored-image",
        LogicalSizeInGB:   123.456,
        MediaLink:         "url-of-the-containing-blob",
        Name:              "image-name",
        OS:                "operating-system-of-the-image",
        EULA:              "image-eula",
        Description:       "image-description",
        ImageFamily:       "image-family",
        ShowInGUI:         "true|false",
        PublishedDate:     "published-date",
        IsPremium:         "true|false",
        PrivacyURI:        "uri-of-privacy-policy",
        RecommendedVMSize: "size-of-the-virtual-machine",
        PublisherName:     "publisher-identifier",
        PricingDetailLink: "pricing-details",
        SmallIconURI:      "uri-of-icon",
        Language:          "language-of-image",
    }

    osimage := &OSImage{}
    err := osimage.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(osimage, DeepEquals, expected)
}

// TestOSImageWRTUpdateOSImage demonstrates the OSImage is a suitable
// container for the OSImage XML trees that are required for the Update OS
// Image API call.
func (suite *xmlSuite) TestOSImageWRTUpdateOSImage(c *C) {
    // From http://msdn.microsoft.com/en-us/library/windowsazure/jj157198.aspx
    input := `
        <OSImage xmlns="http://schemas.microsoft.com/windowsazure">
          <Label>image-label</Label>
          <Eula>image-eula</Eula>
          <Description>Image-Description</Description>
          <ImageFamily>Image-Family</ImageFamily>
          <PublishedDate>published-date</PublishedDate>
          <IsPremium>true/false</IsPremium>
          <ShowInGui>true/false</ShowInGui>
          <PrivacyUri>http://www.example.com/privacypolicy.html</PrivacyUri>
          <IconUri>http://www.example.com/favicon.png</IconUri>
          <RecommendedVMSize>Small/Large/Medium/ExtraLarge</RecommendedVMSize>
          <SmallIconUri>http://www.example.com/smallfavicon.png</SmallIconUri>
          <Language>language-of-image</Language>
        </OSImage>
        `
    expected := &OSImage{
        Label:             "image-label",
        EULA:              "image-eula",
        Description:       "Image-Description",
        ImageFamily:       "Image-Family",
        PublishedDate:     "published-date",
        IsPremium:         "true/false",
        ShowInGUI:         "true/false",
        PrivacyURI:        "http://www.example.com/privacypolicy.html",
        IconURI:           "http://www.example.com/favicon.png",
        RecommendedVMSize: "Small/Large/Medium/ExtraLarge",
        SmallIconURI:      "http://www.example.com/smallfavicon.png",
        Language:          "language-of-image",
    }

    osimage := &OSImage{}
    err := osimage.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(osimage, DeepEquals, expected)
}

func (suite *xmlSuite) TestOSImageHasLocation(c *C) {
    image := &OSImage{
        Location: "East Asia;Southeast Asia;North Europe;West Europe;East US;West US",
    }

    var testValues = []struct {
        location       string
        expectedResult bool
    }{
        {"East Asia", true},
        {"West US", true},
        {"Unknown location", false},
    }
    for _, test := range testValues {
        c.Check(image.hasLocation(test.location), Equals, test.expectedResult)
    }
}

func (suite *xmlSuite) TestIsDailyBuild(c *C) {
    c.Check((&OSImage{Label: "Ubuntu Server 12.04.2 LTS DAILY"}).isDailyBuild(), Equals, true)
    c.Check((&OSImage{Label: "Ubuntu Server 12.04.2 LTS"}).isDailyBuild(), Equals, false)
    c.Check((&OSImage{Label: "Ubuntu Server 13.04"}).isDailyBuild(), Equals, false)
}

func (suite *xmlSuite) TestSortImages(c *C) {
    input := `
<?xml version="1.0"?>
<Images xmlns="http://schemas.microsoft.com/windowsazure" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
  <OSImage>
    <Label>Label 1</Label>
    <PublishedDate>2012-04-25T00:00:00Z</PublishedDate>
  </OSImage>
  <OSImage>
    <Label>Label 2</Label>
    <PublishedDate>2013-02-15T00:00:00Z</PublishedDate>
  </OSImage>
  <OSImage>
    <Label>Label 3</Label>
    <PublishedDate>2013-04-13T00:00:00Z</PublishedDate>
  </OSImage>
  <OSImage>
    <Label>Label 4</Label>
    <PublishedDate>2013-03-15T00:00:00Z</PublishedDate>
  </OSImage>
</Images>`

    images := &Images{}
    err := images.Deserialize([]byte(input))
    c.Assert(err, IsNil)

    sort.Sort(images)
    labels := []string{
        (*images).Images[0].Label,
        (*images).Images[1].Label,
        (*images).Images[2].Label,
        (*images).Images[3].Label,
    }
    c.Check(labels, DeepEquals, []string{"Label 3", "Label 4", "Label 2", "Label 1"})
}

func (suite *xmlSuite) TestGetLatestUbuntuImage(c *C) {
    // This is real-world XML input.
    input := `
<?xml version="1.0"?>
<Images xmlns="http://schemas.microsoft.com/windowsazure" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
  <OSImage>
    <Category>Canonical</Category>
    <Label>Ubuntu Server 12.04.2 LTS</Label>
    <Location>East Asia;Southeast Asia;North Europe;West Europe;East US;West US</Location>
    <LogicalSizeInGB>30</LogicalSizeInGB>
    <Name>b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-12_04_2-LTS-amd64-server-20130415-en-us-30GB</Name>
    <OS>Linux</OS>
    <ImageFamily>Ubuntu Server 12.04 LTS</ImageFamily>
    <PublishedDate>2013-04-15T00:00:00Z</PublishedDate>
    <IsPremium>false</IsPremium>
    <PublisherName>Canonical</PublisherName>
  </OSImage>
  <OSImage>
    <Category>Canonical</Category>
    <Label>Ubuntu Server 12.10</Label>
    <Location>East Asia;Southeast Asia;North Europe;West Europe;East US;West US</Location>
    <LogicalSizeInGB>30</LogicalSizeInGB>
    <Name>b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-12_10-amd64-server-20130414-en-us-30GB</Name>
    <OS>Linux</OS>
    <ImageFamily>Ubuntu 12.10</ImageFamily>
    <PublishedDate>2013-04-15T00:00:00Z</PublishedDate>
    <PublisherName>Canonical</PublisherName>
  </OSImage>
  <OSImage>
    <Category>Canonical</Category>
    <Label>Ubuntu Server 13.04 DAILY</Label>
    <Location>East Asia;Southeast Asia;North Europe;West Europe;East US;West US</Location>
    <LogicalSizeInGB>30</LogicalSizeInGB>
    <Name>fake-name__Ubuntu-13_04-amd64-server-20130423-en-us-30GB</Name>
    <OS>Linux</OS>
    <ImageFamily>Ubuntu Server 13.04</ImageFamily>
    <PublishedDate>2013-06-25T00:00:00Z</PublishedDate>
    <PublisherName>Canonical</PublisherName>
  </OSImage>
  <OSImage>
    <Category>Canonical</Category>
    <Label>Ubuntu Server 13.04</Label>
    <Location>East Asia;Southeast Asia;North Europe;West Europe;East US;West US</Location>
    <LogicalSizeInGB>30</LogicalSizeInGB>
    <Name>b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-13_04-amd64-server-20130423-en-us-30GB</Name>
    <OS>Linux</OS>
    <ImageFamily>Ubuntu Server 13.04</ImageFamily>
    <PublishedDate>2013-04-25T00:00:00Z</PublishedDate>
    <PublisherName>Canonical</PublisherName>
  </OSImage>
  <OSImage>
    <Category>Canonical</Category>
    <Label>Ubuntu Server 13.04 bogus publisher name</Label>
    <Location>East Asia;Southeast Asia;North Europe;West Europe;East US;West US</Location>
    <LogicalSizeInGB>30</LogicalSizeInGB>
    <Name>b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-13_04-amd64-server-20130423-en-us-30GB</Name>
    <OS>Linux</OS>
    <PublishedDate>2013-05-25T00:00:00Z</PublishedDate>
    <ImageFamily>Ubuntu Server 13.04</ImageFamily>
    <PublisherName>Bogus publisher name</PublisherName>
  </OSImage>
</Images>`
    images := &Images{}
    err := images.Deserialize([]byte(input))
    c.Assert(err, IsNil)

    var testValues = []struct {
        releaseName   string
        location      string
        expectedError error
        expectedLabel string
    }{
        {"13.04", "West US", nil, "Ubuntu Server 13.04"},
        {"12.04", "West US", nil, "Ubuntu Server 12.04.2 LTS"},
        {"bogus-name", "Unknown location", fmt.Errorf("No matching images found"), ""},
    }
    for _, test := range testValues {
        image, err := images.GetLatestUbuntuImage(test.releaseName, test.location)
        c.Check(err, DeepEquals, test.expectedError)
        if image != nil {
            c.Check(image.Label, Equals, test.expectedLabel)
        }
    }
}

func (suite *xmlSuite) TestOperation(c *C) {
    // From http://msdn.microsoft.com/en-us/library/windowsazure/ee460783.aspx
    input := `
        <?xml version="1.0" encoding="utf-8"?>
          <Operation xmlns="http://schemas.microsoft.com/windowsazure">
            <ID>request-id</ID>
            <Status>InProgress|Succeeded|Failed</Status>
            <!--Response includes HTTP status code only if the operation succeeded or failed -->
            <HttpStatusCode>200</HttpStatusCode>
            <!--Response includes additional error information only if the operation failed -->
            <Error>
              <Code>error-code</Code>
              <Message>error-message</Message>
            </Error>
          </Operation>
        `
    expected := &Operation{
        ID:             "request-id",
        Status:         "InProgress|Succeeded|Failed",
        HTTPStatusCode: 200,
        ErrorCode:      "error-code",
        ErrorMessage:   "error-message",
    }
    observed := &Operation{}
    err := observed.Deserialize([]byte(input))
    c.Assert(err, IsNil)
    c.Assert(observed, DeepEquals, expected)
}
