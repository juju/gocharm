// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package dedent

import (
    . "launchpad.net/gocheck"
    "testing"
)

type dedentSuite struct{}

var _ = Suite(&dedentSuite{})

// Dedent() does nothing with the empty string.
func (suite *dedentSuite) TestEmptyString(c *C) {
    input := ""
    expected := input
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() does nothing to a single line without an indent.
func (suite *dedentSuite) TestSingleLine(c *C) {
    input := "This is a single line."
    expected := input
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() removes all leading whitespace from single lines.
func (suite *dedentSuite) TestSingleLineWithIndent(c *C) {
    input := "  This is a single line."
    expected := "This is a single line."
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() does nothing when none of the lines are indented.
func (suite *dedentSuite) TestLines(c *C) {
    input := "One\nTwo\n"
    expected := input
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() does nothing when *any* line is not indented.
func (suite *dedentSuite) TestLinesWithSomeIndents(c *C) {
    input := "One\n  Two\n"
    expected := input
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() removes the common leading indent from each line.
func (suite *dedentSuite) TestLinesWithIndents(c *C) {
    input := " One\n  Two\n"
    expected := "One\n Two\n"
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() ignores all-whitespace lines for the purposes of margin
// calculation. However, the margin *is* trimmed from these lines, if they
// begin with it.
func (suite *dedentSuite) TestLinesWithEmptyLine(c *C) {
    input := "  One\n    \n  Three\n"
    expected := "One\n  \nThree\n"
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() ignores blank lines for the purposes of margin calculation,
// including the first line.
func (suite *dedentSuite) TestLinesWithEmptyFirstLine(c *C) {
    input := "\n  Two\n  Three\n"
    expected := "\nTwo\nThree\n"
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Dedent() treats spaces and tabs as completely different; no number of
// spaces is equivalent to a tab.
func (suite *dedentSuite) TestLinesWithTabsAndSpaces(c *C) {
    input := "\tOne\n        Two\n"
    expected := input
    observed := Dedent(input)
    c.Check(observed, Equals, expected)
}

// Master loader for all tests.
func Test(t *testing.T) {
    TestingT(t)
}
