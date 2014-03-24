// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "bytes"
    "encoding/base64"
    "errors"
    "fmt"
    "io/ioutil"
    . "launchpad.net/gocheck"
    "launchpad.net/gwacl/dedent"
    "net/http"
    "net/url"
    "strings"
    "time"
)

type testComposeHeaders struct{}

var _ = Suite(&testComposeHeaders{})

func makeHttpResponse(status int, body string) *http.Response {
    return &http.Response{
        Status:     fmt.Sprintf("%d", status),
        StatusCode: status,
        Body:       makeResponseBody(body),
    }
}

func (suite *testComposeHeaders) TestNoHeaders(c *C) {
    req, err := http.NewRequest("GET", "http://example.com", nil)
    c.Assert(err, IsNil)

    observed := composeHeaders(req)
    expected := "\n\n\n\n\n\n\n\n\n\n\n"

    c.Assert(observed, Equals, expected)
}

func (suite *testComposeHeaders) TestCreatesHeaders(c *C) {
    req, err := http.NewRequest("GET", "http://example.com", nil)
    c.Assert(err, IsNil)

    var items []string
    for i, headerName := range headersToSign {
        v := fmt.Sprintf("%d", i)
        req.Header.Set(headerName, v)
        items = append(items, v+"\n")
    }
    expected := strings.Join(items, "")

    observed := composeHeaders(req)
    c.Assert(observed, Equals, expected)
}

func (suite *testComposeHeaders) TestCanonicalizedHeaders(c *C) {
    req, err := http.NewRequest("GET", "http://example.com", nil)
    c.Assert(err, IsNil)
    req.Header.Set("x-ms-why", "aye")
    req.Header.Set("x-ms-foo", "bar")
    req.Header.Set("invalid", "blah")

    expected := "x-ms-foo:bar\nx-ms-why:aye\n"
    observed := composeCanonicalizedHeaders(req)
    c.Check(observed, Equals, expected)
}

type TestRetryRequests struct{}

var _ = Suite(&TestRetryRequests{})

func (suite *TestRetryRequests) TestRequestIsRetried(c *C) {
    transport := &MockingTransport{}
    body := []byte("data")
    transport.AddExchange(&http.Response{StatusCode: http.StatusConflict, Body: Empty}, nil)
    transport.AddExchange(&http.Response{StatusCode: http.StatusConflict, Body: Empty}, nil)
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: makeResponseBody(string(body))}, nil)
    retryPolicy := RetryPolicy{NbRetries: 3, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Nanosecond}
    context := makeStorageContext(transport)
    context.RetryPolicy = retryPolicy
    req, err := http.NewRequest("GET", "http://example.com", nil)
    c.Assert(err, IsNil)

    resBody, _, err := context.send(req, nil, http.StatusOK)
    c.Assert(err, IsNil)
    c.Assert(transport.ExchangeCount, Equals, 3)
    c.Check(resBody, DeepEquals, body)
}

type TestComposeCanonicalizedResource struct{}

var _ = Suite(&TestComposeCanonicalizedResource{})

func (suite *TestComposeCanonicalizedResource) TestPrependsSlash(c *C) {
    req, err := http.NewRequest("GET", "http://example.com", nil)
    c.Assert(err, IsNil)
    path := MakeRandomString(10)
    req.URL.Path = path
    accountName := MakeRandomString(10)
    observed := composeCanonicalizedResource(req, accountName)
    expected := "/" + accountName + "/" + path
    c.Assert(observed, Equals, expected)
}

func (suite *TestComposeCanonicalizedResource) TestCreatesResource(c *C) {
    path := MakeRandomString(5)
    req, err := http.NewRequest("GET", fmt.Sprintf("http://example.com/%s", path), nil)
    c.Assert(err, IsNil)

    accountName := MakeRandomString(10)
    observed := composeCanonicalizedResource(req, accountName)
    expected := "/" + accountName + "/" + path
    c.Assert(observed, Equals, expected)
}

func (suite *TestComposeCanonicalizedResource) TestQueryParams(c *C) {
    req, err := http.NewRequest(
        "GET", "http://example.com/?Kevin=Perry&foo=bar", nil)
    c.Assert(err, IsNil)

    accountName := MakeRandomString(10)
    observed := composeCanonicalizedResource(req, accountName)
    expected := "/" + accountName + "/\n" + "foo:bar" + "\n" + "kevin:Perry"
    c.Assert(observed, Equals, expected)
}

func (suite *TestComposeCanonicalizedResource) TestToLowerKeys(c *C) {
    values := url.Values{
        "foo":   []string{"bar", "baz"},
        "alpha": []string{"gamma", "delta"},
        "quux":  []string{"flobble"},
        "BIG":   []string{"Big", "Data"},
        "big":   []string{"Big", "Little"},
    }

    observed := toLowerKeys(values)
    expected := map[string][]string{
        "foo":   {"bar", "baz"},
        "alpha": {"delta", "gamma"},
        "quux":  {"flobble"},
        "big":   {"Big", "Big", "Data", "Little"},
    }

    c.Check(observed, DeepEquals, expected)
}

func (suite *TestComposeCanonicalizedResource) TestEncodeParams(c *C) {
    input := map[string][]string{
        "foo":   {"bar", "baz"},
        "alpha": {"delta", "gamma"},
        "quux":  {"flobble"},
        "big":   {"Big", "Big", "Data", "Little"},
    }

    observed := encodeParams(input)
    expected := ("alpha:delta,gamma\nbig:Big,Big,Data,Little\n" +
        "foo:bar,baz\nquux:flobble")
    c.Assert(observed, Equals, expected)
}

func (suite *TestComposeCanonicalizedResource) TestEncodeParamsEmpty(c *C) {
    input := map[string][]string{}
    observed := encodeParams(input)
    expected := ""
    c.Assert(observed, Equals, expected)
}

type TestComposeStringToSign struct{}

var _ = Suite(&TestComposeStringToSign{})

