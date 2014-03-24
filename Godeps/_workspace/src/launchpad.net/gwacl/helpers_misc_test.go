// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "encoding/base64"
    "fmt"
    "io"
)

// b64 is shorthand for base64-encoding a string.
func b64(s string) string {
    return base64.StdEncoding.EncodeToString([]byte(s))
}

// A Reader and ReadCloser that EOFs immediately.
var Empty io.ReadCloser = makeResponseBody("")

// BoolToString represents a boolean value as a string ("true" or "false").
func BoolToString(v bool) string {
    return fmt.Sprintf("%t", v)
}

// StringToBool parses a string containing a boolean (case-insensitive).
func StringToBool(v string) (b bool) {
    items, err := fmt.Sscanf(v, "%t", &b)
    if err != nil || items != 1 {
        panic(fmt.Errorf("can't convert '%s' to a bool", v))
    }
    return
}
