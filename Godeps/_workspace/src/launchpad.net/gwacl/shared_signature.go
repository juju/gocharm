// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    "net/url"
    "time"
)

// sharedSignatureParams is a object which encapsulate all the parameters
// required to delegate access to a Windows Azure object using the
// "Shared Access Signature" mechanism.
type sharedSignatureParams struct {
    permission       string
    signedStart      string
    signedExpiry     string
    path             string
    accountName      string
    signedIdentifier string
    signedVersion    string
    signedRessource  string
    accountKey       string
}

// composeSharedSignature composes the "Shared Access Signature" as described
// in the paragraph "Constructing the Signature String" in
// http://msdn.microsoft.com/en-us/library/windowsazure/dn140255.aspx
func (params *sharedSignatureParams) composeSharedSignature() (string, error) {
    // Compose the string to sign.
    canonicalizedResource := fmt.Sprintf("/%s%s", params.accountName, params.path)
    stringToSign := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
        params.permission, params.signedStart, params.signedExpiry, canonicalizedResource, params.signedIdentifier, params.signedVersion)
    // Create the signature.
    signature, err := sign(params.accountKey, stringToSign)
    if err != nil {
        return "", err
    }
    return signature, nil
}

// composeAccessQueryValues returns the values that correspond to the query
// string used to build a Shared Access Signature URI as described in
// http://msdn.microsoft.com/en-us/library/windowsazure/dn140255.aspx
func (params *sharedSignatureParams) composeAccessQueryValues() (url.Values, error) {
    signature, err := params.composeSharedSignature()
    if err != nil {
        return nil, err
    }
    // Compose the "Shared Access Signature" query string.
    values := url.Values{}
    values.Set("sv", params.signedVersion)
    values.Set("se", params.signedExpiry)
    values.Set("sr", params.signedRessource)
    values.Set("sp", params.permission)
    values.Set("sig", signature)
    return values, nil
}

// getReadBlobAccessValues returns the values that correspond to the query
// string used to build a Shared Access Signature URI.  The signature grants
// read access to the given blob.
func getReadBlobAccessValues(container, filename, accountName, accountKey string, expires time.Time) (url.Values, error) {
    expiryDateString := expires.UTC().Format(time.RFC3339)

    path := fmt.Sprintf("/%s/%s", container, filename)
    signatureParams := &sharedSignatureParams{
        permission:       "r",
        signedExpiry:     expiryDateString,
        signedStart:      "",
        path:             path,
        accountName:      accountName,
        signedIdentifier: "",
        signedVersion:    "2012-02-12",
        signedRessource:  "b",
        accountKey:       accountKey,
    }
    return signatureParams.composeAccessQueryValues()
}
