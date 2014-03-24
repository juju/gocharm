package hook

import (
	"fmt"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&ServiceSuite{})

type ServiceSuite struct {
	ts *testService
}

func (s *ServiceSuite) SetUpTest(c *C) {
	s.ts = newTestService()
	// This hook is called based on the function name.
	s.ts.RegisterControlPoint("foo", functionControlHook)
	// This hook is called based on a user specified hook name.
	s.ts.RegisterControlPoint("foobar", namedControlHook)
}

type testService struct {
	TestService
	label string
}

func newTestService() *testService {
	return &testService{
		TestService: TestService{
			ControlHooks: make(map[string]ControlProcessor),
		},
	}
}

func functionControlHook(s ServiceControl, args ...interface{}) error {
	label := args[0].(string)
	returnError := args[1].(bool)
	if returnError {
		return fmt.Errorf("An error occurred")
	}
	s.(*testService).label = label
	return nil
}

func namedControlHook(s ServiceControl, args ...interface{}) error {
	s.(*testService).label = "foobar"
	return nil
}

func (s *testService) foo(label string, returnError bool) error {
	if err := s.ProcessFunctionHook(s, label, returnError); err != nil {
		return err
	}
	return nil
}

func (s *testService) bar() error {
	if err := s.ProcessControlHook("foobar", s); err != nil {
		return err
	}
	return nil
}

func (s *ServiceSuite) TestFunctionHookNoError(c *C) {
	err := s.ts.foo("success", false)
	c.Assert(err, IsNil)
	c.Assert(s.ts.label, Equals, "success")
}

func (s *ServiceSuite) TestHookWithError(c *C) {
	err := s.ts.foo("success", true)
	c.Assert(err, Not(IsNil))
	c.Assert(s.ts.label, Equals, "")
}

func (s *ServiceSuite) TestNamedHook(c *C) {
	err := s.ts.bar()
	c.Assert(err, IsNil)
	c.Assert(s.ts.label, Equals, "foobar")
}

func (s *ServiceSuite) TestHookCleanup(c *C) {
	// Manually delete the existing control point.
	s.ts.RegisterControlPoint("foo", nil)
	// Register a new hook and ensure it works.
	cleanup := s.ts.RegisterControlPoint("foo", functionControlHook)
	err := s.ts.foo("cleanuptest", false)
	c.Assert(err, IsNil)
	c.Assert(s.ts.label, Equals, "cleanuptest")
	// Use the cleanup func to remove the hook and check the result.
	cleanup()
	err = s.ts.foo("again", false)
	c.Assert(err, IsNil)
	c.Assert(s.ts.label, Equals, "cleanuptest")
	// Ensure that only the specified hook was removed and the other remaining one still works.
	err = s.ts.bar()
	c.Assert(err, IsNil)
	c.Assert(s.ts.label, Equals, "foobar")

}
