// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/base64"
    . "launchpad.net/gocheck"
)

type testVHDFooter struct{}

var _ = Suite(&testVHDFooter{})

func (s *testVHDFooter) TestValidDecode(c *C) {
    var err error
    decoded, err := base64.StdEncoding.DecodeString(VHD_FOOTER)
    c.Assert(err, IsNil)
    c.Assert(512, Equals, len(decoded))
}
