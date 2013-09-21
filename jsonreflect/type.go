// The jsonreflect package provides a JSON-marshalable representation
// of Go types as marshalled to JSON. The various kinds of type represent
// JSON elements, with CustomType representing Go types that do not
// map to JSON types, or that have a potentially fluid JSON representation
// (interfaces and types with a MarshalJSON)
//
// When a *Type is JSON-encoded, it is encoded differently depending on
// its Kind, as follows.
//
// The String, Bool and Number kinds are encoded as "string", "bool" and
// "number" respectively.
//
// The Custom kind is encoded as a string containing the Go type name.
//
// The Object kind is encoded as an object with one entry for each field
// name, holding its JSON-encoded type.
//
// The Map kind is encoded as an object with a single field, "_map",
// holding the JSON-encoded element type.
//
// The Array kind is encoded as a list with a single element, the
// JSON-encoded element type.
//
// The Nullable kind is encoded an object with a single field,
// "_nullable", holding the JSON-encoded element type.
//
// CAVEAT: self-referential data structures will currently generate
// an infinite recursion. One possibility for the future:
//
// 	type List struct {Val int; Next *List}
//
// could be encoded as
//
// 	{"_type": "foo.List", Val: "number", Next: "foo.List"}
package jsonreflect

import (
	"fmt"
	"reflect"
	"sort"
)

// TypeOf returns the JSON type representation of the given Go type.
// If t is nil, it returns nil.
func TypeOf(t reflect.Type) *Type {
	if t == nil {
		return ObjectOf("nil", nil)
	}
	if _, ok := t.MethodByName("MarshalJSON"); ok {
		return CustomType(t.String())
	}
	switch t.Kind() {
	case reflect.Bool:
		return SimpleType(Bool)
	case reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return SimpleType(Number)
	case reflect.String:
		return SimpleType(String)
	case reflect.Complex64, reflect.Complex128:
		return CustomType("complex")
	case reflect.Chan:
		return CustomType("chan")
	case reflect.Array, reflect.Slice:
		return ArrayOf(TypeOf(t.Elem()))
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			// TODO better error recovery
			panic("unexpected map key kind")
		}
		return MapOf(TypeOf(t.Elem()))
	case reflect.Ptr:
		return NullableOf(TypeOf(t.Elem()))
	case reflect.Struct:
		return ObjectOf(t.Name(), jsonFields(t))
	case reflect.UnsafePointer:
		return CustomType("unsafe.Pointer")
	case reflect.Func:
		return CustomType("func")
	case reflect.Interface:
		return CustomType(t.String())
	}
	panic(fmt.Errorf("unknown kind %v for type %s", t.Kind(), t))
}

func jsonFields(t reflect.Type) map[string]*Type {
	fields := make(map[string]*Type)
	// TODO anonymous fields
	n := t.NumField()
	for i := 0; i < n; i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		// TODO parse json tag
		fields[field.Name] = TypeOf(field.Type)
	}
	return fields
}

// Type represents a JSON type. Unlike reflect.Type, there is no guarantee
// that a given type will have only one instance.
type Type struct {
	kind   Kind
	name   string
	elem   *Type
	fields map[string]*Type
}

// Kind returns the kind of the type.
func (t *Type) Kind() Kind {
	return t.kind
}

// Equal reports whether the two types are equal.
// Two types are considered equal if they are custom types
// or objects with the same non-empty name,
// or if they are structurally the same.
func (t0 *Type) Equal(t1 *Type) bool {
	if t0 == t1 {
		return true
	}
	kind0, kind1 := t0.Kind(), t1.Kind()
	switch kind0 {
	case Number, String, Bool:
		return kind0 == kind1
	case Object:
		switch kind1 {
		case Object:
			return equalObjects(t0, t1)
		case Custom:
			return t0.Name() == t1.Name()
		}
		return false
	case Custom:
		if kind1 != Object && kind1 != Custom {
			return false
		}
		return t0.Name() == t1.Name()
	case Map, Nullable, Array:
		if kind0 != kind1 {
			return false
		}
		return t0.Elem().Equal(t1.Elem())
	default:
		panic("unexpected kind")
	}
}

