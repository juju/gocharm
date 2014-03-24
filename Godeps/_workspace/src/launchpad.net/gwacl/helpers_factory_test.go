// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).
//
// Factories for various types of objects that tests need to create.

package gwacl

// This should be refactored at some point, it does not belong in here.
// Perhaps we can add it to gocheck, or start a testtools-like project.
const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyz"

// MakeRandomString returns an arbitrary string of alphanumerical characters.
// TODO: This isn't really a random string, more of a random identifier.
func MakeRandomString(length int) string {
    return string(MakeRandomByteSlice(length))
}

// MakeRandomString returns a slice of arbitrary bytes, all corresponding to
// alphanumerical characters in ASCII.
// TODO: This isn't really very random.  Good tests need zero and "high" values.
func MakeRandomByteSlice(length int) []byte {
    dest := make([]byte, length)
    for i := range dest {
        num := random.Intn(len(chars))
        randChar := chars[num]
        dest[i] = randChar
    }
    return dest
}

// MakeRandomBool returns an arbitrary bool value (true or false).
func MakeRandomBool() bool {
    v := random.Intn(2)
    if v == 0 {
        return false
    }
    return true
}

// MakeRandomPort returns a port number between 1 and 65535 inclusive.
func MakeRandomPort() uint16 {
    port := uint16(random.Intn(1 << 16))
    if port == 0 {
        return MakeRandomPort()
    }
    return port
}
