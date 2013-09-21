package jsonreflect

import (
	"encoding/json"
	"fmt"
)

// Marshaller knows how to marshal types to JSON.
// It maintains a table of known types, added to by
// calling AddType.
//
// If an object type has been added more than once, it will
// be marshalled as if it were a Custom type rather than
// directly as an object. It is the responsibility of the
// caller to ensure that the information for types marshalled like this
// is available elsewhere.
//
type Marshaller struct {
	types map[string]*refType
}

// refType holds a type along with a reference count.
type refType struct {
	*Type
	refCount int
}

func NewMarshaller() *Marshaller {
	return &Marshaller{
		types: make(map[string]*refType),
	}
}

// AddType adds knowledge of a type to the Marshaller.
// It returns a version of the same type
func (m *Marshaller) AddType(t *Type) {
	switch t.Kind() {
	case String, Bool, Number:
		return
	case Map, Nullable, Array:
		m.AddType(t.Elem())
	case Object:
		if name := t.Name(); name != "" {
			if reft := m.types[name]; reft != nil {
				if reft.Kind() == Custom {
					// Replace Custom placeholder with actual object type information.
					reft.Type = t
				}
				reft.refCount++
				return
			}
			m.types[name] = &refType{Type: t, refCount: 1}
		}
		for _, field := range t.fields {
			m.AddType(field)
		}
	case Custom:
		if reft := m.types[t.Name()]; reft != nil {
			reft.refCount++
			return
		}
		m.types[t.Name()] = &refType{Type: t, refCount: 1}
	default:
		panic("unknown kind")
	}
}

// RefTypes returns all the Object types that have been added more
// than once - these types will be marshalled as if they were
// of Custom kind.
func (m *Marshaller) RefTypes() map[string]*Type {
	r := make(map[string]*Type)
	for name, t := range m.types {
		if t.refCount > 1 && t.Kind() != Custom {
			r[name] = t.Type
		}
	}
	return r
}

// Marshal marshals the given type to JSON.
// If t is an Object that has been added to the marshaller more than
// once, it will be marshalled as if it were Custom.
func (m *Marshaller) Marshal(t *Type) ([]byte, error) {
	var obj interface{}
	switch t.Kind() {
	case String, Number, Bool:
		obj = t.Kind().String()
	case Array:
		obj = []*Type{t.Elem()}
	case Object:
		if reft := m.types[t.Name()]; reft != nil && reft.refCount > 1 {
			obj = t.Name
		} else if t.fields == nil {
			obj = struct{}{}
		} else {
			obj = t.fields
		}
	case Map:
		obj = map[string]*Type{"_map": t.Elem()}
	case Nullable:
		obj = map[string]*Type{"_nullable": t.Elem()}
	case Custom:
		if reft := m.types[t.Name()]; reft != nil && reft.refCount == 1 && reft.Kind() != Custom {
			// Marshal custom types as normal types if we have their
			// type information.
			return m.Marshal(reft.Type)
		}
		obj = t.Name()
	}
	return json.Marshal(obj)
}

// Normalize returns t normalized to resolve
// any Custom types that have been added to
// the Marshaller. This is useful when the Type has
// been created independently of a Marshaller.
func (m *Marshaller) Normalize(t *Type) *Type {
	switch t.Kind() {
	case String, Bool, Number:
	case Array, Nullable, Map:
		if newElem := m.Normalize(t.Elem()); newElem != t.Elem() {
			t = &Type{
				kind: t.Kind(),
				elem: newElem,
			}
		}
	case Object:
		if reft := m.types[t.Name()]; reft != nil {
			newt := m.Normalize(reft.Type)
			// Lazily normalize the existing types stored in the Marshaller.
			if newt != reft.Type {
				reft.Type = newt
			}
			t = newt
			break
		}
		var fields map[string]*Type
		for name, field := range t.fields {
			newField := m.Normalize(field)
			if newField != field {
				if fields == nil {
					fields = make(map[string]*Type)
				}
				fields[name] = newField
			}
		}
		if fields == nil {
			// No field has changed, so no new type required.
			break
		}
		// Fill in remaining unchanged fields.
		for name, field := range t.fields {
			if fields[name] == nil {
				fields[name] = field
			}
		}
		t = &Type{
			kind: Object,
			name: t.Name(),
		}
	default:
		panic("unknown kind")
	}
	return t
}

func (m *Marshaller) Unmarshal(b []byte) (*Type, error) {
	t := new(Type)
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, err
		}
		switch s {
		case "string":
			t.kind = String
		case "number":
			t.kind = Number
		case "bool":
			t.kind = Bool
		default:
			if reft := m.types[s]; reft != nil {
				// It's a custom type that we know about.
				return reft.Type, nil
			}
			t.kind = Custom
			t.name = s
		}
	case '[':
		var elems []*Type
		if err := json.Unmarshal(b, &elems); err != nil {
			return nil, err
		}
		if len(elems) != 1 {
			return nil, fmt.Errorf("unexpected element count %d in array (need 1)", len(elems))
		}
		t.kind = Array
		t.elem = elems[0]
	case '{':
		var fields map[string]*Type
		if err := json.Unmarshal(b, &fields); err != nil {
			return nil, err
		}
		switch {
		case fields["_map"] != nil:
			t.kind = Map
			t.elem = fields["_map"]
		case fields["_nullable"] != nil:
			t.kind = Nullable
			t.elem = fields["_nullable"]
		default:
			t.kind = Object
			t.fields = fields
		}
	default:
		return nil, fmt.Errorf("cannot unmarshal %q into Type", b)
	}
	return t, nil
}

func (t *Type) MarshalJSON() ([]byte, error) {
	m := &Marshaller{}
	return m.Marshal(t)
}

func (t *Type) UnmarshalJSON(b []byte) error {
	m := &Marshaller{}
	ut, err := m.Unmarshal(b)
	if err != nil {
		return err
	}
	*t = *ut
	return nil
}
