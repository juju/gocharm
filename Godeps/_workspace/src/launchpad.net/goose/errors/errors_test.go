package errors_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/errors"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ErrorsSuite struct {
}

var _ = Suite(&ErrorsSuite{})

func (s *ErrorsSuite) TestCreateSimpleNotFoundError(c *C) {
	context := "context"
	err := errors.NewNotFoundf(nil, context, "")
	c.Assert(errors.IsNotFound(err), Equals, true)
	c.Assert(err.Error(), Equals, "Not found: context")
}

func (s *ErrorsSuite) TestCreateNotFoundError(c *C) {
	context := "context"
	err := errors.NewNotFoundf(nil, context, "It was not found: %s", context)
	c.Assert(errors.IsNotFound(err), Equals, true)
	c.Assert(err.Error(), Equals, "It was not found: context")
}

func (s *ErrorsSuite) TestCreateSimpleDuplicateValueError(c *C) {
	context := "context"
	err := errors.NewDuplicateValuef(nil, context, "")
	c.Assert(errors.IsDuplicateValue(err), Equals, true)
	c.Assert(err.Error(), Equals, "Duplicate: context")
}

func (s *ErrorsSuite) TestCreateDuplicateValueError(c *C) {
	context := "context"
	err := errors.NewDuplicateValuef(nil, context, "It was duplicate: %s", context)
	c.Assert(errors.IsDuplicateValue(err), Equals, true)
	c.Assert(err.Error(), Equals, "It was duplicate: context")
}

func (s *ErrorsSuite) TestCreateSimpleUnauthorisedfError(c *C) {
	context := "context"
	err := errors.NewUnauthorisedf(nil, context, "")
	c.Assert(errors.IsUnauthorised(err), Equals, true)
	c.Assert(err.Error(), Equals, "Unauthorised: context")
}

func (s *ErrorsSuite) TestCreateUnauthorisedfError(c *C) {
	context := "context"
	err := errors.NewUnauthorisedf(nil, context, "It was unauthorised: %s", context)
	c.Assert(errors.IsUnauthorised(err), Equals, true)
	c.Assert(err.Error(), Equals, "It was unauthorised: context")
}

func (s *ErrorsSuite) TestErrorCause(c *C) {
	rootCause := errors.NewNotFoundf(nil, "some value", "")
	// Construct a new error, based on a not found root cause.
	err := errors.Newf(rootCause, "an error occurred")
	c.Assert(err.Cause(), Equals, rootCause)
	// Check the other error attributes.
	c.Assert(err.Error(), Equals, "an error occurred\ncaused by: Not found: some value")
}

func (s *ErrorsSuite) TestErrorIsType(c *C) {
	rootCause := errors.NewNotFoundf(nil, "some value", "")
	// Construct a new error, based on a not found root cause.
	err := errors.Newf(rootCause, "an error occurred")
	// Check that the error is not falsely identified as something it is not.
	c.Assert(errors.IsDuplicateValue(err), Equals, false)
	// Check that the error is correctly identified as a not found error.
	c.Assert(errors.IsNotFound(err), Equals, true)
}