func (suite *TestComposeStringToSign) TestFullRequest(c *C) {
    req, err := http.NewRequest(
        "GET", "http://example.com/mypath?Kevin=Perry&foo=bar", nil)
    c.Assert(err, IsNil)
    for i, headerName := range headersToSign {
        req.Header.Set(headerName, fmt.Sprintf("%v", i))
    }
    req.Header.Set("x-ms-testing", "foo")
    expected := "GET\n0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\nx-ms-testing:foo\n/myaccount/mypath\nfoo:bar\nkevin:Perry"
    observed := composeStringToSign(req, "myaccount")
    c.Assert(observed, Equals, expected)
}

type TestSign struct{}

var _ = Suite(&TestSign{})

func (suite *TestSign) TestSign(c *C) {
    key := base64.StdEncoding.EncodeToString([]byte("dummykey"))
    signable := "a-string-to-sign"

    observed, err := sign(key, signable)
    c.Assert(err, IsNil)
    expected := "5j1DSsm07IEh3u9JQQd0KPwtM6pEGChzrAF7Zf/LxLc="
    c.Assert(observed, Equals, expected)
}

type TestComposeAuthHeader struct{}

var _ = Suite(&TestComposeAuthHeader{})

func (suite *TestComposeAuthHeader) TestCreatesHeaderString(c *C) {
    req, err := http.NewRequest(
        "GET", "http://example.com/mypath?Kevin=Perry&foo=bar", nil)
    c.Assert(err, IsNil)

    key := base64.StdEncoding.EncodeToString([]byte("dummykey"))

    observed, err := composeAuthHeader(req, "myname", key)
    c.Assert(err, IsNil)
    expected := "SharedKey myname:Xf9hWQ99mM0IyEOL6rNeAUdTQlixVqiYnt2TpLCCpY0="
    c.Assert(observed, Equals, expected)
}

type TestSignRequest struct{}

var _ = Suite(&TestSignRequest{})

func (suite *TestSignRequest) TestAddsHeaderToRequest(c *C) {
    req, err := http.NewRequest(
        "GET", "http://example.com/mypath?Kevin=Perry&foo=bar", nil)
    c.Assert(err, IsNil)
    c.Assert(req.Header.Get("Authorization"), Equals, "")

    key := base64.StdEncoding.EncodeToString([]byte("dummykey"))
    context := StorageContext{client: nil, Account: "myname", Key: key}
    context.signRequest(req)

    expected := "SharedKey myname:Xf9hWQ99mM0IyEOL6rNeAUdTQlixVqiYnt2TpLCCpY0="
    c.Assert(req.Header.Get("Authorization"), Equals, expected)
}

func (suite *TestSignRequest) TestDoesNotAddHeaderIfEmptyKey(c *C) {
    req, err := http.NewRequest(
        "GET", "http://example.com/mypath?Kevin=Perry&foo=bar", nil)
    c.Assert(err, IsNil)
    c.Assert(req.Header.Get("Authorization"), Equals, "")

    context := StorageContext{client: nil, Account: "myname", Key: ""}
    context.signRequest(req)

    c.Assert(req.Header.Get("Authorization"), Equals, "")
}

type TestRequestHeaders struct{}

var _ = Suite(&TestRequestHeaders{})

func (suite *TestRequestHeaders) TestAddsVersionHeaderToRequest(c *C) {
    req, err := http.NewRequest("GET", "http://example.com/", nil)
    c.Assert(err, IsNil)
    addVersionHeader(req, "2012-02-12")
    c.Assert(req.Header.Get("x-ms-version"), Equals, "2012-02-12")
}

func (suite *TestRequestHeaders) TestContentHeader(c *C) {
    data := "test data"
    req, err := http.NewRequest("PUT", "http://example.com/", strings.NewReader(data))
    c.Assert(err, IsNil)
    addContentHeaders(req)
    c.Assert(
        req.Header.Get("Content-Length"), Equals, fmt.Sprintf("%d", len(data)))

    // Ensure that reading the request data didn't destroy it.
    reqdata, _ := ioutil.ReadAll(req.Body)
    c.Assert(data, Equals, string(reqdata))
}

func (suite *TestRequestHeaders) TestLengthHeaderNotSetForGET(c *C) {
    req, err := http.NewRequest("GET", "http://example.com/", nil)
    c.Assert(err, IsNil)
    addContentHeaders(req)
    _, lengthPresent := req.Header[http.CanonicalHeaderKey("Content-Length")]
    c.Assert(lengthPresent, Equals, false)
}

func (suite *TestRequestHeaders) TestContentHeaderWithNoBody(c *C) {
    req, err := http.NewRequest("PUT", "http://example.com/", nil)
    c.Assert(err, IsNil)
    addContentHeaders(req)
    _, md5Present := req.Header[http.CanonicalHeaderKey("Content-MD5")]
    c.Check(md5Present, Equals, false)
    content := req.Header.Get("Content-Length")
    c.Check(content, Equals, "0")
}

func (suite *TestRequestHeaders) TestDateHeader(c *C) {
    req, err := http.NewRequest("GET", "http://example.com/", nil)
    c.Assert(err, IsNil)
    c.Assert(req.Header.Get("Date"), Equals, "")
    addDateHeader(req)
    observed := req.Header.Get("Date")
    observedTime, err := time.Parse(time.RFC1123, observed)
    c.Assert(err, IsNil)
    difference := time.Now().UTC().Sub(observedTime)
    if difference.Minutes() > 1.0 {
        c.FailNow()
    }
}

type TestStorageContext struct{}

var _ = Suite(&TestStorageContext{})

// makeNastyURLUnfriendlyString returns a string that really needs escaping
// before it can be included in a URL.
func makeNastyURLUnfriendlyString() string {
    return MakeRandomString(3) + "?&" + MakeRandomString(3) + "$%"
}

func (suite *TestStorageContext) TestGetAccountURLCombinesAccountAndEndpoint(c *C) {
    context := StorageContext{
        Account:       "myaccount",
        AzureEndpoint: "http://example.com",
    }
    c.Check(
        context.getAccountURL(),
        Equals,
        "http://myaccount.blob.example.com")
}

