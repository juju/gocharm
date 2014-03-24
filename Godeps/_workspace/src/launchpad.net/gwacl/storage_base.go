// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

// This file contains the operations necessary to work with the Azure
// file storage API.  For more details, see
// http://msdn.microsoft.com/en-us/library/windowsazure/dd179355.aspx

// TODO Improve function documentation: the Go documentation convention is for
// function documentation to start out with the name of the function. This may
// have special significance for godoc.

import (
    "bytes"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "errors"
    "fmt"
    "io"
    "io/ioutil"
    "net/http"
    "net/url"
    "sort"
    "strings"
    "time"
)

var headersToSign = []string{
    "Content-Encoding",
    "Content-Language",
    "Content-Length",
    "Content-MD5",
    "Content-Type",
    "Date",
    "If-Modified-Since",
    "If-Match",
    "If-None-Match",
    "If-Unmodified-Since",
    "Range",
}

func init() {
    // See https://code.google.com/p/go/issues/detail?id=4677
    // We need to force the connection to close each time so that we don't
    // hit the above Go bug.
    roundTripper := http.DefaultClient.Transport
    if transport, ok := roundTripper.(*http.Transport); ok {
        transport.DisableKeepAlives = true
    }
    http.DefaultTransport.(*http.Transport).DisableKeepAlives = true
}

// sign returns the base64-encoded HMAC-SHA256 signature of the given string
// using the given base64-encoded key.
func sign(accountKey, signable string) (string, error) {
    // Allegedly, this is already UTF8 encoded.
    decodedKey, err := base64.StdEncoding.DecodeString(accountKey)
    if err != nil {
        return "", fmt.Errorf("invalid account key: %s", err)
    }
    hash := hmac.New(sha256.New, decodedKey)
    _, err = hash.Write([]byte(signable))
    if err != nil {
        return "", fmt.Errorf("failed to write hash: %s", err)
    }
    var hashed []byte
    hashed = hash.Sum(hashed)
    b64Hashed := base64.StdEncoding.EncodeToString(hashed)
    return b64Hashed, nil
}

// Calculate the value required for an Authorization header.
func composeAuthHeader(req *http.Request, accountName, accountKey string) (string, error) {
    signable := composeStringToSign(req, accountName)

    b64Hashed, err := sign(accountKey, signable)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("SharedKey %s:%s", accountName, b64Hashed), nil
}

// Calculate the string that needs to be HMAC signed.  It is comprised of
// the headers in headersToSign, x-ms-* headers and the URI params.
func composeStringToSign(req *http.Request, accountName string) string {
    // TODO: whitespace should be normalised in value strings.
    return fmt.Sprintf(
        "%s\n%s%s%s", req.Method, composeHeaders(req),
        composeCanonicalizedHeaders(req),
        composeCanonicalizedResource(req, accountName))
}

// toLowerKeys lower cases all map keys. If two keys exist, that differ
// by the case of their keys, the values will be concatenated.
func toLowerKeys(values url.Values) map[string][]string {
    m := make(map[string][]string)
    for k, v := range values {
        k = strings.ToLower(k)
        m[k] = append(m[k], v...)
    }
    for _, v := range m {
        sort.Strings(v)
    }
    return m
}

