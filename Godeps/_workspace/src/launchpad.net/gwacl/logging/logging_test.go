// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package logging

import (
    . "launchpad.net/gocheck"
    "testing"
)

var originalLevel = level

func restoreLevel() {
    level = originalLevel
}

type testLogging struct{}

var _ = Suite(&testLogging{})

func (suite *testLogging) TestSetLevel(c *C) {
    defer restoreLevel()
    // The names of the logging constants are recognised arguments to
    // setLevel().
    level = -1
    setLevel("DEBUG")
    c.Check(level, Equals, DEBUG)
    setLevel("INFO")
    c.Check(level, Equals, INFO)
    setLevel("WARN")
    c.Check(level, Equals, WARN)
    setLevel("ERROR")
    c.Check(level, Equals, ERROR)
    // Unrecognised arguments are ignored.
    level = -1
    setLevel("FOOBAR")
    c.Check(level, Equals, -1)
    setLevel("123")
    c.Check(level, Equals, -1)
}

// Master loader for all tests.
func Test(t *testing.T) {
    TestingT(t)
}
