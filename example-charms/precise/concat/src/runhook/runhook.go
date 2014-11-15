// The runhook package implements a
// charm that takes all the string values from units on upstream
// relations, concatenates them and makes them available to downstream
// relations.
package runhook

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
	concat.http.Register(r.Clone("httpserver"), "http", &concat)
	concat.svc.Register(r.Clone("service"), "", new(ConcatServer))

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
	Port int
	Val  string
}

// concatenator manages the top level logic of the
// concat charm.
type concatenator struct {
	ctxt     *hook.Context
	http     httpcharm.Provider
	svc      service.Service
	state    localState
	oldState localState
}

func (c *concatenator) setContext(ctxt *hook.Context) error {
	c.ctxt = ctxt
	c.oldState = c.state
	return nil
}

func (c *concatenator) HTTPServerPortChanged(port int) error {
	c.ctxt.Logf("http server port changed to %v", port)
	c.state.Port = port
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
	c.ctxt.Logf("changed, localVal %v", localVal)
	c.ctxt.Logf("relation ids %#v", c.ctxt.RelationIds)
	c.ctxt.Logf("relations: %#v", c.ctxt.Relations)
	for _, id := range c.ctxt.RelationIds["upstream"] {
		units := c.ctxt.Relations[id]
		c.ctxt.Logf("found %d units with relation id %s", len(units), id)
		// Use all the values sorted by the unit they come from, so the charm
		// output is deterministic.
		unitIds := make(unitIdSlice, 0, len(units))
		for id := range units {
			unitIds = append(unitIds, id)
		}
		sort.Sort(unitIds)
		for _, unitId := range unitIds {
			c.ctxt.Logf("appending %q", units[unitId]["val"])
			vals = append(vals, units[unitId]["val"])
		}
	}
	c.state.Val = fmt.Sprintf("{%s}", strings.Join(vals, " "))
	c.ctxt.Logf("after changed, val is %q", c.state.Val)
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
	c.ctxt.Logf("finally %s, state %#v; oldState %#v", c.ctxt.HookName, c.state, c.oldState)
	if c.state == c.oldState {
		return nil
	}
	if c.state.Port != c.oldState.Port {
		// The HTTP port has changed. Restart the server
		// to use the new port.
		if err := c.restartService(); err != nil {
			return errors.Wrap(err)
		}
		return nil
	}
	if err := c.setServiceVal(); err != nil {
		return errors.Wrap(err)
	}
	ids := c.ctxt.RelationIds["downstream"]
	c.ctxt.Logf("setting downstream relations %v to %s", ids, c.state.Val)
	for _, id := range ids {
		if err := c.ctxt.SetRelationWithId(id, "val", c.state.Val); err != nil {
			return errors.Wrapf(err, "cannot set relation %v", id)
		}
	}
	return nil
}

func (c *concatenator) restartService() error {
	c.ctxt.Logf("restarting service")
	if c.svc.Started() {
		// Stop the service so that it can be restarted on a different
		// port. In a more sophisticated program, this message the
		// existing server so that it could wait for all outstanding
		// requests to complete as well as starting the server
		// on the new port.
		if err := c.svc.Stop(); err != nil {
			return errors.Wrapf(err, "cannot stop service")
		}
	}
	if err := c.svc.Start(); err != nil {
		return errors.Wrapf(err, "cannot start service")
	}
	if err := c.setServiceVal(); err != nil {
		return errors.Wrapf(err, "cannot set initial value")
	}
	err := c.svc.Call("ConcatServer.Start", &StartParams{
		Port: c.state.Port,
	}, empty)
	if err != nil {
		return errors.Wrapf(err, "cannot set port on service")
	}
	return nil
}

func (c *concatenator) setServiceVal() error {
	c.ctxt.Logf("setting service val to %s", c.state.Val)
	return c.svc.Call("ConcatServer.SetVal", &SetValParams{
		Val: c.state.Val,
	}, empty)
}