func (suite *TestStorageContext) TestGetAccountURLEscapesHostname(c *C) {
    account := makeNastyURLUnfriendlyString()
    context := StorageContext{
        Account:       account,
        AzureEndpoint: "http://example.com",
    }
    c.Check(
        context.getAccountURL(),
        Equals,
        "http://"+url.QueryEscape(account)+".blob.example.com")
}

func (*TestStorageContext) TestGetAccountURLRequiresEndpoint(c *C) {
    context := StorageContext{Account: "myaccount"}
    c.Check(
        context.getAccountURL,
        Panics,
        errors.New("no AzureEndpoint specified in gwacl.StorageContext"))
}

func (suite *TestStorageContext) TestGetContainerURLAddsContainer(c *C) {
    account := makeNastyURLUnfriendlyString()
    container := makeNastyURLUnfriendlyString()
    context := StorageContext{
        Account:       account,
        AzureEndpoint: "http://example.com/",
    }
    c.Check(
        context.getContainerURL(container),
        Equals,
        "http://"+url.QueryEscape(account)+".blob.example.com/"+url.QueryEscape(container))
}

func (suite *TestStorageContext) TestGetContainerURLAddsSlashIfNeeded(c *C) {
    context := StorageContext{
        Account:       "account",
        AzureEndpoint: "http://example.com",
    }
    c.Check(
        context.getContainerURL("container"),
        Equals,
        "http://account.blob.example.com/container")
}

func (suite *TestStorageContext) TestGetFileURL(c *C) {
    account := makeNastyURLUnfriendlyString()
    container := makeNastyURLUnfriendlyString()
    file := makeNastyURLUnfriendlyString()
    context := StorageContext{
        Account:       account,
        AzureEndpoint: "http://example.com/",
    }
    c.Check(
        context.GetFileURL(container, file),
        Equals,
        "http://"+url.QueryEscape(account)+".blob.example.com/"+url.QueryEscape(container)+"/"+url.QueryEscape(file))
}

func (suite *TestStorageContext) TestGetSignedFileURL(c *C) {
    account := "account"
    container := "container"
    file := "/a/file"
    key := base64.StdEncoding.EncodeToString([]byte("dummykey"))
    context := StorageContext{
        Account:       account,
        Key:           key,
        AzureEndpoint: "http://example.com/",
    }
    expires := time.Now()

    signedURL, err := context.GetAnonymousFileURL(container, file, expires)
    c.Assert(err, IsNil)
    // The only difference with the non-anon URL is the query string.
    parsed, err := url.Parse(signedURL)
    c.Assert(err, IsNil)
    fileURL, err := url.Parse(context.GetFileURL(container, file))
    c.Assert(err, IsNil)
    c.Check(parsed.Scheme, Equals, fileURL.Scheme)
    c.Check(parsed.Host, Equals, fileURL.Host)
    c.Check(parsed.Path, Equals, fileURL.Path)

    values, err := url.ParseQuery(parsed.RawQuery)
    c.Assert(err, IsNil)
    signature := values.Get("sig")
    readValues, err := getReadBlobAccessValues(container, file, account, key, expires)
    c.Assert(err, IsNil)
    expectedSignature := readValues.Get("sig")
    c.Check(signature, Equals, expectedSignature)
}

func (suite *TestStorageContext) TestGetClientReturnsDefaultClient(c *C) {
    context := &StorageContext{client: nil}
    c.Assert(context.getClient(), Equals, http.DefaultClient)
}

func (suite *TestStorageContext) TestGetClientReturnsSpecifiedClient(c *C) {
    context := &StorageContext{client: &http.Client{}}
    c.Assert(context.getClient(), Not(Equals), http.DefaultClient)
    c.Assert(context.getClient(), Equals, context.client)
}

type TestListContainers struct{}

var _ = Suite(&TestListContainers{})

// The ListContainers Storage API call returns a ContainerEnumerationResults
// struct on success.
func (suite *TestListContainers) Test(c *C) {
    responseBody := `
        <?xml version="1.0" encoding="utf-8"?>
        <EnumerationResults AccountName="https://myaccount.blob.core.windows.net">
          <Prefix>prefix-value</Prefix>
          <Marker>marker-value</Marker>
          <MaxResults>max-results-value</MaxResults>
          <Containers>
            <Container>
              <Name>name-value</Name>
              <URL>url-value</URL>
              <Properties>
                <Last-Modified>date/time-value</Last-Modified>
                <Etag>etag-value</Etag>
                <LeaseStatus>lease-status-value</LeaseStatus>
                <LeaseState>lease-state-value</LeaseState>
                <LeaseDuration>lease-duration-value</LeaseDuration>
              </Properties>
              <Metadata>
                <metadata-name>metadata-value</metadata-name>
              </Metadata>
            </Container>
          </Containers>
          <NextMarker/>
        </EnumerationResults>`
    response := makeHttpResponse(http.StatusOK, responseBody)
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    context.AzureEndpoint = "http://example.com/"
    request := &ListContainersRequest{Marker: ""}
    results, err := context.ListContainers(request)
    c.Assert(err, IsNil)
    c.Check(transport.Request.URL.String(), Equals, fmt.Sprintf(
        "http://%s.blob.example.com/?comp=list", context.Account))
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    c.Assert(results, NotNil)
    c.Assert(results.Containers[0].Name, Equals, "name-value")
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestListContainers) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    request := &ListContainersRequest{Marker: ""}
    _, err := context.ListContainers(request)
    c.Assert(err, NotNil)
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestListContainers) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    request := &ListContainersRequest{Marker: ""}
    _, err := context.ListContainers(request)
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

