package simplerelation

import (
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/gocharm/hook"
)

// Provider represents the provider side of a simple relation.
type Provider struct {
	state        providerState
	ctxt         *hook.Context
	relationName string
}

type providerState struct {
	// Values holds a set of attribute-value pairs,
	// as required by hook.Context.SetRelationWithId.
	Values []string
}

// Register registers the provider side of a simple relation with the
// given relation name and interface.
//
// It makes the values set by the SetValues method available to all requirers.
func (p *Provider) Register(r *hook.Registry, relationName, interfaceName string) {
	r.RegisterRelation(charm.Relation{
		Name:      relationName,
		Interface: interfaceName,
		Role:      charm.RoleProvider,
		Scope:     charm.ScopeGlobal,
	})
	r.RegisterHook(relationName+"-relation-joined", p.relationJoined)
	r.RegisterContext(p.setContext, &p.state)
	p.relationName = relationName
}

func (p *Provider) setContext(ctxt *hook.Context) error {
	p.ctxt = ctxt
	return nil
}

// SetValues makes the given relation attributes and values
// available to all requirer-side units of the relation.
func (p *Provider) SetValues(vals map[string]string) error {
	keyvals := make([]string, 0, 2*len(vals))
	for attr, val := range vals {
		keyvals = append(keyvals, attr)
		keyvals = append(keyvals, val)
	}
	// Set the current address in all requirers.
	for _, id := range p.ctxt.RelationIds[p.relationName] {
		if err := p.ctxt.SetRelationWithId(id, keyvals...); err != nil {
			return errgo.Mask(err)
		}
	}
	p.state.Values = keyvals
	return nil
}

func (p *Provider) relationJoined() error {
	if err := p.ctxt.SetRelation(p.state.Values...); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
