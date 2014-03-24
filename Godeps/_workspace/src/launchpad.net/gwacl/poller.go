// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    "time"
)

// Generic poller interface/methods.

// A poller exposes two methods to query a remote server and decide when
// the response given by the server means that the polling is finished.
type poller interface {
    poll() (*x509Response, error)
    isDone(*x509Response, error) (bool, error)
}

// performPolling calls the poll() method of the given 'poller' object every
// 'interval' until poller.isDone() returns true.
func performPolling(poller poller, interval time.Duration, timeout time.Duration) (*x509Response, error) {
    timeoutChannel := time.After(timeout)
    ticker := time.Tick(interval)
    // Function to do a single poll, checking for timeout. The bool returned
    // indicates if polling is finished, one way or another.
    poll := func() (bool, *x509Response, error) {
        // This may need to tolerate some transient failures, such as network
        // failures that may go away after a few retries.
        select {
        case <-timeoutChannel:
            return true, nil, fmt.Errorf("polling timed out waiting for an asynchronous operation")
        default:
            response, pollerErr := poller.poll()
            done, err := poller.isDone(response, pollerErr)
            if err != nil {
                return true, nil, err
            }
            if done {
                return true, response, nil
            }
        }
        return false, nil, nil
    }
    // Do an initial poll.
    done, response, err := poll()
    if done {
        return response, err
    }
    // Poll every interval.
    for _ = range ticker {
        done, response, err := poll()
        if done {
            return response, err
        }
    }
    // This code cannot be reached but Go insists on having a return or a panic
    // statement at the end of this method.  Sigh.
    panic("invalid poller state!")
}

// Operation poller structs/methods.

// performOperationPolling calls performPolling on the given arguments and converts
// the returned object into an *Operation.
func performOperationPolling(poller poller, interval time.Duration, timeout time.Duration) (*Operation, error) {
    response, err := performPolling(poller, interval, timeout)
    if err != nil {
        return nil, err
    }
    operation := Operation{}
    err = operation.Deserialize(response.Body)
    return &operation, err
}

// operationPoller is an object implementing the poller interface, used to
// poll the Window Azure server until the operation referenced by the given
// operationID is completed.
type operationPoller struct {
    api         *ManagementAPI
    operationID string
}

var _ poller = &operationPoller{}

// newOperationPoller returns a poller object associated with the given
// management API object and the given operationID.  It can track (by polling
// the server) the status of the operation associated with the provided
// operationID string.
func newOperationPoller(api *ManagementAPI, operationID string) poller {
    return operationPoller{api: api, operationID: operationID}
}

// Poll issues a blocking request to microsoft Azure to fetch the information
// related to the operation associated with the poller.
// See http://msdn.microsoft.com/en-us/library/windowsazure/ee460783.aspx
func (poller operationPoller) poll() (*x509Response, error) {
    URI := "operations/" + poller.operationID
    return poller.api.session.get(URI, "2009-10-01")
}

// IsDone returns true if the given response has a status code indicating
// success and if the returned XML response corresponds to a valid Operation
// with a status indicating that the operation is completed.
func (poller operationPoller) isDone(response *x509Response, pollerError error) (bool, error) {
    // TODO: Add a timeout so that polling won't continue forever if the
    // server cannot be reached.
    if pollerError != nil {
        return true, pollerError
    }
    if response.StatusCode >= 200 && response.StatusCode < 300 {
        operation := Operation{}
        err := operation.Deserialize(response.Body)
        if err != nil {
            return false, err
        }
        status := operation.Status
        done := (status != "" && status != InProgressOperationStatus)
        return done, nil
    }
    return false, nil
}
