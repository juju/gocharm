// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "bytes"
    "encoding/base64"
    "fmt"
    "io/ioutil"
    . "launchpad.net/gocheck"
    "net/http"
    "net/url"
    "strings"
)

type testUploadBlockBlob struct{}

var _ = Suite(&testUploadBlockBlob{})

func (suite *testUploadBlockBlob) TestSmallFile(c *C) {
    transport := &MockingTransport{}
    context := makeStorageContext(transport)
    // UploadBlockBlob uses PutBlock to upload the data.
    transport.AddExchange(makeFakeCreatedResponse(), nil)
    // UploadBlockBlob then sends the list of blocks with PutBlockList.
    transport.AddExchange(makeFakeCreatedResponse(), nil)
    // Upload a random blob of data.
    data := uploadRandomBlob(c, context, 10, "MyContainer", "MyFile")
    // There were two exchanges.
    c.Assert(transport.ExchangeCount, Equals, 2)
    // The first request is a Put Block with the block data.
    fileURL := context.GetFileURL("MyContainer", "MyFile")
    assertBlockSent(c, context, data, b64("000000000000000000000000000000"), transport.Exchanges[0], fileURL)
    // The second request is Put Block List to commit the block above.
    assertBlockListSent(c, context, []string{b64("000000000000000000000000000000")}, transport.Exchanges[1], fileURL)
}

func (suite *testUploadBlockBlob) TestLargeFile(c *C) {
    transport := &MockingTransport{}
    context := makeStorageContext(transport)
    // UploadBlockBlob uses PutBlock twice to upload the data.
    transport.AddExchange(makeFakeCreatedResponse(), nil)
    transport.AddExchange(makeFakeCreatedResponse(), nil)
    // UploadBlockBlob then sends the list of blocks with PutBlockList.
    transport.AddExchange(makeFakeCreatedResponse(), nil)
    // Upload a large random blob of data.
    data := uploadRandomBlob(c, context, 1348*1024, "MyContainer", "MyFile")
    // There were three exchanges.
    c.Assert(transport.ExchangeCount, Equals, 3)
    // The first two requests are Put Block with chunks of the block data. The
    // weird looking block IDs are base64 encodings of the strings "0" and "1".
    fileURL := context.GetFileURL("MyContainer", "MyFile")
    assertBlockSent(c, context, data[:1024*1024], b64("000000000000000000000000000000"), transport.Exchanges[0], fileURL)
    assertBlockSent(c, context, data[1024*1024:], b64("000000000000000000000000000001"), transport.Exchanges[1], fileURL)
    // The second request is Put Block List to commit the block above.
    assertBlockListSent(c, context, []string{b64("000000000000000000000000000000"), b64("000000000000000000000000000001")}, transport.Exchanges[2], fileURL)
}

func uploadRandomBlob(c *C, context *StorageContext, size int, container, filename string) []byte {
    data := MakeRandomByteSlice(size)
    err := context.UploadBlockBlob(
        container, filename, bytes.NewReader(data))
    c.Assert(err, IsNil)
    return data
}

func assertBlockSent(
    c *C, context *StorageContext, data []byte, blockID string, exchange *MockingTransportExchange, fileURL string) {
    c.Check(exchange.Request.URL.String(), Matches, fileURL+"?.*")
    c.Check(exchange.Request.URL.Query(), DeepEquals, url.Values{
        "comp":    {"block"},
        "blockid": {blockID},
    })
    body, err := ioutil.ReadAll(exchange.Request.Body)
    c.Assert(err, IsNil)
    // DeepEquals is painfully slow when comparing larger structures, so we
    // compare the expected (data) and observed (body) slices directly.
    c.Assert(len(body), Equals, len(data))
    for i := range body {
        // c.Assert also noticably slows things down; this condition is an
        // optimisation of the c.Assert call contained within.
        if body[i] != data[i] {
            c.Assert(body[i], Equals, data[i])
        }
    }
}

