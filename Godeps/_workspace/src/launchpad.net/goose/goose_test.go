package goose

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type GooseTestSuite struct {
}

var _ = Suite(&GooseTestSuite{})