// Encode the URI params as required by the API.  They are lower-cased,
// sorted and formatted as param:value,value,...\nparam:value...
func encodeParams(values map[string][]string) string {
    var keys []string
    values = toLowerKeys(values)
    for k := range values {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    var result []string
    for _, v := range keys {
        result = append(result, fmt.Sprintf("%v:%s", v, strings.Join(values[v], ",")))
    }
    return strings.Join(result, "\n")
}

// Calculate the headers required in the string to sign.
func composeHeaders(req *http.Request) string {
    var result []string
    for _, headerName := range headersToSign {
        result = append(result, req.Header.Get(headerName)+"\n")
    }
    return strings.Join(result, "")
}

// Calculate the x-ms-* headers, encode as for encodeParams.
func composeCanonicalizedHeaders(req *http.Request) string {
    var results []string
    for headerName, values := range req.Header {
        headerName = strings.ToLower(headerName)
        if strings.HasPrefix(headerName, "x-ms-") {
            results = append(results, fmt.Sprintf("%v:%s\n", headerName, strings.Join(values, ",")))
        }
    }
    sort.Strings(results)
    return strings.Join(results, "")
}

// Calculate the URI params and encode them in the string.
// See http://msdn.microsoft.com/en-us/library/windowsazure/dd179428.aspx
// for details of this encoding.
func composeCanonicalizedResource(req *http.Request, accountName string) string {
    path := req.URL.Path
    if !strings.HasPrefix(path, "/") {
        path = "/" + path
    }

    values := req.URL.Query()
    valuesLower := toLowerKeys(values)
    paramString := encodeParams(valuesLower)

    result := "/" + accountName + path
    if paramString != "" {
        result += "\n" + paramString
    }

    return result
}

// Take the passed msVersion string and add it to the request headers.
func addVersionHeader(req *http.Request, msVersion string) {
    req.Header.Set("x-ms-version", msVersion)
}

// Calculate the mD5sum and content length for the request payload and add
// as the Content-MD5 header and Content-Length header respectively.
func addContentHeaders(req *http.Request) {
    if req.Body == nil {
        if req.Method == "PUT" || req.Method == "POST" {
            // This cannot be set for a GET, likewise it *must* be set for
            // PUT and POST.
            req.Header.Set("Content-Length", "0")
        }
        return
    }
    reqdata, err := ioutil.ReadAll(req.Body)
    if err != nil {
        panic(fmt.Errorf("Unable to read request body: %s", err))
    }
    // Replace the request's data because we just destroyed it by reading it.
    req.Body = ioutil.NopCloser(bytes.NewReader(reqdata))
    req.Header.Set("Content-Length", fmt.Sprintf("%d", len(reqdata)))
    // Stop Go's http lib from chunking the data because Azure will return
    // an authorization error if it's chunked.
    req.ContentLength = int64(len(reqdata))
}

// Add a Date: header in RFC1123 format.
func addDateHeader(req *http.Request) {
    now := time.Now().UTC().Format(time.RFC1123)
    // The Azure API requires "GMT" and not "UTC".
    now = strings.Replace(now, "UTC", "GMT", 1)
    req.Header.Set("Date", now)
}

// signRequest adds the Authorization: header to a Request.
// Don't make any further changes to the request before sending it, or the
// signature will not be valid.
func (context *StorageContext) signRequest(req *http.Request) error {
    // Only sign the request if the key is not empty.
    if context.Key != "" {
        header, err := composeAuthHeader(req, context.Account, context.Key)
        if err != nil {
            return err
        }
        req.Header.Set("Authorization", header)
    }
    return nil
}

// StorageContext keeps track of the mandatory parameters required to send a
// request to the storage services API.  It also has an HTTP Client to allow
// overriding for custom behaviour, during testing for example.
type StorageContext struct {
    // Account is a storage account name.
    Account string

    // Key authenticates the storage account.  Access will be anonymous if this
    // is left empty.
    Key string

    // AzureEndpoint specifies a base service endpoint URL for the Azure APIs.
    // This field is required.
    AzureEndpoint APIEndpoint

    client *http.Client

    RetryPolicy RetryPolicy
}

// getClient is used when sending a request. If a custom client is specified
// in context.client it is returned, otherwise net.http.DefaultClient is
// returned.
func (context *StorageContext) getClient() *http.Client {
    if context.client == nil {
        return http.DefaultClient
    }
    return context.client
}

// Any object that deserializes XML must meet this interface.
type Deserializer interface {
    Deserialize([]byte) error
}

// requestParams is a Parameter Object for performRequest().
type requestParams struct {
    Method         string       // HTTP method, e.g. "GET" or "PUT".
    URL            string       // Resource locator, e.g. "http://example.com/my/resource".
    Body           io.Reader    // Optional request body.
    APIVersion     string       // Expected Azure API version, e.g. "2012-02-12".
    ExtraHeaders   http.Header  // Optional extra request headers.
    Result         Deserializer // Optional object to parse API response into.
    ExpectedStatus HTTPStatus   // Expected response status, e.g. http.StatusOK.
}

// Check performs a basic sanity check on the request.  This will only catch
// a few superficial problems that you can spot at compile time, to save a
// debugging cycle for the most basic mistakes.
func (params *requestParams) Check() {
    const panicPrefix = "invalid request: "
    if params.Method == "" {
        panic(errors.New(panicPrefix + "HTTP method not specified"))
    }
    if params.URL == "" {
        panic(errors.New(panicPrefix + "URL not specified"))
    }
    if params.APIVersion == "" {
        panic(errors.New(panicPrefix + "API version not specified"))
    }
    if params.ExpectedStatus == 0 {
        panic(errors.New(panicPrefix + "expected HTTP status not specified"))
    }
    methods := map[string]bool{"GET": true, "PUT": true, "POST": true, "DELETE": true}
    if _, ok := methods[params.Method]; !ok {
        panic(fmt.Errorf(panicPrefix+"unsupported HTTP method '%s'", params.Method))
    }
}

// performRequest issues an HTTP request to Azure.
//
// It returns the response body contents and the response headers.
func (context *StorageContext) performRequest(params requestParams) ([]byte, http.Header, error) {
    params.Check()
    req, err := http.NewRequest(params.Method, params.URL, params.Body)
    if err != nil {
        return nil, nil, err
    }
    // net/http has no way of adding headers en-masse, hence this abomination.
    for header, values := range params.ExtraHeaders {
        for _, value := range values {
            req.Header.Add(header, value)
        }
    }
    addVersionHeader(req, params.APIVersion)
    addDateHeader(req)
    addContentHeaders(req)
    if err := context.signRequest(req); err != nil {
        return nil, nil, err
    }
    return context.send(req, params.Result, params.ExpectedStatus)
}

// Send a request to the storage service and process the response.
// The "res" parameter is typically an XML struct that will deserialize the
// raw XML into the struct data.  The http Response object's body is returned.
//
// If the response's HTTP status code is not the same as "expectedStatus"
// then an HTTPError will be returned as the error.
func (context *StorageContext) send(req *http.Request, res Deserializer, expectedStatus HTTPStatus) ([]byte, http.Header, error) {
    client := context.getClient()

    retrier := context.RetryPolicy.getRetrier(client)
    resp, err := retrier.RetryRequest(req)
    if err != nil {
        return nil, nil, err
    }

    body, err := readAndClose(resp.Body)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to read response data: %v", err)
    }

    if resp.StatusCode != int(expectedStatus) {
        msg := newHTTPError(resp.StatusCode, body, "Azure request failed")
        return body, resp.Header, msg
    }

    // If the caller didn't supply an object to deserialize the message into
    // then just return.
    if res == nil {
        return body, resp.Header, nil
    }

    // TODO: Also deserialize response headers into the "res" object.
    err = res.Deserialize(body)
    if err != nil {
        msg := fmt.Errorf("Failed to deserialize data: %s", err)
        return body, resp.Header, msg
    }

    return body, resp.Header, nil
}