func assertBlockListSent(
    c *C, context *StorageContext, blockIDs []string, exchange *MockingTransportExchange, fileURL string) {
    c.Check(exchange.Request.URL.String(), Matches, fileURL+"?.*")
    c.Check(exchange.Request.URL.Query(), DeepEquals, url.Values{
        "comp": {"blocklist"},
    })
    body, err := ioutil.ReadAll(exchange.Request.Body)
    c.Check(err, IsNil)
    expected := "<BlockList>\n"
    for _, blockID := range blockIDs {
        expected += "  <Latest>" + blockID + "</Latest>\n"
    }
    expected += "</BlockList>"
    c.Check(strings.TrimSpace(string(body)), Equals, strings.TrimSpace(expected))
}

type testListAllBlobs struct{}

var _ = Suite(&testListAllBlobs{})

// The ListAllBlobs Storage API call returns a BlobEnumerationResults struct
// on success.
func (suite *testListAllBlobs) Test(c *C) {
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
    response := &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusOK),
        StatusCode: http.StatusOK,
        Body:       makeResponseBody(responseBody),
    }
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    request := &ListBlobsRequest{Container: "container"}
    results, err := context.ListAllBlobs(request)
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
func (suite *testListAllBlobs) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    request := &ListBlobsRequest{Container: "container"}
    _, err := context.ListAllBlobs(request)
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *testListAllBlobs) TestErrorResponse(c *C) {
    response := &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusNotFound),
        StatusCode: http.StatusNotFound,
    }
    context := makeStorageContext(&TestTransport{Response: response})
    request := &ListBlobsRequest{Container: "container"}
    _, err := context.ListAllBlobs(request)
    c.Assert(err, NotNil)
}

// ListAllBlobs combines multiple batches of output.
func (suite *testListAllBlobs) TestBatchedResult(c *C) {
    firstBlob := "blob1"
    lastBlob := "blob2"
    marker := "moreplease"
    firstBatch := http.Response{
        StatusCode: http.StatusOK,
        Body: makeResponseBody(fmt.Sprintf(`
            <EnumerationResults>
              <Blobs>
                <Blob>
                  <Name>%s</Name>
                </Blob>
              </Blobs>
              <NextMarker>%s</NextMarker>
            </EnumerationResults>
        `, firstBlob, marker)),
    }
    lastBatch := http.Response{
        StatusCode: http.StatusOK,
        Body: makeResponseBody(fmt.Sprintf(`
            <EnumerationResults>
              <Blobs>
                <Blob>
                  <Name>%s</Name>
                </Blob>
              </Blobs>
            </EnumerationResults>
        `, lastBlob)),
    }
    transport := &MockingTransport{}
    transport.AddExchange(&firstBatch, nil)
    transport.AddExchange(&lastBatch, nil)
    context := makeStorageContext(transport)

    request := &ListBlobsRequest{Container: "mycontainer"}
    blobs, err := context.ListAllBlobs(request)
    c.Assert(err, IsNil)

    c.Check(len(blobs.Blobs), Equals, 2)
    c.Check(blobs.Blobs[0].Name, Equals, firstBlob)
    c.Check(blobs.Blobs[1].Name, Equals, lastBlob)
}

type testListAllContainers struct{}

var _ = Suite(&testListAllContainers{})

