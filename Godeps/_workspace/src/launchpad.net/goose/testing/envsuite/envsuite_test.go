package envsuite

import (
	. "launchpad.net/gocheck"
	"os"
	"testing"
)

type EnvTestSuite struct {
	EnvSuite
}

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&EnvTestSuite{})

func (s *EnvTestSuite) TestGrabsCurrentEnvironment(c *C) {
	envsuite := &EnvSuite{}
	// EnvTestSuite is an EnvSuite, so we should have already isolated
	// ourselves from the world. So we set a single env value, and we
	// assert that SetUpSuite is able to see that.
	os.Setenv("TEST_KEY", "test-value")
	envsuite.SetUpSuite(c)
	c.Assert(envsuite.environ, DeepEquals, []string{"TEST_KEY=test-value"})
}

func (s *EnvTestSuite) TestClearsEnvironment(c *C) {
	envsuite := &EnvSuite{}
	os.Setenv("TEST_KEY", "test-value")
	envsuite.SetUpSuite(c)
	// SetUpTest should reset the current environment back to being
	// completely empty.
	envsuite.SetUpTest(c)
	c.Assert(os.Getenv("TEST_KEY"), Equals, "")
	c.Assert(os.Environ(), DeepEquals, []string{})
}

func (s *EnvTestSuite) TestRestoresEnvironment(c *C) {
	envsuite := &EnvSuite{}
	os.Setenv("TEST_KEY", "test-value")
	envsuite.SetUpSuite(c)
	envsuite.SetUpTest(c)
	envsuite.TearDownTest(c)
	c.Assert(os.Getenv("TEST_KEY"), Equals, "test-value")
	c.Assert(os.Environ(), DeepEquals, []string{"TEST_KEY=test-value"})
}