// getAccountURL returns the base URL for the context's storage account.
// (The result ends in a slash.)
func (context *StorageContext) getAccountURL() string {
    if context.AzureEndpoint == APIEndpoint("") {
        panic(errors.New("no AzureEndpoint specified in gwacl.StorageContext"))
    }
    return context.AzureEndpoint.BlobStorageAPI(context.Account)
}

// getContainerURL returns the URL for a given storage container.
// (The result does not end in a slash.)
func (context *StorageContext) getContainerURL(container string) string {
    return strings.TrimRight(context.getAccountURL(), "/") + "/" + url.QueryEscape(container)
}

// GetFileURL returns the URL for a given file in the given container.
// (The result does not end in a slash.)
func (context *StorageContext) GetFileURL(container, filename string) string {
    return context.getContainerURL(container) + "/" + url.QueryEscape(filename)
}

// GetAnonymousFileURL returns an anonymously-accessible URL for a given file
// in the given container.
func (context *StorageContext) GetAnonymousFileURL(container, filename string, expires time.Time) (string, error) {
    url := context.GetFileURL(container, filename)
    values, err := getReadBlobAccessValues(container, filename, context.Account, context.Key, expires)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s?%s", url, values.Encode()), nil
}

type ListContainersRequest struct {
    Marker string
}

