// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    "net/url"
    "strings"
)

// APIEndpoint describes the base URL for accesing Windows Azure's APIs.
//
// Azure will have subdomains on this URL's domain, such as blob.<domain> for
// storage, with further sub-domains for storage accounts; management.<domain>
// for the management API; and possibly more such as queue.<domain>,
// table.<domain>.  APIEndpoint defines methods to obtain these URLs.
type APIEndpoint string

// GetEndpoint returns the API endpoint for the given location.  This is
// hard-coded, so some guesswork may be involved.
func GetEndpoint(location string) APIEndpoint {
    if strings.Contains(location, "China") {
        // Mainland China is a special case.  It has its own endpoint.
        return "https://core.chinacloudapi.cn/"
    }

    // The rest of the world shares a single endpoint.
    return "https://core.windows.net/"
}

// prefixHost prefixes the hostname part of a URL with a subdomain.  For
// example, prefixHost("foo", "http://example.com") becomes
// "http://foo.example.com".
//
// The URL must be well-formed, and contain a hostname.
func prefixHost(host, originalURL string) string {
    parsedURL, err := url.Parse(originalURL)
    if err != nil {
        panic(fmt.Errorf("failed to parse URL %s - %v", originalURL, err))
    }
    if parsedURL.Host == "" {
        panic(fmt.Errorf("no hostname in URL '%s'", originalURL))
    }
    // Escape manually.  Strangely, turning a url.URL into a string does not
    // do this for you.
    parsedURL.Host = url.QueryEscape(host) + "." + parsedURL.Host
    return parsedURL.String()
}

// ManagementAPI returns the URL for the endpoint's management API.
func (endpoint APIEndpoint) ManagementAPI() string {
    return prefixHost("management", string(endpoint))
}

// BlobStorageAPI returns a URL for the endpoint's blob storage API, for
// requests on the given account.
func (endpoint APIEndpoint) BlobStorageAPI(account string) string {
    return prefixHost(account, prefixHost("blob", string(endpoint)))
}
