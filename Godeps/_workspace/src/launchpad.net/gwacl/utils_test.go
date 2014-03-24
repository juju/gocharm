// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "io"
    "io/ioutil"
    . "launchpad.net/gocheck"
    "net/url"
    "strings"
)

type UtilsSuite struct{}

var _ = Suite(&UtilsSuite{})

func (suite *UtilsSuite) TestCheckPathComponents(c *C) {
    checkPathComponents("fred", "bob", "123", "a..b") // All okay.
    c.Check(
        func() { checkPathComponents("foo^bar") },
        PanicMatches, "'foo\\^bar' contains URI special characters")
    c.Check(
        func() { checkPathComponents("foo/bar") },
        PanicMatches, "'foo/bar' contains URI special characters")
    c.Check(
        func() { checkPathComponents("..") },
        PanicMatches, "'[.][.]' is not allowed")
}

func (*UtilsSuite) TestReadAndCloseReturnsEmptyStringForNil(c *C) {
    data, err := readAndClose(nil)
    c.Assert(err, IsNil)
    c.Check(string(data), Equals, "")
}

func (*UtilsSuite) TestReadAndCloseReturnsContents(c *C) {
    content := "Stream contents."
    stream := ioutil.NopCloser(strings.NewReader(content))

    data, err := readAndClose(stream)
    c.Assert(err, IsNil)

    c.Check(string(data), Equals, content)
}

// fakeStream is a very simple fake implementation of io.ReadCloser.  It
// acts like an empty stream, but it tracks whether it's been closed yet.
type fakeStream struct {
    closed bool
}

func (stream *fakeStream) Read([]byte) (int, error) {
    if stream.closed {
        panic("Read() from closed fakeStream")
    }
    return 0, io.EOF
}

func (stream *fakeStream) Close() error {
    stream.closed = true
    return nil
}

func (*UtilsSuite) TestReadAndCloseCloses(c *C) {
    stream := &fakeStream{}

    _, err := readAndClose(stream)
    c.Assert(err, IsNil)

    c.Check(stream.closed, Equals, true)
}

type TestAddURLQueryParams struct{}

var _ = Suite(&TestAddURLQueryParams{})

func (*TestAddURLQueryParams) TestUsesBaseURL(c *C) {
    baseURL := "http://example.com"

    extendedURL := addURLQueryParams(baseURL, "key", "value")

    parsedURL, err := url.Parse(extendedURL)
    c.Assert(err, IsNil)
    c.Check(parsedURL.Scheme, Equals, "http")
    c.Check(parsedURL.Host, Equals, "example.com")
}

func (suite *TestAddURLQueryParams) TestEscapesParams(c *C) {
    key := "key&key"
    value := "value%value"

    uri := addURLQueryParams("http://example.com", key, value)

    parsedURL, err := url.Parse(uri)
    c.Assert(err, IsNil)
    c.Check(parsedURL.Query()[key], DeepEquals, []string{value})
}

func (suite *TestAddURLQueryParams) TestAddsToExistingParams(c *C) {
    uri := addURLQueryParams("http://example.com?a=one", "b", "two")

    parsedURL, err := url.Parse(uri)
    c.Assert(err, IsNil)
    c.Check(parsedURL.Query(), DeepEquals, url.Values{
        "a": {"one"},
        "b": {"two"},
    })
}

func (suite *TestAddURLQueryParams) TestAppendsRepeatedParams(c *C) {
    uri := addURLQueryParams("http://example.com?foo=bar", "foo", "bar")
    c.Check(uri, Equals, "http://example.com?foo=bar&foo=bar")
}

func (suite *TestAddURLQueryParams) TestAddsMultipleParams(c *C) {
    uri := addURLQueryParams("http://example.com", "one", "un", "two", "deux")
    parsedURL, err := url.Parse(uri)
    c.Assert(err, IsNil)
    c.Check(parsedURL.Query(), DeepEquals, url.Values{
        "one": {"un"},
        "two": {"deux"},
    })
}

func (suite *TestAddURLQueryParams) TestRejectsOddNumberOfParams(c *C) {
    defer func() {
        err := recover()
        c.Check(err, ErrorMatches, ".*got 1 parameter.*")
    }()
    addURLQueryParams("http://example.com", "key")
    c.Assert("This should have panicked", Equals, "But it didn't.")
}