func (suite *TestListContainers) TestListContainersBatchPassesMarker(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: Empty}, nil)
    context := makeStorageContext(transport)

    // Call ListContainers.  This will fail because of the empty
    // response, but no matter.  We only care about the request.
    request := &ListContainersRequest{Marker: "thismarkerhere"}
    _, err := context.ListContainers(request)
    c.Assert(err, ErrorMatches, ".*Failed to deserialize data.*")
    c.Assert(transport.ExchangeCount, Equals, 1)

    query := transport.Exchanges[0].Request.URL.RawQuery
    values, err := url.ParseQuery(query)
    c.Assert(err, IsNil)
    c.Check(values["marker"], DeepEquals, []string{"thismarkerhere"})
}

func (suite *TestListContainers) TestListContainersBatchDoesNotPassEmptyMarker(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: Empty}, nil)
    context := makeStorageContext(transport)

    // The error is OK.  We only care about the request.
    request := &ListContainersRequest{Marker: ""}
    _, err := context.ListContainers(request)
    c.Assert(err, ErrorMatches, ".*Failed to deserialize data.*")
    c.Assert(transport.ExchangeCount, Equals, 1)

    query := transport.Exchanges[0].Request.URL.RawQuery
    values, err := url.ParseQuery(query)
    c.Assert(err, IsNil)
    marker, present := values["marker"]
    c.Check(present, Equals, false)
    c.Check(marker, DeepEquals, []string(nil))
}

func (suite *TestListContainers) TestListContainersBatchEscapesMarker(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: Empty}, nil)
    context := makeStorageContext(transport)

    // The error is OK.  We only care about the request.
    request := &ListContainersRequest{Marker: "x&y"}
    _, err := context.ListContainers(request)
    c.Assert(err, ErrorMatches, ".*Failed to deserialize data.*")
    c.Assert(transport.ExchangeCount, Equals, 1)

    query := transport.Exchanges[0].Request.URL.RawQuery
    values, err := url.ParseQuery(query)
    c.Assert(err, IsNil)
    c.Check(values["marker"], DeepEquals, []string{"x&y"})
}

type TestListBlobs struct{}

var _ = Suite(&TestListBlobs{})

// The ListBlobs Storage API call returns a BlobEnumerationResults struct on
// success.
func (suite *TestListBlobs) Test(c *C) {
    responseBody := `
        <?xml version="1.0" encoding="utf-8"?>
        <EnumerationResults ContainerName="http://myaccount.blob.core.windows.net/mycontainer">
          <Prefix>prefix</Prefix>
          <Marker>marker</Marker>
          <MaxResults>maxresults</MaxResults>
          <Delimiter>delimiter</Delimiter>
          <Blobs>
            <Blob>
              <Name>blob-name</Name>
              <Snapshot>snapshot-date-time</Snapshot>
              <Url>blob-address</Url>
              <Properties>
                <Last-Modified>last-modified</Last-Modified>
                <Etag>etag</Etag>
                <Content-Length>size-in-bytes</Content-Length>
                <Content-Type>blob-content-type</Content-Type>
                <Content-Encoding />
                <Content-Language />
                <Content-MD5 />
                <Cache-Control />
                <x-ms-blob-sequence-number>sequence-number</x-ms-blob-sequence-number>
                <BlobType>blobtype</BlobType>
                <LeaseStatus>leasestatus</LeaseStatus>
                <LeaseState>leasestate</LeaseState>
                <LeaseDuration>leasesduration</LeaseDuration>
                <CopyId>id</CopyId>
                <CopyStatus>copystatus</CopyStatus>
                <CopySource>copysource</CopySource>
                <CopyProgress>copyprogress</CopyProgress>
                <CopyCompletionTime>copycompletiontime</CopyCompletionTime>
                <CopyStatusDescription>copydesc</CopyStatusDescription>
              </Properties>
              <Metadata>
                <MetaName1>metadataname1</MetaName1>
                <MetaName2>metadataname2</MetaName2>
              </Metadata>
            </Blob>
            <BlobPrefix>
              <Name>blob-prefix</Name>
            </BlobPrefix>
          </Blobs>
          <NextMarker />
        </EnumerationResults>`
    response := makeHttpResponse(http.StatusOK, responseBody)
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)

    request := &ListBlobsRequest{Container: "container"}
    results, err := context.ListBlobs(request)
    c.Assert(err, IsNil)
    c.Check(transport.Request.URL.String(), Matches, context.getContainerURL("container")+"?.*")
    c.Check(transport.Request.URL.Query(), DeepEquals, url.Values{
        "restype": {"container"},
        "comp":    {"list"},
    })
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    c.Assert(results, NotNil)
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestListBlobs) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})

    request := &ListBlobsRequest{Container: "container"}
    _, err := context.ListBlobs(request)
    c.Assert(err, NotNil)
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestListBlobs) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    request := &ListBlobsRequest{Container: "container"}
    _, err := context.ListBlobs(request)
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

func (suite *TestListBlobs) TestListBlobsPassesMarker(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: Empty}, nil)
    context := makeStorageContext(transport)

    // Call ListBlobs.  This will fail because of the empty
    // response, but no matter.  We only care about the request.
    request := &ListBlobsRequest{Container: "mycontainer", Marker: "thismarkerhere"}
    _, err := context.ListBlobs(request)
    c.Assert(err, ErrorMatches, ".*Failed to deserialize data.*")
    c.Assert(transport.ExchangeCount, Equals, 1)

    query := transport.Exchanges[0].Request.URL.RawQuery
    values, err := url.ParseQuery(query)
    c.Assert(err, IsNil)
    c.Check(values["marker"], DeepEquals, []string{"thismarkerhere"})
}

func (suite *TestListBlobs) TestListBlobsDoesNotPassEmptyMarker(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: Empty}, nil)
    context := makeStorageContext(transport)

    // The error is OK.  We only care about the request.
    request := &ListBlobsRequest{Container: "mycontainer"}
    _, err := context.ListBlobs(request)
    c.Assert(err, ErrorMatches, ".*Failed to deserialize data.*")
    c.Assert(transport.ExchangeCount, Equals, 1)

    query := transport.Exchanges[0].Request.URL.RawQuery
    values, err := url.ParseQuery(query)
    c.Assert(err, IsNil)
    marker, present := values["marker"]
    c.Check(present, Equals, false)
    c.Check(marker, DeepEquals, []string(nil))
}