// ListContainers calls the "List Containers" operation on the storage
// API, and returns a single batch of results.
// The marker argument should be empty for a new List Containers request.  for
// subsequent calls to get additional batches of the same result, pass the
// NextMarker from the previous call's result.
func (context *StorageContext) ListContainers(request *ListContainersRequest) (*ContainerEnumerationResults, error) {
    uri := addURLQueryParams(context.getAccountURL(), "comp", "list")
    if request.Marker != "" {
        uri = addURLQueryParams(uri, "marker", request.Marker)
    }
    containers := ContainerEnumerationResults{}
    _, _, err := context.performRequest(requestParams{
        Method:         "GET",
        URL:            uri,
        APIVersion:     "2012-02-12",
        Result:         &containers,
        ExpectedStatus: http.StatusOK,
    })
    if err != nil {
        msg := "request for containers list failed: "
        return nil, extendError(err, msg)
    }
    return &containers, nil
}

type ListBlobsRequest struct {
    Container string
    Marker    string
    Prefix    string
}

// ListBlobs calls the "List Blobs" operation on the storage API, and returns
// a single batch of results.
// The request.Marker argument should be empty for a new List Blobs request.
// For subsequent calls to get additional batches of the same result, pass the
// NextMarker from the previous call's result.
func (context *StorageContext) ListBlobs(request *ListBlobsRequest) (*BlobEnumerationResults, error) {
    uri := addURLQueryParams(
        context.getContainerURL(request.Container),
        "restype", "container",
        "comp", "list")
    if request.Marker != "" {
        uri = addURLQueryParams(uri, "marker", request.Marker)
    }
    if request.Prefix != "" {
        uri = addURLQueryParams(uri, "prefix", request.Prefix)
    }
    blobs := BlobEnumerationResults{}
    _, _, err := context.performRequest(requestParams{
        Method:         "GET",
        URL:            uri,
        APIVersion:     "2012-02-12",
        Result:         &blobs,
        ExpectedStatus: http.StatusOK,
    })
    if err != nil {
        msg := "request for blobs list failed: "
        return nil, extendError(err, msg)
    }
    return &blobs, err
}

// Send a request to the storage service to create a new container.  If the
// request fails, error is non-nil.
func (context *StorageContext) CreateContainer(container string) error {
    uri := addURLQueryParams(
        context.getContainerURL(container),
        "restype", "container")
    _, _, err := context.performRequest(requestParams{
        Method:         "PUT",
        URL:            uri,
        APIVersion:     "2012-02-12",
        ExpectedStatus: http.StatusCreated,
    })
    if err != nil {
        msg := fmt.Sprintf("failed to create container %s: ", container)
        return extendError(err, msg)
    }
    return nil
}

// Send a request to the storage service to delete a container.  It will also
// delete all the blobs inside it.
func (context *StorageContext) DeleteContainer(container string) error {
    uri := addURLQueryParams(
        context.getContainerURL(container),
        "restype", "container")
    _, _, err := context.performRequest(requestParams{
        Method:         "DELETE",
        URL:            uri,
        APIVersion:     "2012-02-12",
        ExpectedStatus: http.StatusAccepted,
    })
    if err != nil {
        // If the error is an Azure 404 error, return silently: the container
        // does not exist.
        if IsNotFoundError(err) {
            return nil
        }
        msg := fmt.Sprintf("failed to delete container %s: ", container)
        return extendError(err, msg)
    }
    return nil
}

// Send a request to the storage service to retrieve a container's properties.
// Also doubles as a handy way to see if a container exists.
func (context *StorageContext) GetContainerProperties(container string) (*Properties, error) {
    uri := addURLQueryParams(
        context.getContainerURL(container),
        "restype", "container")
    params := requestParams{
        Method:         "GET",
        URL:            uri,
        APIVersion:     "2012-02-12",
        ExpectedStatus: http.StatusOK,
    }
    _, headers, err := context.performRequest(params)
    if err != nil {
        msg := fmt.Sprintf("failed to find container %s: ", container)
        return nil, extendError(err, msg)
    }

    props := &Properties{
        LastModified:  headers.Get(http.CanonicalHeaderKey("Last-Modified")),
        ETag:          headers.Get(http.CanonicalHeaderKey("ETag")),
        LeaseStatus:   headers.Get(http.CanonicalHeaderKey("X-Ms-Lease-Status")),
        LeaseState:    headers.Get(http.CanonicalHeaderKey("X-Ms-Lease-State")),
        LeaseDuration: headers.Get(http.CanonicalHeaderKey("X-Ms-Lease-Duration")),
    }

    return props, nil
}

