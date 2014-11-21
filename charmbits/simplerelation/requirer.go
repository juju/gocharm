// The simplerelation package implements a generic simple relation
// where the provider units each make a set of attributes available,
// and the requirer units can see those attributes.
//
// It allows only one provider service.
package simplerelation

import (
	"sort"

	"gopkg.in/juju/charm.v4"

	"github.com/juju/gocharm/hook"
)

// Requirer represents the requirer side of a simple relation.
// It allows only one provider service. it makes all attributes
// set by units on the provider side of the relation available
// through the Values method.
type Requirer struct {
	ctxt         *hook.Context
	relationName string
}

// Register registers a requirer relation with the given
// relation name and interface with the given hook registry.
//
// To find out when the provider values change, register
// a wildcard ("*") hook, which will trigger when any
// value changes.
func (req *Requirer) Register(r *hook.Registry, relationName, interfaceName string) {
	req.relationName = relationName
	r.RegisterContext(req.setContext, nil)
	r.RegisterRelation(charm.Relation{
		Name:      relationName,
		Interface: interfaceName,
		Role:      charm.RoleRequirer,
	})
	// We don't actually need to do anything in these hooks,
	// but we need them so the hook is actually created
	// and the user of this package will have a "*" hook
	// triggered.
	r.RegisterHook(req.relationName+"-relation-joined", nop)
	r.RegisterHook(req.relationName+"-relation-changed", nop)
	r.RegisterHook(req.relationName+"-relation-departed", nop)
}

func nop() error {
	return nil
}

func (req *Requirer) setContext(ctxt *hook.Context) error {
	req.ctxt = ctxt
	return nil
}

// Values returns the values provided by all the provider units,
// as a map from unit id to attributes to values.
func (req *Requirer) Values() map[hook.UnitId]map[string]string {
	ids := req.ctxt.RelationIds[req.relationName]
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 1 {
		req.ctxt.Logf("more than one provider for the %s relation", req.relationName)
		return nil
	}
	return req.ctxt.Relations[ids[0]]
}

// Strings is a convenience method that converts the
// values returned by Values into a slice of strings by
// calling the given convert function for each unit.
//
// Errors found when doing the conversion are logged
// but otherwise ignored. If the convert function returns
// an empty string with no error, that string will be
// omitted too.
//
// The result is stable across calls.
func (req *Requirer) Strings(convert func(map[string]string) (string, error)) []string {
	unitVals := req.Values()

	// Sort the returned attributes by unit id so that
	// the order is stable across calls to Values.
	unitIds := make([]string, 0, len(unitVals))
	for unitId := range unitVals {
		unitIds = append(unitIds, string(unitId))
	}
	sort.Strings(unitIds)
	vals := make([]string, 0, len(unitVals))
	for _, unitId := range unitIds {
		s, err := convert(unitVals[hook.UnitId(unitId)])
		if err != nil {
			req.ctxt.Logf("unit %s has invalid attributes: %v", unitId, err)
			continue
		}
		if s == "" {
			continue
		}
		vals = append(vals, s)
	}
	return vals
}
