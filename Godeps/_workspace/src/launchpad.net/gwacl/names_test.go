// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    . "launchpad.net/gocheck"
    "strings"
)

type namesSuite struct{}

var _ = Suite(&namesSuite{})

func (*namesSuite) TestPickOneReturnsOneCharacter(c *C) {
    c.Check(len(pickOne("abcd")), Equals, 1)
}

func (*namesSuite) TestMakeRandomIdentifierObeysLength(c *C) {
    length := 6 + random.Intn(50)
    c.Check(len(makeRandomIdentifier("x", length)), Equals, length)
}

// makeRandomIdentifier ensures that there are at least 5 random characters in
// an identifier.
func (*namesSuite) TestMakeRandomIdentifierEnsuresSomeRandomness(c *C) {
    c.Check(makeRandomIdentifier("1234-", 10), Matches, "1234-[a-z0-9]{5}")
    c.Check(
        func() { makeRandomIdentifier("12345-", 10) }, PanicMatches,
        "prefix '12345-' is too long; space is needed for at least 5 random characters, only 4 given")
}

func (*namesSuite) TestMakeRandomIdentifierRandomizes(c *C) {
    // There is a minute chance that this will start failing just because
    // the randomizer repeats a pattern of results.  If so, seed it.
    c.Check(
        makeRandomIdentifier("x", 100),
        Not(Equals),
        makeRandomIdentifier("x", 100))
}

func (*namesSuite) TestMakeRandomIdentifierPicksDifferentCharacters(c *C) {
    // There is a minute chance that this will start failing just because
    // the randomizer repeats a pattern of results.  If so, seed it.
    chars := make(map[rune]bool)
    for _, chr := range makeRandomIdentifier("", 100) {
        chars[chr] = true
    }
    c.Check(len(chars), Not(Equals), 1)
}

func (*namesSuite) TestMakeRandomIdentifierUsesPrefix(c *C) {
    c.Check(makeRandomIdentifier("prefix", 11), Matches, "prefix.*")
}

func (*namesSuite) TestMakeRandomIdentifierUsesOnlyAcceptedCharacters(c *C) {
    c.Check(makeRandomIdentifier("", 100), Matches, "[0-9a-z]*")
}

func (*namesSuite) TestMakeRandomIdentifierAcceptsEmptyPrefix(c *C) {
    // In particular, the first character must still be a letter.
    c.Check(makeRandomIdentifier("", 5), Matches, "[a-z].*")
}

func (*namesSuite) TestMakeRandomDiskName(c *C) {
    c.Check(MakeRandomDiskName(""), Not(HasLen), 0)
}

func (*namesSuite) TestMakeRandomRoleName(c *C) {
    c.Check(MakeRandomRoleName(""), Not(HasLen), 0)
}

func (*namesSuite) TestMakeRandomVirtualNetworkName(c *C) {
    c.Check(MakeRandomVirtualNetworkName(""), Not(HasLen), 0)
}

func (*namesSuite) TestMakeRandomHostedServiceName(c *C) {
    c.Check(MakeRandomHostedServiceName(""), Not(HasLen), 0)
}

func (*namesSuite) TestMakeRandomHostedUsesALimitedNumberOfRandomChars(c *C) {
    prefix := "prefix"
    expectedSize := len(prefix) + HostedServiceNameRandomChars
    c.Check(MakeRandomHostedServiceName(prefix), HasLen, expectedSize)
}

func (*namesSuite) TestMakeRandomHostedRejectsLongPrefix(c *C) {
    tooLongPrefix := makeRandomIdentifier("", HostedServiceNameMaximumPrefixSize+1)
    c.Check(
        func() { MakeRandomHostedServiceName(tooLongPrefix) }, PanicMatches,
        ".*is too long.*")
}

func (*namesSuite) TestMakeRandomHostedAcceptsLongestPrefix(c *C) {
    prefix := makeRandomIdentifier("", HostedServiceNameMaximumPrefixSize)
    c.Check(MakeRandomHostedServiceName(prefix), HasLen, HostedServiceNameMaxiumSize)
}

func assertIsAzureValidPassword(c *C, password string) {
    c.Check(MakeRandomPassword(), HasLen, passwordSize)
    if !strings.ContainsAny(password, upperCaseLetters) {
        c.Errorf("Password %v does not contain a single upper-case letter!", password)
    }
    if !strings.ContainsAny(password, letters) {
        c.Errorf("Password %v does not contain a single lower-case letter!", password)
    }
    if !strings.ContainsAny(password, digits) {
        c.Errorf("Password %v does not contain a single digit!", password)
    }
}

func (*namesSuite) TestMakeRandomPassword(c *C) {
    for index := 0; index < 100; index += 1 {
        password := MakeRandomPassword()
        assertIsAzureValidPassword(c, password)
    }
}
