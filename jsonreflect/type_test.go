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
	c.Assert(t.String(), gc.Equals, `"foo"`)
}

func (typeSuite) TestArrayOf(c *gc.C) {
	elem := jsonreflect.SimpleType(jsonreflect.Number)
	t := jsonreflect.ArrayOf(elem)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Array)
	c.Assert(t.Elem(), gc.Equals, elem)
	c.Assert(func() {t.Name()}, gc.PanicMatches, "Name called on array")
	c.Assert(func() {t.FieldNames()}, gc.PanicMatches, "FieldNames called on array")
	c.Assert(func() {t.Field("foo")}, gc.PanicMatches, "Field called on array")
	c.Assert(t.String(), gc.Equals, `["number"]`)
}

func (typeSuite) TestMapOf(c *gc.C) {
	elem := jsonreflect.SimpleType(jsonreflect.Number)
	t := jsonreflect.MapOf(elem)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Map)
	c.Assert(t.Elem(), gc.Equals, elem)
	c.Assert(func() {t.Name()}, gc.PanicMatches, "Name called on map")
	c.Assert(func() {t.FieldNames()}, gc.PanicMatches, "FieldNames called on map")
	c.Assert(func() {t.Field("foo")}, gc.PanicMatches, "Field called on map")
	c.Assert(t.String(), gc.Equals, `{"_map":"number"}`)
}

func (typeSuite) TestNullableOf(c *gc.C) {
	elem := jsonreflect.SimpleType(jsonreflect.Number)
	t := jsonreflect.NullableOf(elem)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Nullable)
	c.Assert(t.Elem(), gc.Equals, elem)
	c.Assert(func() {t.Name()}, gc.PanicMatches, "Name called on nullable")
	c.Assert(func() {t.FieldNames()}, gc.PanicMatches, "FieldNames called on nullable")
	c.Assert(func() {t.Field("foo")}, gc.PanicMatches, "Field called on nullable")
	c.Assert(t.String(), gc.Equals, `{"_nullable":"number"}`)
}

func (typeSuite) TestObjectOf(c *gc.C) {
	elemX := jsonreflect.SimpleType(jsonreflect.Number)
	elemS := jsonreflect.SimpleType(jsonreflect.String)
	fields := map[string]*jsonreflect.Type{"S": elemS, "X": elemX}
	t := jsonreflect.ObjectOf("foo", fields)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Object)
	c.Assert(func() {t.Elem()}, gc.PanicMatches, "Elem called on object")
	c.Assert(t.Name(), gc.Equals, "foo")
	c.Assert(t.FieldNames(), gc.DeepEquals, []string{"S", "X"})
	f, ok := t.Field("S")
	c.Assert(ok, gc.Equals, true)
	c.Assert(f, gc.Equals, elemS)
	f, ok = t.Field("S")
	c.Assert(ok, gc.Equals, true)
	c.Assert(f, gc.Equals, elemS)
	f, ok = t.Field("Other")
	c.Assert(ok, gc.Equals, false)
	c.Assert(f, gc.IsNil)
	c.Assert(t.String(), gc.Equals, `{"S":"string","X":"number"}`)
}