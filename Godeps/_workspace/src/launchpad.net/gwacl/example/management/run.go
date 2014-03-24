// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

/*
This is an example on how the Azure Go library can be used to interact with
the Windows Azure Service.
Note that this is a provided only as an example and that real code should
probably do something more sensible with errors than ignoring them or panicking.
*/
package main

import (
    "encoding/base64"
    "flag"
    "fmt"
    "launchpad.net/gwacl"
    . "launchpad.net/gwacl/logging"
    "math/rand"
    "os"
    "time"
)

var certFile string
var subscriptionID string
var pause bool
var location string

func getParams() error {
    flag.StringVar(&certFile, "cert", "", "Name of your management certificate file (in PEM format).")
    flag.StringVar(&subscriptionID, "subscriptionid", "", "Your Azure subscription ID.")
    flag.BoolVar(&pause, "pause", false, "Wait for user input after the VM is brought up (useful for further testing)")
    flag.StringVar(&location, "location", "North Europe", "Azure cloud location, e.g. 'West US' or 'China East'")

    flag.Parse()

    if certFile == "" {
        return fmt.Errorf("No .pem certificate specified.  Use the -cert option.")
    }
    if subscriptionID == "" {
        return fmt.Errorf("No subscription ID specified.  Use the -subscriptionid option.")
    }
    return nil
}

func checkError(err error) {
    if err != nil {
        panic(err)
    }
}

// makeRandomIdentifier creates an arbitrary identifier of the given length,
// consisting of only ASCII digits and lower-case ASCII letters.
// The identifier will start with the given prefix.  The prefix must be no
// longer than the specified length, or there'll be trouble.
func makeRandomIdentifier(prefix string, length int) string {
    // Only digits and lower-case ASCII letters are accepted.
    const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

    if len(prefix) > length {
        panic(fmt.Errorf("prefix '%s' is more than the requested %d characters long", prefix, length))
    }

    id := prefix
    for len(id) < length {
        id += string(chars[rand.Intn(len(chars))])
    }
    return id
}

func main() {
    rand.Seed(int64(time.Now().Nanosecond()))

    err := getParams()
    if err != nil {
        Info(err)
        os.Exit(1)
    }

    api, err := gwacl.NewManagementAPI(subscriptionID, certFile, location)
    checkError(err)

    ExerciseHostedServicesAPI(api)

    Info("All done.")
}

