// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

package goose

import (
	. "launchpad.net/gocheck"
)

type VersionTestSuite struct {
}

var _ = Suite(&VersionTestSuite{})

func (s *VersionTestSuite) TestStringMatches(c *C) {
	c.Assert(Version, Equals, VersionNumber.String())
}
