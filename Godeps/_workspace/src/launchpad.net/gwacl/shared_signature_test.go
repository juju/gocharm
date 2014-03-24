// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/base64"
    . "launchpad.net/gocheck"
    "time"
)

type sharedSignatureSuite struct{}

var _ = Suite(&sharedSignatureSuite{})

func (*sharedSignatureSuite) TestComposeSharedSignature(c *C) {
    params := &sharedSignatureParams{
        permission:       "r",
        signedExpiry:     "2015-02-12",
        path:             "/path/to/file",
        accountName:      "name",
        signedIdentifier: "identifier",
        signedVersion:    "2012-02-12",
        signedRessource:  "/the/ressource",
        accountKey:       base64.StdEncoding.EncodeToString([]byte("key")),
    }

    signature, err := params.composeSharedSignature()
    c.Assert(err, IsNil)
    c.Check(signature, Equals, "C/COJt8UagHJR2LBT1129bhDChtgfLGFqfZ0YQpBdF0=")
}

func (*sharedSignatureSuite) TestComposeAccessQueryValues(c *C) {
    params := &sharedSignatureParams{
        permission:       "r",
        signedExpiry:     "2015-02-12",
        path:             "/path/to/file",
        accountName:      "name",
        signedIdentifier: "identifier",
        signedVersion:    "2012-02-12",
        signedRessource:  "/the/ressource",
        accountKey:       base64.StdEncoding.EncodeToString([]byte("key")),
    }

    values, err := params.composeAccessQueryValues()
    c.Assert(err, IsNil)

    c.Check(values.Get("sv"), Equals, params.signedVersion)
    c.Check(values.Get("se"), Equals, params.signedExpiry)
    c.Check(values.Get("sr"), Equals, params.signedRessource)
    c.Check(values.Get("sp"), Equals, params.permission)
    c.Check(values.Get("sig"), Not(HasLen), 0)
}

func (*sharedSignatureSuite) TestGetReadBlobAccessValues(c *C) {
    container := "container"
    filename := "/path/to/file"
    accountName := "name"
    accountKey := base64.StdEncoding.EncodeToString([]byte("key"))
    expires, err := time.Parse("Monday, 02-Jan-06 15:04:05 MST", time.RFC850)
    c.Assert(err, IsNil)

    values, err := getReadBlobAccessValues(container, filename, accountName, accountKey, expires)
    c.Assert(err, IsNil)

    c.Check(values.Get("sv"), Equals, "2012-02-12")
    expiryDateString := values.Get("se")
    expectedExpiryDateString := expires.UTC().Format(time.RFC3339)
    c.Check(expiryDateString, Equals, expectedExpiryDateString)
    c.Check(values.Get("sr"), Equals, "b")
    c.Check(values.Get("sp"), Equals, "r")
    c.Check(values.Get("sig"), Equals, "HK7xmxiUY/vBNkaIWoJkIcv27g/+QFjwKVgO/I3yWmI=")
}