// The ListAllContainers Storage API call returns a ContainerEnumerationResults
// struct on success.
func (suite *testListAllContainers) Test(c *C) {
    responseBody := `
        <?xml version="1.0" encoding="utf-8"?>
        <EnumerationResults AccountName="http://myaccount.blob.core.windows.net">
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
    response := &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusOK),
        StatusCode: http.StatusOK,
        Body:       makeResponseBody(responseBody),
    }
    transport := &TestTransport{Response: response}
    context := makeStorageContext(transport)
    context.AzureEndpoint = "http://example.com/"
    results, err := context.ListAllContainers()
    c.Assert(err, IsNil)
    c.Check(transport.Request.URL.String(), Equals, fmt.Sprintf(
        "http://%s.blob.example.com/?comp=list", context.Account))
    c.Check(transport.Request.Header.Get("Authorization"), Not(Equals), "")
    c.Assert(results, NotNil)
    c.Assert(results.Containers[0].Name, Equals, "name-value")
}

// Client-side errors from the HTTP client are propagated back to the caller.
func (suite *testListAllContainers) TestError(c *C) {
    error := fmt.Errorf("canned-error")
    context := makeStorageContext(&TestTransport{Error: error})
    _, err := context.ListAllContainers()
    c.Assert(err, NotNil)
}

// Server-side errors are propagated back to the caller.
func (suite *testListAllContainers) TestErrorResponse(c *C) {
    response := &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusNotFound),
        StatusCode: http.StatusNotFound,
    }
    context := makeStorageContext(&TestTransport{Response: response})
    _, err := context.ListAllContainers()
    c.Assert(err, NotNil)
}

// ListAllContainers combines multiple batches of output.
func (suite *testListAllContainers) TestBatchedResult(c *C) {
    firstContainer := "container1"
    lastContainer := "container2"
    marker := "moreplease"
    firstBatch := http.Response{
        StatusCode: http.StatusOK,
        Body: makeResponseBody(fmt.Sprintf(`
            <EnumerationResults>
              <Containers>
                <Container>
                  <Name>%s</Name>
                  <URL>container-address</URL>
                </Container>
              </Containers>
              <NextMarker>%s</NextMarker>
            </EnumerationResults>
        `, firstContainer, marker)),
    }
    lastBatch := http.Response{
        StatusCode: http.StatusOK,
        Body: makeResponseBody(fmt.Sprintf(`
            <EnumerationResults>
              <Containers>
                <Container>
                  <Name>%s</Name>
                  <URL>container-address</URL>
                </Container>
              </Containers>
            </EnumerationResults>
        `, lastContainer)),
    }
    transport := &MockingTransport{}
    transport.AddExchange(&firstBatch, nil)
    transport.AddExchange(&lastBatch, nil)
    context := makeStorageContext(transport)

    containers, err := context.ListAllContainers()
    c.Assert(err, IsNil)

    c.Check(len(containers.Containers), Equals, 2)
    c.Check(containers.Containers[0].Name, Equals, firstContainer)
    c.Check(containers.Containers[1].Name, Equals, lastContainer)
}

type testDeleteAllBlobs struct{}

var _ = Suite(&testDeleteAllBlobs{})

func (s *testDeleteAllBlobs) makeListingResponse() *http.Response {
    return &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusOK),
        StatusCode: http.StatusOK,
        Body: makeResponseBody(`
            <?xml version="1.0" encoding="utf-8"?>
            <EnumerationResults ContainerName="http://myaccount.blob.core.windows.net/mycontainer">
              <Prefix>prefix</Prefix>
              <Marker>marker</Marker>
              <MaxResults>maxresults</MaxResults>
              <Delimiter>delimiter</Delimiter>
              <Blobs>
                <Blob>
                  <Name>blob-name</Name>
                </Blob>
                <Blob>
                  <Name>blob-name2</Name>
                </Blob>
              </Blobs>
              <NextMarker />
            </EnumerationResults>`),
    }
}

func (s *testDeleteAllBlobs) TestHappyPath(c *C) {
    listResponse := s.makeListingResponse()
    deleteResponse := &http.Response{
        Status:     fmt.Sprintf("%d", http.StatusAccepted),
        StatusCode: http.StatusAccepted,
    }

    transport := &MockingTransport{}
    transport.AddExchange(listResponse, nil)
    transport.AddExchange(deleteResponse, nil)
    transport.AddExchange(deleteResponse, nil)
    context := makeStorageContext(transport)

    err := context.DeleteAllBlobs(&DeleteAllBlobsRequest{Container: "container"})
    c.Assert(err, IsNil)
    c.Assert(transport.ExchangeCount, Equals, 3)

    // Check the ListAllBlobs exchange.
    c.Check(
        transport.Exchanges[0].Request.URL.String(),
        Matches, context.getContainerURL("container")+"[?].*")
    c.Check(transport.Exchanges[0].Request.URL.Query(),
        DeepEquals, url.Values{
            "restype": {"container"},
            "comp":    {"list"},
        })

    // Check the DeleteBlob exchanges.
    c.Check(
        transport.Exchanges[1].Request.URL.String(),
        Equals, context.GetFileURL("container", "blob-name"))
    c.Check(transport.Exchanges[1].Request.Method, Equals, "DELETE")

    c.Check(
        transport.Exchanges[2].Request.URL.String(),
        Equals, context.GetFileURL("container", "blob-name2"))
    c.Check(transport.Exchanges[2].Request.Method, Equals, "DELETE")
}

func (s *testDeleteAllBlobs) TestErrorsListing(c *C) {
    transport := &MockingTransport{}
    transport.AddExchange(&http.Response{
        Status:     fmt.Sprintf("%d", http.StatusNotFound),
        StatusCode: http.StatusNotFound}, nil)
    context := makeStorageContext(transport)
    err := context.DeleteAllBlobs(&DeleteAllBlobsRequest{Container: "c"})
    c.Assert(err, ErrorMatches, `request for blobs list failed: Azure request failed \(404: Not Found\)`)
}

func (s *testDeleteAllBlobs) TestErrorsDeleting(c *C) {
    transport := &MockingTransport{}
    listResponse := s.makeListingResponse()
    transport.AddExchange(listResponse, nil)
    transport.AddExchange(&http.Response{
        Status:     fmt.Sprintf("%d", http.StatusBadRequest),
        StatusCode: http.StatusBadRequest}, nil)
    context := makeStorageContext(transport)
    err := context.DeleteAllBlobs(&DeleteAllBlobsRequest{Container: "c"})
    c.Assert(err, ErrorMatches, `failed to delete blob blob-name: Azure request failed \(400: Bad Request\)`)
}

type testCreateInstanceDataVHD struct{}

var _ = Suite(&testCreateInstanceDataVHD{})

func (s *testCreateInstanceDataVHD) TestHappyPath(c *C) {
    response := http.Response{
        Status:     fmt.Sprintf("%d", http.StatusOK),
        StatusCode: http.StatusCreated,
    }
    transport := &MockingTransport{}
    transport.AddExchange(&response, nil) // putblob response
    transport.AddExchange(&response, nil) // first putpage
    transport.AddExchange(&response, nil) // second putpage
    context := makeStorageContext(transport)

    randomData := MakeRandomByteSlice(512)
    dataReader := bytes.NewReader(randomData)

    var err error

    err = context.CreateInstanceDataVHD(&CreateVHDRequest{
        Container: "container", Filename: "filename",
        FilesystemData: dataReader, Size: 512})
    c.Assert(err, IsNil)

    // Check the PutBlob exchange.
    c.Check(
        transport.Exchanges[0].Request.Header.Get("x-ms-blob-type"),
        Equals, "PageBlob")
    expectedSize := fmt.Sprintf("%d", VHD_SIZE)
    c.Check(
        transport.Exchanges[0].Request.Header.Get("x-ms-blob-content-length"),
        Equals, expectedSize)

    // Check the PutPage for the footer exchange.
    data, err := ioutil.ReadAll(transport.Exchanges[1].Request.Body)
    c.Assert(err, IsNil)
    expectedData, err := base64.StdEncoding.DecodeString(VHD_FOOTER)
    c.Assert(err, IsNil)
    c.Check(data, DeepEquals, expectedData)
    expectedRange := fmt.Sprintf("bytes=%d-%d", VHD_SIZE-512, VHD_SIZE-1)
    c.Check(
        transport.Exchanges[1].Request.Header.Get("x-ms-range"),
        Equals, expectedRange)

    // Check the PutPage for the data exchange.
    data, err = ioutil.ReadAll(transport.Exchanges[2].Request.Body)
    c.Assert(err, IsNil)
    c.Check(data, DeepEquals, randomData)

    c.Check(
        transport.Exchanges[2].Request.Header.Get("x-ms-range"),
        Equals, "bytes=0-511")
}

func (s *testCreateInstanceDataVHD) TestSizeConstraints(c *C) {
    var err error
    context := makeStorageContext(&TestTransport{})

    err = context.CreateInstanceDataVHD(&CreateVHDRequest{Size: 10})
    c.Check(err, ErrorMatches, "Size must be a multiple of 512")

    err = context.CreateInstanceDataVHD(&CreateVHDRequest{
        Size: VHD_SIZE})
    errString := fmt.Sprintf("Size cannot be bigger than %d", VHD_SIZE-512)
    c.Check(err, ErrorMatches, errString)
}