type PutBlobRequest struct {
    Container string // Container name in the storage account
    BlobType  string // Pass "page" or "block"
    Filename  string // Filename for the new blob
    Size      int    // Size for the new blob. Only required for page blobs.
}

// Send a request to create a space to upload a blob.  Note that this does not
// do the uploading, it just makes an empty file.
func (context *StorageContext) PutBlob(req *PutBlobRequest) error {
    var blobType string
    switch req.BlobType {
    case "page":
        blobType = "PageBlob"
        if req.Size == 0 {
            return fmt.Errorf("Must supply a size for a page blob")
        }
        if req.Size%512 != 0 {
            return fmt.Errorf("Size must be a multiple of 512 bytes")
        }
    case "block":
        blobType = "BlockBlob"
    default:
        panic("blockType must be 'page' or 'block'")
    }

    extraHeaders := http.Header{}
    extraHeaders.Add("x-ms-blob-type", blobType)
    if req.BlobType == "page" {
        size := fmt.Sprintf("%d", req.Size)
        extraHeaders.Add("x-ms-blob-content-length", size)
    }

    _, _, err := context.performRequest(requestParams{
        Method:         "PUT",
        URL:            context.GetFileURL(req.Container, req.Filename),
        APIVersion:     "2012-02-12",
        ExtraHeaders:   extraHeaders,
        ExpectedStatus: http.StatusCreated,
    })
    if err != nil {
        msg := fmt.Sprintf("failed to create blob %s: ", req.Filename)
        return extendError(err, msg)
    }
    return nil
}

type PutPageRequest struct {
    Container  string    // Container name in the storage account
    Filename   string    // The blob's file name
    StartRange int       // Must be modulo 512, or an error is returned.
    EndRange   int       // Must be (modulo 512)-1, or an error is returned.
    Data       io.Reader // The data to upload to the page.
}

// Send a request to add a range of data into a page blob.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee691975.aspx
func (context *StorageContext) PutPage(req *PutPageRequest) error {
    validStart := (req.StartRange % 512) == 0
    validEnd := (req.EndRange % 512) == 511
    if !(validStart && validEnd) {
        return fmt.Errorf(
            "StartRange must be a multiple of 512, EndRange must be one less than a multiple of 512")
    }
    uri := addURLQueryParams(
        context.GetFileURL(req.Container, req.Filename),
        "comp", "page")

    extraHeaders := http.Header{}

    rangeData := fmt.Sprintf("bytes=%d-%d", req.StartRange, req.EndRange)
    extraHeaders.Add("x-ms-range", rangeData)
    extraHeaders.Add("x-ms-page-write", "update")

    _, _, err := context.performRequest(requestParams{
        Method:         "PUT",
        URL:            uri,
        Body:           req.Data,
        APIVersion:     "2012-02-12",
        ExtraHeaders:   extraHeaders,
        ExpectedStatus: http.StatusCreated,
    })
    if err != nil {
        msg := fmt.Sprintf("failed to put page for file %s: ", req.Filename)
        return extendError(err, msg)
    }
    return nil
}

// Send a request to fetch the list of blocks that have been uploaded as part
// of a block blob.
func (context *StorageContext) GetBlockList(container, filename string) (*GetBlockList, error) {
    uri := addURLQueryParams(
        context.GetFileURL(container, filename),
        "comp", "blocklist",
        "blocklisttype", "all")
    bl := GetBlockList{}
    _, _, err := context.performRequest(requestParams{
        Method:         "GET",
        URL:            uri,
        APIVersion:     "2012-02-12",
        Result:         &bl,
        ExpectedStatus: http.StatusOK,
    })
    if err != nil {
        msg := fmt.Sprintf("request for block list in file %s failed: ", filename)
        return nil, extendError(err, msg)
    }
    return &bl, nil
}

