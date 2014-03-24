// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

// A poller object used to delete a disk.
//
// It takes an indeterminate time for a disk previously attached to a
// deleted VM to become "not in use" and thus be available for deletion.
// When we receive the "disk is still attached" error, we try again every
// 10 seconds until it succeeds, with a timeout of 30 minutes).
// This bug might be related to:
// http://social.msdn.microsoft.com/Forums/en-US/WAVirtualMachinesforWindows/thread/4394c75d-59ff-4634-8212-2ad71bf6fbd5/
//
// Once this bug is fixed in Windows Azure, this file and the related tests
// can safely be removed, and ManagementAPI._DeleteDisk() can replace the
// current implementation of ManagementAPI.DeleteDisk() (which uses this
// poller).

package gwacl

import (
    "fmt"
    "regexp"
    "time"
)

var deleteDiskTimeout = 30 * time.Minute
var deleteDiskInterval = 10 * time.Second

type diskDeletePoller struct {
    api        *ManagementAPI
    diskName   string
    deleteBlob bool
}

var _ poller = &diskDeletePoller{}

func (poller diskDeletePoller) poll() (*x509Response, error) {
    return nil, poller.api._DeleteDisk(poller.diskName, poller.deleteBlob)
}

// isInUseError returns whether or not the given string is of the "disk in use"
// type.
// Here is a real-world example of the error in question:
// "BadRequest - A disk with name gwacldiske5w7lkj is currently in use
// by virtual machine gwaclrolemvo1yab running within hosted service
// gwacl623yosxtppsa9577xy5, deployment gwaclmachinewes4n64f. (http
// code 400: Bad Request)"
func isInUseError(errString string, diskName string) bool {
    pattern := fmt.Sprintf("BadRequest - A disk with name %s is currently in use by virtual machine.*", regexp.QuoteMeta(diskName))
    reg := regexp.MustCompile(pattern)
    return reg.MatchString(errString)
}

func (poller diskDeletePoller) isDone(response *x509Response, pollerErr error) (bool, error) {
    if pollerErr == nil {
        return true, nil
    }
    if isInUseError(pollerErr.Error(), poller.diskName) {
        // The error is of the "disk in use" type: continue polling.
        return false, nil
    }
    // The error is *not* of the "disk in use" type: stop polling and return
    // the error.
    return true, pollerErr
}