func ExerciseHostedServicesAPI(api *gwacl.ManagementAPI) {
    var err error
    location := "West US"
    release := "13.04"

    affinityGroupName := gwacl.MakeRandomHostname("affinitygroup")
    Info("Creating an affinity group...")
    cag := gwacl.NewCreateAffinityGroup(affinityGroupName, "affinity-label", "affinity-description", location)
    err = api.CreateAffinityGroup(&gwacl.CreateAffinityGroupRequest{
        CreateAffinityGroup: cag})
    checkError(err)
    Infof("Created affinity group %s\n", affinityGroupName)

    defer func() {
        Infof("Deleting affinity group %s\n", affinityGroupName)
        err := api.DeleteAffinityGroup(&gwacl.DeleteAffinityGroupRequest{
            Name: affinityGroupName})
        checkError(err)
        Infof("Done deleting affinity group %s\n", affinityGroupName)
    }()

    virtualNetworkName := gwacl.MakeRandomVirtualNetworkName("virtual-net-")
    Infof("Creating virtual network %s...\n", virtualNetworkName)
    virtualNetwork := gwacl.VirtualNetworkSite{
        Name:          virtualNetworkName,
        AffinityGroup: affinityGroupName,
        AddressSpacePrefixes: []string{
            "10.0.0.0/8",
        },
    }
    err = api.AddVirtualNetworkSite(&virtualNetwork)
    checkError(err)
    Info("Done creating virtual network")

    defer func() {
        Infof("Deleting virtual network %s...\n", virtualNetworkName)
        err := api.RemoveVirtualNetworkSite(virtualNetworkName)
        checkError(err)
        Infof("Done deleting virtual network %s\n", virtualNetworkName)
    }()

    networkConfig, err := api.GetNetworkConfiguration()
    checkError(err)
    if networkConfig == nil {
        Info("No network configuration is set")
    } else {
        xml, err := networkConfig.Serialize()
        checkError(err)
        Info(xml)
    }

    Infof("Getting OS Image for release '%s' and location '%s'...\n", release, location)
    images, err := api.ListOSImages()
    checkError(err)
    image, err := images.GetLatestUbuntuImage(release, location)
    checkError(err)
    sourceImageName := image.Name
    Infof("Got image named '%s'\n", sourceImageName)
    Info("Done getting OS Image\n")

    hostServiceName := gwacl.MakeRandomHostedServiceName("gwacl")
    Infof("Creating a hosted service: '%s'...\n", hostServiceName)
    createHostedService := gwacl.NewCreateHostedServiceWithLocation(hostServiceName, "testLabel", location)
    createHostedService.AffinityGroup = affinityGroupName
    err = api.AddHostedService(createHostedService)
    checkError(err)
    Info("Done creating a hosted service\n")

    defer func() {
        Info("Destroying hosted service...")
        // FIXME: Check error
        api.DestroyHostedService(&gwacl.DestroyHostedServiceRequest{
            ServiceName: hostServiceName})
        Info("Done destroying hosted service\n")
    }()

    Info("Listing hosted services...")
    hostedServices, err := api.ListHostedServices()
    checkError(err)
    Infof("Got %d hosted service(s)\n", len(hostedServices))
    if len(hostedServices) > 0 {
        hostedService := hostedServices[0]
        detailedHostedService, err := api.GetHostedServiceProperties(hostedService.ServiceName, true)
        checkError(err)
        Infof("Hosted service '%s' contains %d deployments\n", hostedService.ServiceName, len(detailedHostedService.Deployments))
        // Do the same again with ListAllDeployments.
        deployments, err := api.ListAllDeployments(&gwacl.ListAllDeploymentsRequest{ServiceName: hostedService.ServiceName})
        checkError(err)
        if len(deployments) != len(detailedHostedService.Deployments) {
            Errorf(
                "Mismatch in reported deployments: %d != %d",
                len(deployments), len(detailedHostedService.Deployments))
        }
    }
    Info("Done listing hosted services\n")

    Info("Adding VM deployment...")
    hostname := gwacl.MakeRandomHostname("gwaclhost")
    // Random passwords are no use to man nor beast here if you want to
    // test with your instance, so we'll use a fixed one.  It's not really a
    // security hazard in such a short-lived private instance.
    password := "Ubuntu123"
    username := "ubuntu"
    vhdName := gwacl.MakeRandomDiskName("gwacldisk")
    userdata := base64.StdEncoding.EncodeToString([]byte("TEST_USER_DATA"))
    linuxConfigurationSet := gwacl.NewLinuxProvisioningConfigurationSet(
        hostname, username, password, userdata, "false")
    inputendpoint := gwacl.InputEndpoint{LocalPort: 22, Name: "sshport", Port: 22, Protocol: "TCP"}
    networkConfigurationSet := gwacl.NewNetworkConfigurationSet([]gwacl.InputEndpoint{inputendpoint}, nil)

    storageAccount := makeRandomIdentifier("gwacl", 24)
    storageLabel := makeRandomIdentifier("gwacl", 64)
    Infof("Requesting storage account with name '%s' and label '%s'...\n", storageAccount, storageLabel)
    cssi := gwacl.NewCreateStorageServiceInputWithLocation(storageAccount, storageLabel, location, "false")
    err = api.AddStorageAccount(cssi)
    checkError(err)
    Info("Done requesting storage account\n")

    defer func() {
        Infof("Deleting storage account %s...\n", storageAccount)
        // FIXME: Check error
        api.DeleteStorageAccount(storageAccount)
        Info("Done deleting storage account\n")
    }()

    mediaLink := gwacl.CreateVirtualHardDiskMediaLink(storageAccount, fmt.Sprintf("vhds/%s.vhd", vhdName))
    diskName := makeRandomIdentifier("gwacldisk", 16)
    diskLabel := makeRandomIdentifier("gwacl", 64)
    vhd := gwacl.NewOSVirtualHardDisk("", diskLabel, diskName, mediaLink, sourceImageName, "Linux")
    roleName := gwacl.MakeRandomRoleName("gwaclrole")
    role := gwacl.NewRole("ExtraSmall", roleName,
        []gwacl.ConfigurationSet{*linuxConfigurationSet, *networkConfigurationSet},
        []gwacl.OSVirtualHardDisk{*vhd})
    machineName := makeRandomIdentifier("gwaclmachine", 20)
    deployment := gwacl.NewDeploymentForCreateVMDeployment(
        machineName, "Production", machineName, []gwacl.Role{*role}, virtualNetworkName)
    err = api.AddDeployment(deployment, hostServiceName)
    checkError(err)
    Info("Done adding VM deployment\n")

    Info("Starting VM...")
    err = api.StartRole(&gwacl.StartRoleRequest{hostServiceName, deployment.Name, role.RoleName})
    checkError(err)
    Info("Done starting VM\n")

    Info("Listing VM...")
    instances, err := api.ListInstances(&gwacl.ListInstancesRequest{hostServiceName})
    checkError(err)
    Infof("Got %d instance(s)\n", len(instances))
    Info("Done listing VM\n")

    Info("Getting deployment info...")
    request := &gwacl.GetDeploymentRequest{ServiceName: hostServiceName, DeploymentName: machineName}
    deploy, err := api.GetDeployment(request)
    checkError(err)
    fqdn, err := deploy.GetFQDN()
    checkError(err)
    Info("Got deployment info\n")

    Info("Adding role input endpoint...")
    endpoint := gwacl.InputEndpoint{
        Name:      gwacl.MakeRandomHostname("endpoint-"),
        Port:      1080,
        LocalPort: 80,
        Protocol:  "TCP",
    }
    err = api.AddRoleEndpoints(&gwacl.AddRoleEndpointsRequest{
        ServiceName:    hostServiceName,
        DeploymentName: deployment.Name,
        RoleName:       role.RoleName,
        InputEndpoints: []gwacl.InputEndpoint{endpoint},
    })
    checkError(err)
    Info("Added role input endpoint\n")

    defer func() {
        Info("Removing role input endpoint...")
        err := api.RemoveRoleEndpoints(&gwacl.RemoveRoleEndpointsRequest{
            ServiceName:    hostServiceName,
            DeploymentName: deployment.Name,
            RoleName:       role.RoleName,
            InputEndpoints: []gwacl.InputEndpoint{endpoint},
        })
        checkError(err)
        Info("Removed role input endpoint\n")
    }()

    if pause {
        var wait string
        fmt.Println("host:", fqdn)
        fmt.Println("username:", username)
        fmt.Println("password:", password)
        fmt.Println("")
        fmt.Println("Pausing so you can play with the newly-created VM")
        fmt.Println("To clear up, type something and press enter to continue")
        fmt.Scan(&wait)
    }
}
