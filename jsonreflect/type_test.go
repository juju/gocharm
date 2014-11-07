package jsonreflect_test

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"unsafe"

	gc "launchpad.net/gocheck"

	"gopkg.in/juju-utils.v0/jsonreflect"
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
		c.Assert(func() { t.Elem() }, gc.PanicMatches, "Elem called on "+kind.String())
		c.Assert(func() { t.Name() }, gc.PanicMatches, "Name called on "+kind.String())
		c.Assert(func() { t.FieldNames() }, gc.PanicMatches, "FieldNames called on "+kind.String())
		c.Assert(func() { t.Field("foo") }, gc.PanicMatches, "Field called on "+kind.String())
		c.Assert(t.String(), gc.Equals, fmt.Sprintf("%q", kind.String()))
	}
}

func (typeSuite) TestSimpleTypeWithBadKind(c *gc.C) {
	for i, kind := range []jsonreflect.Kind{jsonreflect.Map, jsonreflect.Nullable, jsonreflect.Object, jsonreflect.Custom} {
		c.Logf("test %d: %v", i, kind)
		c.Assert(func() { jsonreflect.SimpleType(kind) }, gc.PanicMatches, "SimpleType called with invalid kind "+kind.String())
	}
}

func (typeSuite) TestCustomType(c *gc.C) {
	c.Assert(func() { jsonreflect.CustomType("") }, gc.PanicMatches, "CustomType called with empty name")
	t := jsonreflect.CustomType("foo")
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Custom)
	c.Assert(func() { t.Elem() }, gc.PanicMatches, "Elem called on custom")
	c.Assert(t.Name(), gc.Equals, "foo")
	c.Assert(func() { t.FieldNames() }, gc.PanicMatches, "FieldNames called on custom")
	c.Assert(func() { t.Field("foo") }, gc.PanicMatches, "Field called on custom")
	c.Assert(t.String(), gc.Equals, `"foo"`)
}

func (typeSuite) TestArrayOf(c *gc.C) {
	elem := jsonreflect.SimpleType(jsonreflect.Number)
	t := jsonreflect.ArrayOf(elem)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Array)
	c.Assert(t.Elem(), gc.Equals, elem)
	c.Assert(func() { t.Name() }, gc.PanicMatches, "Name called on array")
	c.Assert(func() { t.FieldNames() }, gc.PanicMatches, "FieldNames called on array")
	c.Assert(func() { t.Field("foo") }, gc.PanicMatches, "Field called on array")
	c.Assert(t.String(), gc.Equals, `["number"]`)
}

func (typeSuite) TestMapOf(c *gc.C) {
	elem := jsonreflect.SimpleType(jsonreflect.Number)
	t := jsonreflect.MapOf(elem)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Map)
	c.Assert(t.Elem(), gc.Equals, elem)
	c.Assert(func() { t.Name() }, gc.PanicMatches, "Name called on map")
	c.Assert(func() { t.FieldNames() }, gc.PanicMatches, "FieldNames called on map")
	c.Assert(func() { t.Field("foo") }, gc.PanicMatches, "Field called on map")
	c.Assert(t.String(), gc.Equals, `{"_map":"number"}`)
}

func (typeSuite) TestNullableOf(c *gc.C) {
	elem := jsonreflect.SimpleType(jsonreflect.Number)
	t := jsonreflect.NullableOf(elem)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Nullable)
	c.Assert(t.Elem(), gc.Equals, elem)
	c.Assert(func() { t.Name() }, gc.PanicMatches, "Name called on nullable")
	c.Assert(func() { t.FieldNames() }, gc.PanicMatches, "FieldNames called on nullable")
	c.Assert(func() { t.Field("foo") }, gc.PanicMatches, "Field called on nullable")
	c.Assert(t.String(), gc.Equals, `{"_nullable":"number"}`)
}

func (typeSuite) TestObjectOf(c *gc.C) {
	elemX := jsonreflect.SimpleType(jsonreflect.Number)
	elemS := jsonreflect.SimpleType(jsonreflect.String)
	fields := map[string]*jsonreflect.Type{"S": elemS, "X": elemX}
	t := jsonreflect.ObjectOf("foo", fields)
	c.Assert(t.Kind(), gc.Equals, jsonreflect.Object)
	c.Assert(func() { t.Elem() }, gc.PanicMatches, "Elem called on object")
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

var nonEqualTypes = []*jsonreflect.Type{
	jsonreflect.SimpleType(jsonreflect.Bool),
	jsonreflect.SimpleType(jsonreflect.Number),
	jsonreflect.SimpleType(jsonreflect.String),
	jsonreflect.CustomType("custom.One"),
	jsonreflect.CustomType("custom.Two"),
	jsonreflect.ArrayOf(jsonreflect.SimpleType(jsonreflect.Number)),
	jsonreflect.ArrayOf(jsonreflect.SimpleType(jsonreflect.String)),
	jsonreflect.MapOf(jsonreflect.SimpleType(jsonreflect.Number)),
	jsonreflect.MapOf(jsonreflect.SimpleType(jsonreflect.String)),
	jsonreflect.NullableOf(jsonreflect.SimpleType(jsonreflect.Number)),
	jsonreflect.NullableOf(jsonreflect.SimpleType(jsonreflect.String)),
	jsonreflect.ObjectOf("Foo", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
		"y": jsonreflect.SimpleType(jsonreflect.String),
	}),
	jsonreflect.ObjectOf("Bar", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
	}),
	jsonreflect.ObjectOf("Baz", nil),
}