func (suite *TestListBlobs) TestListBlobsPassesPrefix(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: Empty}, nil)
    context := makeStorageContext(transport)

    // Call ListBlobs.  This will fail because of the empty
    // response, but no matter.  We only care about the request.
    request := &ListBlobsRequest{Container: "mycontainer", Prefix: "thisprefixhere"}
    _, err := context.ListBlobs(request)
    c.Assert(err, ErrorMatches, ".*Failed to deserialize data.*")
    c.Assert(transport.ExchangeCount, Equals, 1)

    query := transport.Exchanges[0].Request.URL.RawQuery
    values, err := url.ParseQuery(query)
    c.Assert(err, IsNil)
    c.Check(values["prefix"], DeepEquals, []string{"thisprefixhere"})
}

func (suite *TestListBlobs) TestListBlobsDoesNotPassEmptyPrefix(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: Empty}, nil)
    context := makeStorageContext(transport)

    // The error is OK.  We only care about the request.
    request := &ListBlobsRequest{Container: "mycontainer"}
    _, err := context.ListBlobs(request)
    c.Assert(err, ErrorMatches, ".*Failed to deserialize data.*")
    c.Assert(transport.ExchangeCount, Equals, 1)

    query := transport.Exchanges[0].Request.URL.RawQuery
    values, err := url.ParseQuery(query)
    c.Assert(err, IsNil)
    prefix, present := values["prefix"]
    c.Check(present, Equals, false)
    c.Check(prefix, DeepEquals, []string(nil))
}

type TestCreateContainer struct{}

var _ = Suite(&TestCreateContainer{})

// The CreateContainer Storage API call returns without error when the
// container has been created successfully.
func (suite *TestCreateContainer) Test(c *C) {
    response := makeHttpResponse(http.StatusCreated, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    context.AzureEndpoint = "http://example.com/"
    containerName := MakeRandomString(10)
    err := context.CreateContainer(containerName)
    c.Assert(err, IsNil)
    c.Check(transport.Request.URL.String(), Equals, fmt.Sprintf(
        "http://%s.blob.example.com/%s?restype=container",
        context.Account, containerName))
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestCreateContainer) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    err := context.CreateContainer("container")
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *TestCreateContainer) TestErrorResponse(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.CreateContainer("container")
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *TestCreateContainer) TestNotCreatedResponse(c *C) {
    response := makeHttpResponse(http.StatusOK, "")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.CreateContainer("container")
    c.Assert(err, NotNil)
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestCreateContainer) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.CreateContainer("container")
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

type TestDeleteContainer struct{}

var _ = Suite(&TestDeleteContainer{})

// The DeleteContainer Storage API call returns without error when the
// container has been created successfully.
func (suite *TestDeleteContainer) Test(c *C) {
    response := makeHttpResponse(http.StatusAccepted, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    context.AzureEndpoint = "http://example.com/"
    containerName := MakeRandomString(10)
    err := context.DeleteContainer(containerName)
    c.Assert(err, IsNil)
    c.Check(transport.Request.URL.String(), Equals, fmt.Sprintf(
        "http://%s.blob.example.com/%s?restype=container",
        context.Account, containerName))
    c.Check(transport.Request.Method, Equals, "DELETE")
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestDeleteContainer) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    err := context.DeleteContainer("container")
    c.Assert(err, ErrorMatches, ".*canned-error.*")
}

// Server-side errors are propagated back to the caller.
func (suite *TestDeleteContainer) TestNotCreatedResponse(c *C) {
    response := makeHttpResponse(http.StatusOK, "")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.DeleteContainer("container")
    c.Assert(err, ErrorMatches, ".*Azure request failed.*")
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestDeleteContainer) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusMethodNotAllowed, "not allowed")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.DeleteContainer("container")
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusMethodNotAllowed)
}

func (suite *TestDeleteContainer) TestDeleteNotExistentContainerDoesNotFail(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.DeleteContainer("container")
    c.Assert(err, IsNil)
}

type TestGetContainerProperties struct{}

var _ = Suite(&TestGetContainerProperties{})

// The GetContainerProperties Storage API call returns without error when the
// container has been created successfully.
func (suite *TestGetContainerProperties) Test(c *C) {
    header := make(http.Header)
    header.Add("Last-Modified", "last-modified")
    header.Add("ETag", "etag")
    header.Add("X-Ms-Lease-Status", "status")
    header.Add("X-Ms-Lease-State", "state")
    header.Add("X-Ms-Lease-Duration", "duration")
    response := &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusOK),
        StatusCode: http.StatusOK,
        Body:       makeResponseBody(""),
        Header:     header,
    }

    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    context.AzureEndpoint = "http://example.com/"
    containerName := MakeRandomString(10)
    props, err := context.GetContainerProperties(containerName)
    c.Assert(err, IsNil)
    c.Check(transport.Request.URL.String(), Equals, fmt.Sprintf(
        "http://%s.blob.example.com/%s?restype=container",
        context.Account, containerName))
    c.Check(transport.Request.Method, Equals, "GET")
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")

    c.Check(props.LastModified, Equals, "last-modified")
    c.Check(props.ETag, Equals, "etag")
    c.Check(props.LeaseStatus, Equals, "status")
    c.Check(props.LeaseState, Equals, "state")
    c.Check(props.LeaseDuration, Equals, "duration")
}

func (suite *TestGetContainerProperties) TestWithoutAllHeaders(c *C) {
    response := &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusOK),
        StatusCode: http.StatusOK,
        Body:       makeResponseBody(""),
    }

    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    containerName := MakeRandomString(10)
    props, err := context.GetContainerProperties(containerName)
    c.Assert(err, IsNil)

    c.Check(props.LastModified, Equals, "")
    c.Check(props.ETag, Equals, "")
    c.Check(props.LeaseStatus, Equals, "")
    c.Check(props.LeaseState, Equals, "")
    c.Check(props.LeaseDuration, Equals, "")
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestGetContainerProperties) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    _, err := context.GetContainerProperties("container")
    c.Assert(err, ErrorMatches, ".*canned-error.*")
}

