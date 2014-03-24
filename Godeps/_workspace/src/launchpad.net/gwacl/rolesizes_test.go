// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    . "launchpad.net/gocheck"
)

type rolesizeSuite struct{}

var _ = Suite(&rolesizeSuite{})

func (suite *rolesizeSuite) TestMapIsCreated(c *C) {
    c.Check(RoleNameMap, HasLen, len(RoleSizes))
}
