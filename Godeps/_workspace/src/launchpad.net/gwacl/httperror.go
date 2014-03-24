package gwacl

import (
    "encoding/xml"
    "errors"
    "fmt"
    "net/http"
)

// HTTPError is an extended version of the standard "error" interface.  It
// adds an HTTP status code.
type HTTPError interface {
    error
    StatusCode() int
}

// HTTPStatus is an HTTP status code.
type HTTPStatus int

// Status returns the HTTP status code as an int.
func (s HTTPStatus) StatusCode() int {
    return int(s)
}

// AzureError is an HTTPError returned by the Azure API.  It contains an
// error message, and an Azure-defined error code.
type AzureError struct {
    error      `xml:"-"`
    HTTPStatus `xml:"-"`
    Code       string `xml:"Code"`
    Message    string `xml:"Message"`
}

// *AzureError implements HTTPError.
var _ HTTPError = &AzureError{}

func (e *AzureError) Error() string {
    description := e.error.Error()
    status := e.StatusCode()
    name := http.StatusText(status)
    return fmt.Sprintf("%s: %s - %s (http code %d: %s)", description, e.Code, e.Message, status, name)
}

// ServerError is a generic HTTPError, without any further helpful information
// from the server that we can count on.
type ServerError struct {
    error
    HTTPStatus
}

// *ServerError implements HTTPError.
var _ HTTPError = &ServerError{}

func (e *ServerError) Error() string {
    description := e.error.Error()
    status := e.StatusCode()
    name := http.StatusText(status)
    return fmt.Sprintf("%s (%d: %s)", description, status, name)
}

// newHTTPError returns the appropriate HTTPError implementation for a given
// HTTP response.
// It takes a status code and response body, rather than just a standard
// http.Response object.
func newHTTPError(status int, body []byte, description string) HTTPError {
    httpStatus := HTTPStatus(status)
    baseErr := errors.New(description)
    azureError := AzureError{error: baseErr, HTTPStatus: httpStatus}
    err := xml.Unmarshal(body, &azureError)
    if err != nil {
        // It's OK if the response body wasn't actually XML...  That just means
        // it wasn't a proper AzureError.  We have another error type for that.
        return &ServerError{error: baseErr, HTTPStatus: httpStatus}
    }
    return &azureError
}

// newAzureErrorFromOperation composes an HTTPError based on an Operation
// struct, i.e. the result of an asynchronous operation.
func newAzureErrorFromOperation(outcome *Operation) *AzureError {
    if outcome.Status != FailedOperationStatus {
        msg := fmt.Errorf("interpreting Azure %s as an asynchronous failure", outcome.Status)
        panic(msg)
    }
    return &AzureError{
        error:      errors.New("asynchronous operation failed"),
        HTTPStatus: HTTPStatus(outcome.HTTPStatusCode),
        Code:       outcome.ErrorCode,
        Message:    outcome.ErrorMessage,
    }
}

// extendError returns an error whos description is the concatenation of
// the given message plus the error string from the original error.
// It preserves the value of the error types it knows about (currently only
// ServerError).
//
// The main purpose of this method is to offer a unified way to
// extend the information present in errors while still not losing the
// additioning information present on specific errors gwacl knows out to extend
// in a more meaningful way.
func extendError(err error, message string) error {
    switch err := err.(type) {
    case *ServerError:
        extendedError := *err
        extendedError.error = fmt.Errorf(message+"%v", err.error)
        return &extendedError
    case *AzureError:
        extendedError := *err
        extendedError.error = fmt.Errorf(message+"%v", err.error)
        return &extendedError
    default:
        return fmt.Errorf(message+"%v", err)
    }
    // This code cannot be reached but Go insists on having a return or a panic
    // statement at the end of this method.  Sigh.
    panic("invalid extendError state!")
}

// IsNotFoundError returns whether or not the given error is an error (as
// returned by a gwacl method) which corresponds to a 'Not Found' error
// returned by Windows Azure.
func IsNotFoundError(err error) bool {
    httpError, ok := err.(HTTPError)
    if ok {
        return httpError.StatusCode() == http.StatusNotFound
    }
    return false
}
