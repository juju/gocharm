// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
)

// pickOne returns a random choice of one of the characters in chars.
func pickOne(chars string) string {
    index := random.Intn(len(chars))
    return string(chars[index])
}

const (
    upperCaseLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    letters          = "abcdefghijklmnopqrstuvwxyz"
    digits           = "0123456789"
)

// makeRandomIdentifier creates an arbitrary identifier of the given length,
// consisting of only ASCII digits and lower-case ASCII letters.
// The identifier will start with the given prefix.  The prefix must be no
// longer than the specified length, or there'll be trouble.

func makeRandomIdentifier(prefix string, length int) string {
    // Only digits and lower-case ASCII letters are accepted.
    const (
        chars = letters + digits
    )

    if len(prefix) > length {
        panic(fmt.Errorf("prefix '%s' is more than the requested %d characters long", prefix, length))
    }

    if len(prefix)+5 > length {
        panic(fmt.Errorf(
            "prefix '%s' is too long; space is needed for at least 5 random characters, only %d given",
            prefix, length-len(prefix)))
    }

    if len(prefix) == 0 {
        // No prefix.  Still have to start with a letter, so pick one.
        prefix = pickOne(letters)
    }

    id := prefix
    for len(id) < length {
        id += pickOne(chars)
    }
    return id
}

const (
    // We don't know of any documentation on how long a hosted-service name can
    // be, but this is the maximum length that worked in experiments.
    HostedServiceNameMaxiumSize = 63
    // The number of random characters used when generating random Hosted
    // Service names.
    HostedServiceNameRandomChars = 10
    // The maximum length allowed for a Hosted Service name prefix (as passed
    // to MakeRandomHostedServiceName().)
    HostedServiceNameMaximumPrefixSize = HostedServiceNameMaxiumSize - HostedServiceNameRandomChars
)

// MakeRandomHostedServiceName generates a pseudo-random name for a hosted
// service, with the given prefix.
//
// The prefix must be as short as possible, be entirely in ASCII, start with
// a lower-case letter, and contain only lower-case letters and digits after
// that.
func MakeRandomHostedServiceName(prefix string) string {
    // We don't know of any documentation on long a hosted-service name can
    // be, but this is the maximum length that worked in experiments.
    size := len(prefix) + HostedServiceNameRandomChars
    if size > HostedServiceNameMaxiumSize {
        panic(fmt.Errorf("prefix '%s' is too long;  it can be at most %d characters", prefix, HostedServiceNameMaximumPrefixSize))
    }

    return makeRandomIdentifier(prefix, size)
}

// MakeRandomHostname generates a pseudo-random hostname for a virtual machine,
// with the given prefix.
//
// The prefix must be as short as possible, be entirely in ASCII, start with
// a lower-case letter, and contain only lower-case letters and digits after
// that.
func MakeRandomHostname(prefix string) string {
    // Azure documentation says the hostname can be between 1 and 64
    // letters long, but in practice we found it didn't work with anything
    // over 55 letters long.
    return makeRandomIdentifier(prefix, 55)
}

// MakeRandomDiskName generates a pseudo-random disk name for a virtual machine
// with the given prefix.
//
// The prefix must be as short as possible, be entirely in ASCII, start with
// a lower-case letter, and contain only lower-case letters and digits after
// that.
func MakeRandomDiskName(prefix string) string {
    // Azure documentation does not say what the maximum size of a disk name
    // is.  Testing indicate that 50 works.
    return makeRandomIdentifier(prefix, 50)
}

// MakeRandomRoleName generates a pseudo-random role name for a virtual machine
// with the given prefix.
//
// The prefix must be as short as possible, be entirely in ASCII, start with
// a lower-case letter, and contain only lower-case letters and digits after
// that.
func MakeRandomRoleName(prefix string) string {
    // Azure documentation does not say what the maximum size of a role name
    // is.  Testing indicate that 50 works.
    return makeRandomIdentifier(prefix, 50)
}

// MakeRandomVirtualNetworkName generates a pseudo-random name for a virtual
// network with the given prefix.
//
// The prefix must be as short as possible, be entirely in ASCII, start with
// a lower-case letter, and contain only lower-case letters and digits after
// that.
func MakeRandomVirtualNetworkName(prefix string) string {
    return makeRandomIdentifier(prefix, 20)
}

const (
    // Valid passwords must be 6-72 characters long.
    passwordSize = 50
)

// MakeRandomPassword generates a pseudo-random password for a Linux Virtual
// Machine.
func MakeRandomPassword() string {
    const chars = letters + digits + upperCaseLetters

    upperCaseLetter := pickOne(upperCaseLetters)
    letter := pickOne(letters)
    digit := pickOne(digits)
    // Make sure the password has at least one letter, one upper-case letter
    // and a digit to meet Azure's password complexity requirements.
    password := letter + upperCaseLetter + digit
    for len(password) < passwordSize {
        password += pickOne(chars)
    }
    return password
}