// Send a request to create a new block.  The request payload contains the
// data block to upload.
func (context *StorageContext) PutBlock(container, filename, id string, data io.Reader) error {
    base64ID := base64.StdEncoding.EncodeToString([]byte(id))
    uri := addURLQueryParams(
        context.GetFileURL(container, filename),
        "comp", "block",
        "blockid", base64ID)
    _, _, err := context.performRequest(requestParams{
        Method:         "PUT",
        URL:            uri,
        Body:           data,
        APIVersion:     "2012-02-12",
        ExpectedStatus: http.StatusCreated,
    })
    if err != nil {
        msg := fmt.Sprintf("failed to put block %s for file %s: ", id, filename)
        return extendError(err, msg)
    }
    return nil
}

// Send a request to piece together blocks into a list that specifies a blob.
func (context *StorageContext) PutBlockList(container, filename string, blocklist *BlockList) error {
    uri := addURLQueryParams(
        context.GetFileURL(container, filename),
        "comp", "blocklist")
    data, err := blocklist.Serialize()
    if err != nil {
        return err
    }
    dataReader := bytes.NewReader(data)

    _, _, err = context.performRequest(requestParams{
        Method:         "PUT",
        URL:            uri,
        Body:           dataReader,
        APIVersion:     "2012-02-12",
        ExpectedStatus: http.StatusCreated,
    })
    if err != nil {
        msg := fmt.Sprintf("failed to put blocklist for file %s: ", filename)
        return extendError(err, msg)
    }
    return nil
}

// Delete the specified blob from the given container.  Deleting a non-existant
// blob will return without an error.
func (context *StorageContext) DeleteBlob(container, filename string) error {
    _, _, err := context.performRequest(requestParams{
        Method:         "DELETE",
        URL:            context.GetFileURL(container, filename),
        APIVersion:     "2012-02-12",
        ExpectedStatus: http.StatusAccepted,
    })
    if err != nil {
        // If the error is an Azure 404 error, return silently: the blob does
        // not exist.
        if IsNotFoundError(err) {
            return nil
        }
        msg := fmt.Sprintf("failed to delete blob %s: ", filename)
        return extendError(err, msg)
    }
    return nil
}

// Get the specified blob from the given container.
func (context *StorageContext) GetBlob(container, filename string) (io.ReadCloser, error) {
    body, _, err := context.performRequest(requestParams{
        Method:         "GET",
        URL:            context.GetFileURL(container, filename),
        APIVersion:     "2012-02-12",
        ExpectedStatus: http.StatusOK,
    })
    if err != nil {
        msg := fmt.Sprintf("failed to get blob %q: ", filename)
        return nil, extendError(err, msg)
    }
    return ioutil.NopCloser(bytes.NewBuffer(body)), nil
}

type SetContainerACLRequest struct {
    Container string // Container name in the storage account
    Access    string // "container", "blob", or "private"
}

// SetContainerACL sets the specified container's access rights.
// See http://msdn.microsoft.com/en-us/library/windowsazure/dd179391.aspx
func (context *StorageContext) SetContainerACL(req *SetContainerACLRequest) error {
    uri := addURLQueryParams(
        context.getContainerURL(req.Container),
        "restype", "container",
        "comp", "acl")

    extraHeaders := http.Header{}
    switch req.Access {
    case "container", "blob":
        extraHeaders.Add("x-ms-blob-public-access", req.Access)
    case "private":
        // Don't add a header, Azure resets to private if it's omitted.
    default:
        panic("Access must be one of 'container', 'blob' or 'private'")
    }

    _, _, err := context.performRequest(requestParams{
        Method:         "PUT",
        URL:            uri,
        APIVersion:     "2009-09-19",
        ExtraHeaders:   extraHeaders,
        ExpectedStatus: http.StatusOK,
    })

    if err != nil {
        msg := fmt.Sprintf("failed to set ACL for container %s: ", req.Container)
        return extendError(err, msg)
    }
    return nil
}
