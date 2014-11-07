package jsonreflect_test

import (
	"reflect"

	gc "launchpad.net/gocheck"

	"gopkg.in/juju-utils.v0/jsonreflect"
)

type marshalSuite struct{}

var _ = gc.Suite(marshalSuite{})

type marshalTypeA struct {
	X common
}

type marshalTypeB struct {
	Y common
}

type marshalTypeC struct {
	Z common
}

type common struct {
	Foo int
}

func typeof(x interface{}) *jsonreflect.Type {
	return jsonreflect.TypeOf(reflect.TypeOf(x))
}

func (marshalSuite) TestAddType(c *gc.C) {
	m := jsonreflect.NewMarshaller()

	t0 := typeof(marshalTypeA{})
	t1 := typeof(marshalTypeB{})
	t2 := typeof(marshalTypeC{})
	m.AddType(t0)
	c.Check(m.RefTypes(), gc.HasLen, 0)

	m.AddType(t1)
	refTypes := m.RefTypes()
	c.Assert(refTypes, gc.HasLen, 1)
	f, ok := t0.Field("X")
	c.Assert(ok, gc.Equals, true)
	c.Check(refTypes["common"], gc.Equals, f)

	m.AddType(t2)
	c.Assert(m.RefTypes(), gc.DeepEquals, refTypes)
}

func (marshalSuite) TestMarshaller(c *gc.C) {
	c.Skip("broken test until marshalling fixed")
	m := jsonreflect.NewMarshaller()
	t0 := typeof(marshalTypeA{})
	t1 := typeof(marshalTypeB{})
	t2 := typeof(marshalTypeC{})
	m.AddType(t0)
	m.AddType(t1)
	m.AddType(t2)

	c.Logf("after adding types, marshaller: %s", m)
	data, err := m.Marshal(t0)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, `{"X":"common"}`)

	data, err = m.Marshal(t1)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, `{"Y":"common"}`)

	data, err = m.Marshal(t2)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, `{"Z":"common"}`)

	refTypes := m.RefTypes()
	c.Assert(refTypes, gc.HasLen, 1)
	data, err = m.Marshal(refTypes["common"])
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, `xxx`)
}
