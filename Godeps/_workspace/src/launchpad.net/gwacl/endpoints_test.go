// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    . "launchpad.net/gocheck"
    "net/url"
)

type endpointsSuite struct{}

var _ = Suite(&endpointsSuite{})

func (*endpointsSuite) TestGetEndpointReturnsEndpointsForKnownRegions(c *C) {
    internationalLocations := []string{
        "West Europe",
        "East Asia",
        "East US 2",
        "Southeast Asia",
        "East US",
        "Central US",
        "West US",
        "North Europe",
    }
    internationalEndpoint := APIEndpoint("https://core.windows.net/")

    for _, location := range internationalLocations {
        c.Check(GetEndpoint(location), Equals, internationalEndpoint)
    }

    // The mainland-China locations have a different endpoint.
    // (Actually the East Asia data centre is said to be in Hong Kong, but it
    // acts as international).
    mainlandChinaLocations := []string{
        "China East",
        "China North",
    }
    mainlandChinaEndpoint := APIEndpoint("https://core.chinacloudapi.cn/")
    for _, location := range mainlandChinaLocations {
        c.Check(GetEndpoint(location), Equals, mainlandChinaEndpoint)
    }
}

func (*endpointsSuite) TestGetEndpointMakesGoodGuessesForUknownRegions(c *C) {
    c.Check(
        GetEndpoint("South San Marino Highlands"),
        Equals,
        GetEndpoint("West US"))
    c.Check(
        GetEndpoint("Central China West"),
        Equals,
        GetEndpoint("China East"))
}

func (*endpointsSuite) TestPrefixHostPrefixesSubdomain(c *C) {
    c.Check(
        prefixHost("foo", "http://example.com"),
        Equals,
        "http://foo.example.com")
}

func (*endpointsSuite) TestPrefixHostPreservesOtherURLComponents(c *C) {
    c.Check(
        prefixHost("foo", "http://example.com/"),
        Equals,
        "http://foo.example.com/")
    c.Check(
        prefixHost("foo", "nntp://example.com"),
        Equals,
        "nntp://foo.example.com")
    c.Check(
        prefixHost("foo", "http://user@example.com"),
        Equals,
        "http://user@foo.example.com")
    c.Check(
        prefixHost("foo", "http://example.com:999"),
        Equals,
        "http://foo.example.com:999")
    c.Check(
        prefixHost("foo", "http://example.com/path"),
        Equals,
        "http://foo.example.com/path")
}

func (*endpointsSuite) TestPrefixHostEscapes(c *C) {
    host := "5%=1/20?"
    c.Check(
        prefixHost(host, "http://example.com"),
        Equals,
        fmt.Sprintf("http://%s.example.com", url.QueryEscape(host)))
}

func (*endpointsSuite) TestManagementAPICombinesWithGetEndpoint(c *C) {
    c.Check(
        GetEndpoint("West US").ManagementAPI(),
        Equals,
        "https://management.core.windows.net/")
    c.Check(
        GetEndpoint("China East").ManagementAPI(),
        Equals,
        "https://management.core.chinacloudapi.cn/")
}

func (*endpointsSuite) TestBlobStorageAPIIncludesAccountName(c *C) {
    c.Check(
        APIEndpoint("http://example.com").BlobStorageAPI("myaccount"),
        Equals,
        "http://myaccount.blob.example.com")
}

func (*endpointsSuite) TestBlobStorageAPICombinesWithGetEndpoint(c *C) {
    c.Check(
        GetEndpoint("West US").BlobStorageAPI("account"),
        Equals,
        "https://account.blob.core.windows.net/")
    c.Check(
        GetEndpoint("China East").BlobStorageAPI("account"),
        Equals,
        "https://account.blob.core.chinacloudapi.cn/")
}