func (typeSuite) TestNotEqual(c *gc.C) {
	for i, t0 := range nonEqualTypes {
		for j, t1 := range nonEqualTypes {
			if i == j {
				continue
			}
			if t0.Equal(t1) {
				c.Errorf("%s compared equal to %s", t0, t1)
			}
		}
	}
}

var equalTypes = [][]*jsonreflect.Type{{
	jsonreflect.SimpleType(jsonreflect.Bool),
	jsonreflect.SimpleType(jsonreflect.Bool),
}, {
	jsonreflect.SimpleType(jsonreflect.Number),
	jsonreflect.SimpleType(jsonreflect.Number),
}, {
	jsonreflect.SimpleType(jsonreflect.String),
	jsonreflect.SimpleType(jsonreflect.String),
}, {
	jsonreflect.ArrayOf(jsonreflect.SimpleType(jsonreflect.String)),
	jsonreflect.ArrayOf(jsonreflect.SimpleType(jsonreflect.String)),
}, {
	jsonreflect.NullableOf(jsonreflect.SimpleType(jsonreflect.String)),
	jsonreflect.NullableOf(jsonreflect.SimpleType(jsonreflect.String)),
}, {
	jsonreflect.ObjectOf("Foo", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
		"y": jsonreflect.SimpleType(jsonreflect.String),
	}),
	jsonreflect.ObjectOf("Foo", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
		"y": jsonreflect.SimpleType(jsonreflect.String),
	}),
}, {
	jsonreflect.ObjectOf("Foo", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
		"y": jsonreflect.SimpleType(jsonreflect.String),
	}),
	jsonreflect.ObjectOf("", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
		"y": jsonreflect.SimpleType(jsonreflect.String),
	}),
	jsonreflect.ObjectOf("", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
		"y": jsonreflect.SimpleType(jsonreflect.String),
	}),
}, {
	// Two object types with the same non-blank name are considered
	// equal even if they aren't actually structurally the same.
	jsonreflect.ObjectOf("Foo", map[string]*jsonreflect.Type{
		"x": jsonreflect.SimpleType(jsonreflect.Number),
		"y": jsonreflect.SimpleType(jsonreflect.String),
	}),
	jsonreflect.ObjectOf("Foo", map[string]*jsonreflect.Type{
		"foo": jsonreflect.SimpleType(jsonreflect.Number),
	}),
	jsonreflect.CustomType("Foo"),
	jsonreflect.CustomType("Foo"),
}}

func (typeSuite) TestEqual(c *gc.C) {
	for _, types := range equalTypes {
		for _, t0 := range types {
			for _, t1 := range types {
				if !t0.Equal(t1) {
					c.Errorf("%s compared unequal to %s", t0, t1)
				}
			}
		}
	}
}

type (
	myString string
	myBool   bool
	jsonable int
	foo      struct {
		X int
		Y string
	}
)

func (jsonable) MarshalJSON() ([]byte, error) {
	return nil, nil
}

var (
	jnumber = jsonreflect.SimpleType(jsonreflect.Number)
	jstring = jsonreflect.SimpleType(jsonreflect.String)
	jbool   = jsonreflect.SimpleType(jsonreflect.Bool)
)

func (typeSuite) TestTypeOf(c *gc.C) {
	type big struct {
		String        string
		MyString      myString
		Int           int
		Int8          int8
		Int16         int16
		Int32         int32
		Int64         int64
		Uint8         uint8
		Uint16        uint16
		Uint32        uint32
		Uint64        uint64
		Float32       float32
		Float64       float64
		Uintptr       uintptr
		Bool          bool
		MyBool        myBool
		Complex64     complex64
		Complex128    complex128
		ChanInt       chan int
		UnsafePointer unsafe.Pointer
		Error         error
		SortInterface sort.Interface
		Func          func()
		IntSlice      []int
		IntArray      [1]int
		IntPtr        *int
		Jsonable      jsonable
		Foo           foo
	}
	expect := jsonreflect.ObjectOf("big", map[string]*jsonreflect.Type{
		"String":        jstring,
		"MyString":      jstring,
		"Int":           jnumber,
		"Int8":          jnumber,
		"Int16":         jnumber,
		"Int32":         jnumber,
		"Int64":         jnumber,
		"Uint8":         jnumber,
		"Uint16":        jnumber,
		"Uint32":        jnumber,
		"Uint64":        jnumber,
		"Float32":       jnumber,
		"Float64":       jnumber,
		"Uintptr":       jnumber,
		"Bool":          jbool,
		"MyBool":        jbool,
		"Complex64":     jsonreflect.CustomType("complex"),
		"Complex128":    jsonreflect.CustomType("complex"),
		"ChanInt":       jsonreflect.CustomType("chan"),
		"UnsafePointer": jsonreflect.CustomType("unsafe.Pointer"),
		"Error":         jsonreflect.CustomType("error"),
		"SortInterface": jsonreflect.CustomType("Interface"), // wrong?
		"Func":          jsonreflect.CustomType("func"),
		"IntSlice":      jsonreflect.ArrayOf(jnumber),
		"IntArray":      jsonreflect.ArrayOf(jnumber),
		"IntPtr":        jsonreflect.NullableOf(jnumber),
		"Jsonable":      jsonreflect.CustomType("jsonable"),
		"Foo":           jsonreflect.CustomType("foo"),
	})
	t := jsonreflect.TypeOf(reflect.TypeOf(big{}))
	if !t.Equal(expect) {
		c.Errorf("mismatch; expected %s got %s", expect, t)
	}
}
