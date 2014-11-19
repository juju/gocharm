// The concat package implements a
// charm that takes all the string values from units on upstream
// relations, concatenates them and makes them available to downstream
// relations.
package concat

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/charmbits/httpcharm"
	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
)

var empty = &struct{}{}

func RegisterHooks(r *hook.Registry) {
	r.RegisterRelation(charm.Relation{
		Name:      "downstream",
		Interface: "stringval",
		Role:      charm.RoleProvider,
	})
	r.RegisterRelation(charm.Relation{
		Name:      "upstream",
		Interface: "stringval",
		Role:      charm.RoleRequirer,
	})
	r.RegisterConfig("val", charm.Option{
		Description: "A string value",
		Type:        "string",
	})
	var concat concatenator
	concat.http.Register(r.Clone("httpserver"), "http", nil)
	concat.svc.Register(r.Clone("service"), "", startServer)

	r.RegisterContext(concat.setContext, &concat.state)

	// Note that registered hooks are called in order, so the
	// functions below are guaranteed to run after any callbacks
	// triggered by concat.http or concat.svc.

	r.RegisterHook("upstream-relation-changed", concat.changed)
	r.RegisterHook("upstream-relation-departed", concat.changed)
	r.RegisterHook("downstream-relation-joined", concat.changed)
	r.RegisterHook("charm-upgraded", concat.changed)
	r.RegisterHook("config-changed", concat.changed)

	// The finall method runs after any other hook, and
	// reconciles any state changed by the hooks.
	r.RegisterHook("*", concat.finally)
}

// localState holds persistent state for the concatenator charms.
type localState struct {
	Val string
}

// concatenator manages the top level logic of the
// concat charm.
type concatenator struct {
	ctxt     *hook.Context
	http     httpcharm.Provider
	svc      service.Service
	state    localState
	oldState localState
	oldPort  int
}

func (c *concatenator) setContext(ctxt *hook.Context) error {
	c.ctxt = ctxt
	c.oldState = c.state
	c.oldPort = c.http.Port()
	return nil
}

func (c *concatenator) changed() error {
	var vals []string
	localVal, err := c.ctxt.GetConfig("val")
	if err != nil {
		return errors.Wrap(err)
	}
	if localVal != nil && localVal != "" {
		vals = append(vals, localVal.(string))
	}
	for _, id := range c.ctxt.RelationIds["upstream"] {
		units := c.ctxt.Relations[id]
		// Use all the values sorted by the unit they come from, so the charm
		// output is deterministic.
		unitIds := make(unitIdSlice, 0, len(units))
		for id := range units {
			unitIds = append(unitIds, id)
		}
		sort.Sort(unitIds)
		for _, unitId := range unitIds {
			vals = append(vals, units[unitId]["val"])
		}
	}
	c.state.Val = fmt.Sprintf("{%s}", strings.Join(vals, " "))
	return nil
}

type unitIdSlice []hook.UnitId

func (u unitIdSlice) Len() int           { return len(u) }
func (u unitIdSlice) Swap(i, j int)      { u[i], u[j] = u[j], u[i] }
func (u unitIdSlice) Less(i, j int) bool { return u[i] < u[j] }

// finally runs after all the other hooks. We do this rather
// than running the logic in the individual hooks, so that
// we get a consolidated view of the current state of the unit,
// and can avoid doing too much.
func (c *concatenator) finally() error {
	if c.state == c.oldState && c.http.Port() == c.oldPort {
		return nil
	}
	if err := c.notifyServer(); err != nil {
		return errors.Wrap(err)
	}
	ids := c.ctxt.RelationIds["downstream"]
	for _, id := range ids {
		if err := c.ctxt.SetRelationWithId(id, "val", c.state.Val); err != nil {
			return errors.Wrapf(err, "cannot set relation %v", id)
		}
	}
	return nil
}

func (c *concatenator) notifyServer() error {
	if !c.svc.Started() {
		if err := c.svc.Start(c.ctxt.StateDir()); err != nil {
			return errors.Wrap(err)
		}
	}
	err := c.svc.Call("ConcatServer.Set", &ServerState{
		Val:  c.state.Val,
		Port: c.http.Port(),
	}, &struct{}{})
	if err != nil {
		return errors.Wrapf(err, "cannot set state in server")
	}
	return nil
}