// Server-side errors are propagated back to the caller.
func (suite *TestGetContainerProperties) TestErrorResponse(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    _, err := context.GetContainerProperties("container")
    c.Assert(err, ErrorMatches, ".*Not Found.*")
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestGetContainerProperties) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    _, err := context.GetContainerProperties("container")
    serverError, ok := err.(*ServerError)
    c.Assert(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

type TestPutPage struct{}

var _ = Suite(&TestPutPage{})

// Basic happy path testing.
func (suite *TestPutPage) TestHappyPath(c *C) {
    response := makeHttpResponse(http.StatusCreated, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    randomData := MakeRandomByteSlice(10)
    dataReader := bytes.NewReader(randomData)

    err := context.PutPage(&PutPageRequest{
        Container: "container", Filename: "filename", StartRange: 0,
        EndRange: 511, Data: dataReader})
    c.Assert(err, IsNil)

    // Ensure that container was set right.
    c.Check(transport.Request.URL.String(), Matches, context.GetFileURL("container", "filename")+"?.*")
    // Ensure that the Authorization header is set.
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    // Check the range is set.
    c.Check(transport.Request.Header.Get("x-ms-range"), Equals, "bytes=0-511")
    // Check special page write header.
    c.Check(transport.Request.Header.Get("x-ms-page-write"), Equals, "update")
    // "?comp=page" should be part of the URL.
    c.Check(transport.Request.URL.Query(), DeepEquals, url.Values{
        "comp": {"page"},
    })
    // Check the data payload.
    data, err := ioutil.ReadAll(transport.Request.Body)
    c.Assert(err, IsNil)
    c.Check(data, DeepEquals, randomData)
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestPutPage) TestError(c *C) {
    cannedError := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: cannedError})
    err := context.PutPage(&PutPageRequest{
        Container: "container", Filename: "filename", StartRange: 0,
        EndRange: 511, Data: nil})
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *TestPutPage) TestErrorResponse(c *C) {
    responseBody := "<Error><Code>Frotzed</Code><Message>failed to put blob</Message></Error>"
    response := makeHttpResponse(102, responseBody)
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.PutPage(&PutPageRequest{
        Container: "container", Filename: "filename", StartRange: 0,
        EndRange: 511, Data: nil})
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, ".*102.*")
    c.Check(err, ErrorMatches, ".*Frotzed.*")
    c.Check(err, ErrorMatches, ".*failed to put blob.*")
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestPutPage) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.PutPage(&PutPageRequest{
        Container: "container", Filename: "filename", StartRange: 0,
        EndRange: 511, Data: nil})
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

// Range values outside the limits should get rejected.
func (suite *TestPutPage) TestRangeLimits(c *C) {
    context := makeStorageContext(&TestTransport{})
    err := context.PutPage(&PutPageRequest{
        StartRange: 513, EndRange: 555})
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, ".*StartRange must be a multiple of 512, EndRange must be one less than a multiple of 512.*")
}

type TestPutBlob struct{}

var _ = Suite(&TestPutBlob{})

// Test basic PutBlob happy path functionality.
func (suite *TestPutBlob) TestPutBlockBlob(c *C) {
    response := makeHttpResponse(http.StatusCreated, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)

    err := context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "block", Filename: "blobname"})
    c.Assert(err, IsNil)
    // Ensure that container was set right.
    c.Check(transport.Request.URL.String(), Equals, context.GetFileURL("container", "blobname"))
    // Ensure that the Authorization header is set.
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    // The blob type should be a block.
    c.Check(transport.Request.Header.Get("x-ms-blob-type"), Equals, "BlockBlob")
}

// PutBlob should set x-ms-blob-type to PageBlob for Page Blobs.
func (suite *TestPutBlob) TestPutPageBlob(c *C) {
    response := makeHttpResponse(http.StatusCreated, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    err := context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "page", Filename: "blobname",
        Size: 512})
    c.Assert(err, IsNil)
    c.Check(transport.Request.Header.Get("x-ms-blob-type"), Equals, "PageBlob")
    c.Check(transport.Request.Header.Get("x-ms-blob-content-length"), Equals, "512")
}

// PutBlob for a page should return an error when Size is not specified.
func (suite *TestPutBlob) TestPutPageBlobWithSizeOmitted(c *C) {
    context := makeStorageContext(&TestTransport{})
    err := context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "page", Filename: "blob"})
    c.Assert(err, ErrorMatches, "Must supply a size for a page blob")
}

// PutBlob for a page should return an error when Size is not a multiple
// of 512 bytes.
func (suite *TestPutBlob) TestPutPageBlobWithInvalidSiuze(c *C) {
    context := makeStorageContext(&TestTransport{})
    err := context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "page", Filename: "blob",
        Size: 1015})
    c.Assert(err, ErrorMatches, "Size must be a multiple of 512 bytes")
}

// Passing a BlobType other than page or block results in a panic.
func (suite *TestPutBlob) TestBlobType(c *C) {
    defer func() {
        err := recover()
        c.Assert(err, Equals, "blockType must be 'page' or 'block'")
    }()
    context := makeStorageContext(&TestTransport{})
    context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "invalid-blob-type",
        Filename: "blobname"})
    c.Assert("This should have panicked", Equals, "But it didn't.")
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestPutBlob) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    err := context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "block", Filename: "blobname"})
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *TestPutBlob) TestErrorResponse(c *C) {
    responseBody := "<Error><Code>Frotzed</Code><Message>failed to put blob</Message></Error>"
    response := makeHttpResponse(102, responseBody)
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "block", Filename: "blobname"})
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, ".*102.*")
    c.Check(err, ErrorMatches, ".*Frotzed.*")
    c.Check(err, ErrorMatches, ".*failed to put blob.*")
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestPutBlob) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.PutBlob(&PutBlobRequest{
        Container: "container", BlobType: "block", Filename: "blobname"})
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

