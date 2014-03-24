// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

// This file contains higher level operations necessary to work with the Azure
// file storage API.

import (
    "bytes"
    "encoding/base64"
    "fmt"
    "io"
    . "launchpad.net/gwacl/logging"
    "strconv"
    "strings"
)

// UploadBlockBlob uses PutBlock and PutBlockList API operations to upload
// arbitrarily large files, 1MB at a time.
func (context *StorageContext) UploadBlockBlob(
    container, filename string, data io.Reader) error {

    buffer := make([]byte, 1024*1024) // 1MB buffer
    blockList := &BlockList{}

    // Upload the file in chunks.
    for blockNum := int64(0); ; blockNum++ {
        blockSize, err := data.Read(buffer)
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        block := bytes.NewReader(buffer[:blockSize])
        blockID := strconv.FormatInt(blockNum, 36)
        // Block IDs must be a consistent length, so pad it out.
        blockID = fmt.Sprintf("%030s", blockID)
        Debugf("Uploading block %d (size=%d, id=%s).\n",
            blockNum, blockSize, blockID)
        err = context.PutBlock(container, filename, blockID, block)
        if err != nil {
            return err
        }
        blockList.Add(BlockListLatest, blockID)
    }

    // Commit those blocks by writing the block list.
    Debugf("Committing %d blocks.\n", len(blockList.Items))
    return context.PutBlockList(container, filename, blockList)
}

// ListAllBlobs requests from the API a list of blobs in a container.
func (context *StorageContext) ListAllBlobs(request *ListBlobsRequest) (*BlobEnumerationResults, error) {
    blobs := make([]Blob, 0)
    var batch *BlobEnumerationResults

    // Request the initial result, using the empty marker.  Then, for as long
    // as the result has a nonempty NextMarker, request the next batch using
    // that marker.
    // This loop is very similar to the one in ListAllContainers().
    for marker, nextMarker := "", "x"; nextMarker != ""; marker = nextMarker {
        var err error
        // Don't use := here or you'll shadow variables from the function's
        // outer scopes.
        request.Marker = marker
        batch, err = context.ListBlobs(request)
        if err != nil {
            return nil, err
        }
        // The response may contain a NextMarker field, to let us request a
        // subsequent batch of results.  The XML parser won't trim whitespace out
        // of the marker tag, so we do that here.
        nextMarker = strings.TrimSpace(batch.NextMarker)
        blobs = append(blobs, batch.Blobs...)
    }

    // There's more in a BlobsEnumerationResults than just the blobs.
    // Return the latest batch, but give it the full cumulative blobs list
    // instead of just the last batch.
    // To the caller, this will look like they made one call to Azure's
    // List Blobs method, but batch size was unlimited.
    batch.Blobs = blobs
    return batch, nil
}

type DeleteAllBlobsRequest struct {
    Container string
    // Other params possible, add later.
}

// RemoveAllBlobs requests a deletion of all the blobs in a container.
// The blobs are not deleted immediately, so when this call returns they
// may still be present for a while.
func (context *StorageContext) DeleteAllBlobs(request *DeleteAllBlobsRequest) error {
    blobs, err := context.ListAllBlobs(&ListBlobsRequest{
        Container: request.Container})
    if err != nil {
        return err
    }

    for _, blob := range blobs.Blobs {
        err := context.DeleteBlob(request.Container, blob.Name)
        if err != nil {
            return err
        }
    }

    return nil
}

// ListAllContainers requests from the storage service a list of containers
// in the storage account.
func (context *StorageContext) ListAllContainers() (*ContainerEnumerationResults, error) {
    containers := make([]Container, 0)
    var batch *ContainerEnumerationResults

    // Request the initial result, using the empty marker.  Then, for as long
    // as the result has a nonempty NextMarker, request the next batch using
    // that marker.
    for marker, nextMarker := "", "x"; nextMarker != ""; marker = nextMarker {
        var err error
        // Don't use := here or you'll shadow variables from the function's
        // outer scopes.
        request := &ListContainersRequest{Marker: marker}
        batch, err = context.ListContainers(request)
        if err != nil {
            return nil, err
        }
        // The response may contain a NextMarker field, to let us request a
        // subsequent batch of results.  The XML parser won't trim whitespace out
        // of the marker tag, so we do that here.
        nextMarker = strings.TrimSpace(batch.NextMarker)
        containers = append(containers, batch.Containers...)
    }

    // There's more in a ContainerEnumerationResults than just the containers.
    // Return the latest batch, but give it the full cumulative containers list
    // instead of just the last batch.
    // To the caller, this will look like they made one call to Azure's
    // List Containers method, but batch size was unlimited.
    batch.Containers = containers
    return batch, nil
}

type CreateVHDRequest struct {
    Container      string    // Container name in the storage account
    Filename       string    // Specify the filename in which to store the VHD
    FilesystemData io.Reader // A formatted filesystem, e.g. iso9660.
    Size           int       // How many bytes from the Filesystem data to upload.  *Must* be a multiple of 512.
}

// CreateInstanceDataVHD will take the supplied filesystem data and create an
// Azure VHD in a page blob out of it.  The data cannot be bigger than
// gwacl.VHD_SIZE-512.  This is intended to be used as a way of passing
// arbitrary data to a new instance - create a disk here and then attach it to
// the new instance.
func (context *StorageContext) CreateInstanceDataVHD(req *CreateVHDRequest) error {
    // We need several steps:
    // 1. Create an empty page blob of exactly VHD_SIZE bytes (see
    // vhd_footer.go)
    // 2. Upload VHD_FOOTER to the last page of the blob.
    // 3. Upload the supplied FilesystemData from the start of the blob.

    var err error

    if req.Size%512 != 0 {
        return fmt.Errorf("Size must be a multiple of 512")
    }
    if req.Size > VHD_SIZE-512 {
        // Protect against writing over the VHD footer.
        return fmt.Errorf("Size cannot be bigger than %d", VHD_SIZE-512)
    }

    // Step 1.
    err = context.PutBlob(&PutBlobRequest{
        Container: req.Container,
        BlobType:  "page",
        Filename:  req.Filename,
        Size:      VHD_SIZE,
    })

    if err != nil {
        return err
    }

    // Step 2.
    data, err := base64.StdEncoding.DecodeString(VHD_FOOTER)
    if err != nil {
        // This really shouldn't ever happen since there's a test to make sure
        // it can be decoded.
        panic(err)
    }
    dataReader := bytes.NewReader(data)
    err = context.PutPage(&PutPageRequest{
        Container:  req.Container,
        Filename:   req.Filename,
        StartRange: VHD_SIZE - 512, // last page of the blob
        EndRange:   VHD_SIZE - 1,
        Data:       dataReader,
    })

    if err != nil {
        return err
    }

    // Step 3.
    err = context.PutPage(&PutPageRequest{
        Container:  req.Container,
        Filename:   req.Filename,
        StartRange: 0,
        EndRange:   req.Size - 1,
        Data:       req.FilesystemData,
    })

    if err != nil {
        return err
    }

    return nil
}
