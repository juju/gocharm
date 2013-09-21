package jsonreflect_test
import (
	"fmt"
	"testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-utils/jsonreflect"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type typeSuite struct{}

var _ = gc.Suite(typeSuite{})

func (typeSuite) TestSimpleType(c *gc.C) {
	for i, kind := range []jsonreflect.Kind{jsonreflect.String, jsonreflect.Number, jsonreflect.Bool} {
		c.Logf("test %d: %v", i, kind)
		t := jsonreflect.SimpleType(kind)
		c.Assert(t.Kind(), gc.Equals, kind)
		c.Assert(func() {t.Elem()}, gc.PanicMatches, "Elem called on " + kind.String())
		c.Assert(func() {t.Name()}, gc.PanicMatches, "Name called on " + kind.String())
		c.Assert(func() {t.FieldNames()}, gc.PanicMatches, "FieldNames called on " + kind.String())
		c.Assert(func() {t.Field("foo")}, gc.PanicMatches, "Field called on " + kind.String())
		c.Assert(t.String(), gc.Equals, fmt.Sprintf("%q", kind.String()))
	}
}

func (typeSuite) TestCustomType(c *gc.C) {
	c.Assert(func() {jsonreflect.CustomType("")}, gc.PanicMatches, "CustomType called with empty name")
	t := jsonreflect.CustomType("foo")
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Custom)
	c.Assert(func() {t.Elem()}, gc.PanicMatches, "Elem called on custom")
	c.Assert(t.Name(), gc.Equals, "foo")
	c.Assert(func() {t.FieldNames()}, gc.PanicMatches, "FieldNames called on custom")
	c.Assert(func() {t.Field("foo")}, gc.PanicMatches, "Field called on custom")
}