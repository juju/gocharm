package gwacl

import (
    "errors"
    "fmt"
    . "launchpad.net/gocheck"
    "net/http"
)

type httpErrorSuite struct{}

var _ = Suite(&httpErrorSuite{})

func (suite *httpErrorSuite) TestNewHTTPErrorParsesAzureError(c *C) {
    description := "upload failed"
    status := 415
    code := "CannotUpload"
    message := "Unknown data format"
    xml := fmt.Sprintf(`<Error>
        <Code>%s</Code>
        <Message>%s</Message>
    </Error>`,
        code, message)

    httpErr := newHTTPError(status, []byte(xml), description)

    azureErr, ok := httpErr.(*AzureError)
    c.Assert(ok, Equals, true)
    c.Check(azureErr.StatusCode(), Equals, status)
    c.Check(azureErr.Code, Equals, code)
    c.Check(azureErr.Message, Equals, message)
    c.Check(httpErr, ErrorMatches, ".*"+description+".*")
    c.Check(httpErr, ErrorMatches, ".*415: Unsupported Media Type.*")
}

func (suite *httpErrorSuite) TestNewHTTPErrorResortsToServerError(c *C) {
    description := "could not talk to server"
    status := 505

    httpErr := newHTTPError(status, []byte{}, description)

    _, ok := httpErr.(*ServerError)
    c.Assert(ok, Equals, true)
    c.Check(httpErr.StatusCode(), Equals, status)
    c.Check(httpErr, ErrorMatches, ".*505: HTTP Version Not Supported.*")
    c.Check(httpErr, ErrorMatches, ".*"+description+".*")
}

func (suite *httpErrorSuite) TestAzureErrorComposesError(c *C) {
    description := "something failed"
    status := 410
    httpErr := AzureError{
        error:      errors.New(description),
        HTTPStatus: HTTPStatus(status),
        Code:       "MissingError",
        Message:    "Your object has disappeared",
    }
    c.Check(httpErr.Error(), Equals, "something failed: MissingError - Your object has disappeared (http code 410: Gone)")
}

func (suite *httpErrorSuite) TestServerErrorComposesError(c *C) {
    description := "something failed"
    status := 501
    httpErr := ServerError{
        error:      errors.New(description),
        HTTPStatus: HTTPStatus(status),
    }
    c.Check(httpErr.Error(), Equals, "something failed (501: Not Implemented)")
}

func (suite *httpErrorSuite) TestNewAzureErrorFromOperation(c *C) {
    status := http.StatusConflict
    code := MakeRandomString(7)
    message := MakeRandomString(20)
    // Test body copied from Azure documentation, mutatis mutandi.
    body := fmt.Sprintf(`
        <?xml version="1.0" encoding="utf-8"?>
          <Operation xmlns="http://schemas.microsoft.com/windowsazure">
            <ID>%s</ID>
            <Status>Failed</Status>
            <HttpStatusCode>%d</HttpStatusCode>
            <Error>
              <Code>%s</Code>
              <Message>%s</Message>
            </Error>
          </Operation>
        `,
        MakeRandomString(5), status, code, message)
    operation := Operation{}
    operation.Deserialize([]byte(body))

    err := newAzureErrorFromOperation(&operation)
    c.Check(err.HTTPStatus, Equals, HTTPStatus(status))
    c.Check(err.Code, Equals, code)
    c.Check(err.Message, Equals, message)
}

func (suite *httpErrorSuite) TestExtendErrorExtendsGenericError(c *C) {
    errorString := "an-error"
    error := fmt.Errorf(errorString)
    additionalErrorMsg := "additional message"
    newError := extendError(error, additionalErrorMsg)
    c.Check(newError.Error(), Equals, fmt.Sprintf("%s%s", additionalErrorMsg, error.Error()))
}

func (suite *httpErrorSuite) TestExtendErrorExtendsServerError(c *C) {
    err := &ServerError{
        error:      errors.New("could not talk to server"),
        HTTPStatus: HTTPStatus(http.StatusGatewayTimeout),
    }
    additionalErrorMsg := "additional message: "
    newError := extendError(err, additionalErrorMsg)
    newServerError, ok := newError.(*ServerError)
    c.Assert(ok, Equals, true)
    c.Check(newError.Error(), Equals, additionalErrorMsg+err.Error())
    c.Check(newServerError.HTTPStatus, Equals, err.HTTPStatus)
}

func (suite *httpErrorSuite) TestExtendErrorExtendsAzureError(c *C) {
    err := &AzureError{
        error:      errors.New("could not talk to server"),
        HTTPStatus: HTTPStatus(http.StatusGatewayTimeout),
    }
    additionalErrorMsg := "additional message: "
    newError := extendError(err, additionalErrorMsg)
    newAzureError, ok := newError.(*AzureError)
    c.Assert(ok, Equals, true)
    c.Check(newError.Error(), Equals, additionalErrorMsg+err.Error())
    c.Check(newAzureError.HTTPStatus, Equals, err.HTTPStatus)
}

func (suite *httpErrorSuite) TestIsNotFound(c *C) {
    var testValues = []struct {
        err            error
        expectedResult bool
    }{
        {fmt.Errorf("generic error"), false},
        {&AzureError{HTTPStatus: HTTPStatus(http.StatusOK)}, false},
        {&AzureError{HTTPStatus: HTTPStatus(http.StatusNotFound)}, true},
        {&AzureError{HTTPStatus: HTTPStatus(http.StatusInternalServerError)}, false},
        {&ServerError{HTTPStatus: HTTPStatus(http.StatusOK)}, false},
        {&ServerError{HTTPStatus: HTTPStatus(http.StatusNotFound)}, true},
        {&ServerError{HTTPStatus: HTTPStatus(http.StatusInternalServerError)}, false},
    }
    for _, test := range testValues {
        c.Check(IsNotFoundError(test.err), Equals, test.expectedResult)
    }
}