func equalObjects(t0, t1 *Type) bool {
	if name := t0.Name(); name != "" && name == t1.Name() {
		return true
	}
	if len(t0.fields) != len(t1.fields) {
		return false
	}
	for fname, f0 := range t0.fields {
		 f1 := t1.fields[fname]
		if f1 == nil {
			return false
		}
		if !f0.Equal(f1) {
			return false
		}
	}
	return true
}

// Elem returns the type's element type. It panics
// unless Kind is Array, Map or Nullable.
func (t *Type) Elem() *Type {
	switch kind := t.Kind(); kind {
	case Array, Map, Nullable:
		return t.elem
	default:
		panic(fmt.Errorf("Elem called on %s", t.Kind()))
	}
}

// Name returns the type name. It panics unless
// the type's kind is Custom or Object.
func (t *Type) Name() string {
	switch kind := t.Kind(); kind {
	case Custom, Object:
		return t.name
	default:
		panic(fmt.Errorf("Name called on %s", t.Kind()))
	}
}

// FieldNames returns the names of the object's fields
// in alphabetical order.
// It panics unless the type's kind is Object.
func (t *Type) FieldNames() []string {
	if t.Kind() != Object {
		panic(fmt.Errorf("FieldNames called on %s", t.Kind()))
	}
	names := make([]string, 0, len(t.fields))
	for name := range t.fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Field returns the type of the named field
// and whether the field was found.
// It panics unless the type's kind is Object.
func (t *Type) Field(name string) (*Type, bool) {
	if t.Kind() != Object {
		panic(fmt.Errorf("Field called on %s", t.Kind()))
	}
	f, ok := t.fields[name]
	return f, ok
}

func (t *Type) String() string {
	if t == nil {
		return "nil"
	}
	m := NewMarshaller()
	m.AddType(t)
	b, err := m.Marshal(t)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(b)
}

// ArrayOf returns a JSON array type with the given element.
func ArrayOf(t *Type) *Type {
	return &Type{
		kind: Array,
		elem: t,
	}
}

// MapOf returns a JSON map type - a JSON
// object where we don't know the members
// in advance, but we know that all members will
// have the given element type.
func MapOf(t *Type) *Type {
	return &Type{
		kind: Map,
		elem: t,
	}
}

// NullableOf returns a JSON type that may be nil, with
// the given element type.
func NullableOf(t *Type) *Type {
	return &Type{
		kind: Nullable,
		elem: t,
	}
}

// ObjectOf returns a JSON object type with the
// given type name and members.
func ObjectOf(name string, fields map[string]*Type) *Type {
	newFields := make(map[string]*Type)
	for name, field := range fields {
		newFields[name] = field
	}
	return &Type{
		kind:   Object,
		fields: newFields,
		name:   name,
	}
}

// SimpleType returns a simple type. It panics unless the
// given kind is one of Bool, Number or String.
func SimpleType(kind Kind) *Type {
	switch kind {
	case Bool, Number, String:
		return &Type{kind: kind}
	}
	panic(fmt.Errorf("SimpleType called with invalid kind %v", kind))
}

// Custom returns a type with an unknown JSON
// representation. The returned type has the given name.
// It panics if name is empty.
func CustomType(name string) *Type {
	if name == "" {
		panic(fmt.Errorf("CustomType called with empty name"))
	}
	return &Type{
		kind: Custom,
		name: name,
	}
}

// Kind represents the kind of a JSON object.
type Kind int

const (
	String Kind = iota + 1
	Number
	Bool
	Array
	Object
	Custom
	Map
	Nullable
)

var kindStrings = []string{
	String:   "string",
	Number:   "number",
	Bool:     "bool",
	Array:    "array",
	Object:   "object",
	Map:      "map",
	Nullable: "nullable",
	Custom:   "custom",
}

func (k Kind) String() string {
	return kindStrings[k]
}