type TestPutBlock struct{}

var _ = Suite(&TestPutBlock{})

func (suite *TestPutBlock) Test(c *C) {
    response := makeHttpResponse(http.StatusCreated, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    blockid := "\x1b\xea\xf7Mv\xb5\xddH\xebm"
    randomData := MakeRandomByteSlice(10)
    dataReader := bytes.NewReader(randomData)
    err := context.PutBlock("container", "blobname", blockid, dataReader)
    c.Assert(err, IsNil)

    // The blockid should have been base64 encoded and url escaped.
    base64ID := base64.StdEncoding.EncodeToString([]byte(blockid))
    c.Check(transport.Request.URL.String(), Matches, context.GetFileURL("container", "blobname")+"?.*")
    c.Check(transport.Request.URL.Query(), DeepEquals, url.Values{
        "comp":    {"block"},
        "blockid": {base64ID},
    })
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")

    data, err := ioutil.ReadAll(transport.Request.Body)
    c.Assert(err, IsNil)
    c.Check(data, DeepEquals, randomData)
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestPutBlock) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    dataReader := bytes.NewReader(MakeRandomByteSlice(10))
    err := context.PutBlock("container", "blobname", "blockid", dataReader)
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *TestPutBlock) TestErrorResponse(c *C) {
    responseBody := "<Error><Code>Frotzed</Code><Message>failed to put block</Message></Error>"
    response := makeHttpResponse(102, responseBody)
    context := makeStorageContext(&TestTransport{Response: response})
    dataReader := bytes.NewReader(MakeRandomByteSlice(10))
    err := context.PutBlock("container", "blobname", "blockid", dataReader)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, ".*102.*")
    c.Check(err, ErrorMatches, ".*Frotzed.*")
    c.Check(err, ErrorMatches, ".*failed to put block.*")
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestPutBlock) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    dataReader := bytes.NewReader(MakeRandomByteSlice(10))
    err := context.PutBlock("container", "blobname", "blockid", dataReader)
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

type TestPutBlockList struct{}

var _ = Suite(&TestPutBlockList{})

func (suite *TestPutBlockList) Test(c *C) {
    response := makeHttpResponse(http.StatusCreated, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    context.AzureEndpoint = "http://example.com/"
    blocklist := &BlockList{}
    blocklist.Add(BlockListLatest, "b1")
    blocklist.Add(BlockListLatest, "b2")
    err := context.PutBlockList("container", "blobname", blocklist)
    c.Assert(err, IsNil)

    c.Check(transport.Request.Method, Equals, "PUT")
    c.Check(transport.Request.URL.String(), Equals, fmt.Sprintf(
        "http://%s.blob.example.com/container/blobname?comp=blocklist",
        context.Account))
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")

    data, err := ioutil.ReadAll(transport.Request.Body)
    c.Assert(err, IsNil)
    expected := dedent.Dedent(`
        <BlockList>
          <Latest>YjE=</Latest>
          <Latest>YjI=</Latest>
        </BlockList>`)
    c.Check(strings.TrimSpace(string(data)), Equals, strings.TrimSpace(expected))
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestPutBlockList) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    blocklist := &BlockList{}
    err := context.PutBlockList("container", "blobname", blocklist)
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *TestPutBlockList) TestErrorResponse(c *C) {
    responseBody := "<Error><Code>Frotzed</Code><Message>failed to put blocklist</Message></Error>"
    response := makeHttpResponse(102, responseBody)
    context := makeStorageContext(&TestTransport{Response: response})
    blocklist := &BlockList{}
    err := context.PutBlockList("container", "blobname", blocklist)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, ".*102.*")
    c.Check(err, ErrorMatches, ".*Frotzed.*")
    c.Check(err, ErrorMatches, ".*failed to put blocklist.*")
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestPutBlockList) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    blocklist := &BlockList{}
    err := context.PutBlockList("container", "blobname", blocklist)
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

type TestGetBlockList struct{}

var _ = Suite(&TestGetBlockList{})

// The GetBlockList Storage API call returns a GetBlockList struct on
// success.
func (suite *TestGetBlockList) Test(c *C) {
    responseBody := `
        <?xml version="1.0" encoding="utf-8"?>
        <BlockList>
          <CommittedBlocks>
            <Block>
              <Name>BlockId001</Name>
              <Size>4194304</Size>
            </Block>
          </CommittedBlocks>
          <UncommittedBlocks>
            <Block>
              <Name>BlockId002</Name>
              <Size>1024</Size>
            </Block>
          </UncommittedBlocks>
        </BlockList>`

    response := makeHttpResponse(http.StatusOK, responseBody)
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    results, err := context.GetBlockList("container", "myfilename")
    c.Assert(err, IsNil)
    c.Check(transport.Request.URL.String(), Matches, context.GetFileURL("container", "myfilename")+"?.*")
    c.Check(transport.Request.URL.Query(), DeepEquals, url.Values{
        "comp":          {"blocklist"},
        "blocklisttype": {"all"},
    })
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    c.Assert(results, NotNil)
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestGetBlockList) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    _, err := context.GetBlockList("container", "myfilename")
    c.Assert(err, NotNil)
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestGetBlockList) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    _, err := context.GetBlockList("container", "blobname")
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
}

type TestDeleteBlob struct{}

var _ = Suite(&TestDeleteBlob{})

func (suite *TestDeleteBlob) Test(c *C) {
    response := makeHttpResponse(http.StatusAccepted, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    err := context.DeleteBlob("container", "blobname")
    c.Assert(err, IsNil)

    c.Check(transport.Request.Method, Equals, "DELETE")
    c.Check(transport.Request.URL.String(), Equals, context.GetFileURL("container", "blobname"))
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    c.Check(transport.Request.Body, IsNil)
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestDeleteBlob) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    err := context.DeleteBlob("container", "blobname")
    c.Assert(err, NotNil)
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestDeleteBlob) TestServerError(c *C) {
    // We're not using http.StatusNotFound for the test here because
    // 404 errors are handled in a special way by DeleteBlob().  See the test
    // TestDeleteNotExistentBlobDoesNotFail.
    response := makeHttpResponse(http.StatusMethodNotAllowed, "not allowed")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.DeleteBlob("container", "blobname")
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusMethodNotAllowed)
}

