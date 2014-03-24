// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    "io"
    "io/ioutil"
    "net/url"
)

// checkPathComponents checks that none of the passed components contains any
// special characters, where special means "needs percent-encoding in a URI",
// does not contain any forward slashes, and is not the string "..".
func checkPathComponents(components ...string) {
    for _, component := range components {
        if component != url.QueryEscape(component) {
            panic(fmt.Errorf("'%s' contains URI special characters", component))
        }
        if component == ".." {
            panic(fmt.Errorf("'..' is not allowed"))
        }
    }
}

// readAndClose reads and closes the given ReadCloser.
//
// Trying to read from a nil simply returns nil, no error.
func readAndClose(stream io.ReadCloser) ([]byte, error) {
    if stream == nil {
        return nil, nil
    }
    defer stream.Close()
    return ioutil.ReadAll(stream)
}

// addURLQueryParams adds query parameters to a URL (and escapes as needed).
// Parameters are URL, [key, value, [key, value, [...]]].
// The original URL must be correct, i.e. it should parse without error.
func addURLQueryParams(originalURL string, params ...string) string {
    if len(params)%2 != 0 {
        panic(fmt.Errorf("got %d parameter argument(s), instead of matched key/value pairs", len(params)))
    }
    parsedURL, err := url.Parse(originalURL)
    if err != nil {
        panic(err)
    }
    query := parsedURL.Query()

    for index := 0; index < len(params); index += 2 {
        key := params[index]
        value := params[index+1]
        query.Add(key, value)
    }

    parsedURL.RawQuery = query.Encode()
    return parsedURL.String()
}
