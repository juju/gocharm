// The runhook package implements a
// charm that takes all the string values from units on upstream
// relations, concatenates them and makes them available to downstream
// relations.
package runhook

import (
	"fmt"
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
	concat.http.Register(r.NewRegistry("httpserver"), "http", &concat)
	concat.svc.Register(r.NewRegistry("service"), "", new(ConcatServer))

	r.RegisterContext(concat.setContext)

	// Note that registered hooks are called in order, so the
	// functions below are guaranteed to run after any callbacks
	// triggered by concat.http or concat.svc.

	r.RegisterHook("upstream-relation-changed", concat.changed)
	r.RegisterHook("upstream-relation-departed", concat.changed)
	r.RegisterHook("downstream-relation-joined", concat.changed)
	r.RegisterHook("config-changed", concat.changed)
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
	state    *localState
	oldState localState
}

func (c *concatenator) setContext(ctxt *hook.Context) error {
	c.ctxt = ctxt
	if err := ctxt.LocalState("state", &c.state); err != nil {
		return errors.Wrap(err)
	}
	c.oldState = *c.state
	return nil
}

func (c *concatenator) HTTPServerPortChanged(port int) error {
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
	upstreamIds, err := c.ctxt.RelationIds("upstream")
	if err != nil {
		return errors.Wrap(err)
	}
	for _, id := range upstreamIds {
		units, err := c.ctxt.RelationUnits(id)
		if err != nil {
			return errors.Wrap(err)
		}
		for _, unit := range units {
			val, err := c.ctxt.GetRelationUnit(id, unit, "val")
			if err != nil {
				return errors.Wrap(err)
			}
			vals = append(vals, val)
		}
	}
	c.state.Val = fmt.Sprintf("{%s}", strings.Join(vals, " "))
	return nil
}

// finally runs after all the other hooks. We do this rather
// than running the logic in the individual hooks, so that
// we get a consolidated view of the current state of the unit,
// and can avoid doing too much.
func (c *concatenator) finally() error {
	if *c.state == c.oldState {
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
	ids, err := c.ctxt.RelationIds("downstream")
	if err != nil {
		return errors.Wrap(err)
	}
	c.ctxt.Logf("setting downstream relations %v to %s", ids, c.state.Val)
	for _, id := range ids {
		if err := c.ctxt.SetRelationWithId(id, "val", c.state.Val); err != nil {
			return errors.Wrapf(err, "cannot set relation %v", id)
		}
	}
	return nil
}

func (c *concatenator) restartService() error {
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
	return c.svc.Call("ConcatServer.SetVal", &SetValParams{
		Val: c.state.Val,
	}, empty)
}