func (suite *TestDeleteBlob) TestDeleteNotExistentBlobDoesNotFail(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.DeleteBlob("container", "blobname")
    c.Assert(err, IsNil)
}

// Server-side errors are propagated back to the caller.
func (suite *TestDeleteBlob) TestErrorResponse(c *C) {
    responseBody := "<Error><Code>Frotzed</Code><Message>failed to delete blob</Message></Error>"
    response := makeHttpResponse(146, responseBody)
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.DeleteBlob("container", "blobname")
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, ".*146.*")
    c.Check(err, ErrorMatches, ".*Frotzed.*")
    c.Check(err, ErrorMatches, ".*failed to delete blob.*")
}

type TestGetBlob struct{}

var _ = Suite(&TestGetBlob{})

func (suite *TestGetBlob) Test(c *C) {
    responseBody := "blob-in-a-can"
    response := makeHttpResponse(http.StatusOK, responseBody)
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    reader, err := context.GetBlob("container", "blobname")
    c.Assert(err, IsNil)
    c.Assert(reader, NotNil)
    defer reader.Close()

    c.Check(transport.Request.Method, Equals, "GET")
    c.Check(transport.Request.URL.String(), Equals, context.GetFileURL("container", "blobname"))
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")

    data, err := ioutil.ReadAll(reader)
    c.Assert(err, IsNil)
    c.Check(string(data), Equals, responseBody)
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *TestGetBlob) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    reader, err := context.GetBlob("container", "blobname")
    c.Check(reader, IsNil)
    c.Assert(err, NotNil)
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestGetBlob) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    reader, err := context.GetBlob("container", "blobname")
    c.Check(reader, IsNil)
    c.Assert(err, NotNil)
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
    c.Check(IsNotFoundError(err), Equals, true)
}

// Server-side errors are propagated back to the caller.
func (suite *TestGetBlob) TestErrorResponse(c *C) {
    response := &http.Response{
        Status:     "246 Frotzed",
        StatusCode: 246,
        Body:       makeResponseBody("<Error><Code>Frotzed</Code><Message>failed to get blob</Message></Error>"),
    }
    context := makeStorageContext(&TestTransport{Response: response})
    reader, err := context.GetBlob("container", "blobname")
    c.Check(reader, IsNil)
    c.Assert(err, NotNil)
    c.Check(err, ErrorMatches, ".*246.*")
    c.Check(err, ErrorMatches, ".*Frotzed.*")
    c.Check(err, ErrorMatches, ".*failed to get blob.*")
}

type TestSetContainerACL struct{}

var _ = Suite(&TestSetContainerACL{})

func (suite *TestSetContainerACL) TestHappyPath(c *C) {
    response := makeHttpResponse(http.StatusOK, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    context.AzureEndpoint = "http://example.com/"
    err := context.SetContainerACL(&SetContainerACLRequest{
        Container: "mycontainer", Access: "container"})

    c.Assert(err, IsNil)
    c.Check(transport.Request.Method, Equals, "PUT")
    c.Check(transport.Request.URL.String(), Matches,
        fmt.Sprintf(
            "http://%s.blob.example.com/mycontainer?.*", context.Account))
    c.Check(transport.Request.URL.Query(), DeepEquals, url.Values{
        "comp":    {"acl"},
        "restype": {"container"},
    })

    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    c.Check(transport.Request.Header.Get("x-ms-blob-public-access"), Equals, "container")
}

func (suite *TestSetContainerACL) TestAcceptsBlobAccess(c *C) {
    response := makeHttpResponse(http.StatusOK, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    err := context.SetContainerACL(&SetContainerACLRequest{
        Container: "mycontainer", Access: "blob"})
    c.Assert(err, IsNil)
    c.Check(transport.Request.Header.Get("x-ms-blob-public-access"), Equals, "blob")
}

func (suite *TestSetContainerACL) TestAccessHeaderOmittedWhenPrivate(c *C) {
    response := makeHttpResponse(http.StatusOK, "")
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    err := context.SetContainerACL(&SetContainerACLRequest{
        Container: "mycontainer", Access: "private"})

    c.Assert(err, IsNil)
    c.Check(transport.Request.Header.Get("x-ms-blob-public-access"), Equals, "")
}

func (suite *TestSetContainerACL) TestInvalidAccessTypePanics(c *C) {
    defer func() {
        err := recover()
        c.Assert(err, Equals, "Access must be one of 'container', 'blob' or 'private'")
    }()
    context := makeStorageContext(&TestTransport{})
    context.SetContainerACL(&SetContainerACLRequest{
        Container: "mycontainer", Access: "thisisnotvalid"})
    c.Assert("This test failed", Equals, "because there was no panic")
}

func (suite *TestSetContainerACL) TestClientSideError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    err := context.SetContainerACL(&SetContainerACLRequest{
        Container: "mycontainer", Access: "private"})
    c.Assert(err, NotNil)
}

// Azure HTTP errors (for instance 404 responses) are propagated back to
// the caller as ServerError objects.
func (suite *TestSetContainerACL) TestServerError(c *C) {
    response := makeHttpResponse(http.StatusNotFound, "not found")
    context := makeStorageContext(&TestTransport{Response: response})
    err := context.SetContainerACL(&SetContainerACLRequest{
        Container: "mycontainer", Access: "private"})
    c.Assert(err, NotNil)
    serverError, ok := err.(*ServerError)
    c.Check(ok, Equals, true)
    c.Check(serverError.HTTPStatus.StatusCode(), Equals, http.StatusNotFound)
    c.Check(IsNotFoundError(err), Equals, true)
}
