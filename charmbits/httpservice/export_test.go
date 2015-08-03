package httpservice

import (
	"reflect"
)

func ClearRegisteredRelations() {
	registeredRelations = make(map[reflect.Type]*registeredRelation)
}

func RegisteredRelationInfo(t reflect.Type) (T, D reflect.Type, ok bool) {
	reg, ok := registeredRelations[t]
	if !ok {
		return nil, nil, false
	}
	return reg.t, reg.d, true
}
