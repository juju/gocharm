// The concat package implements a charm that takes all the string
// values from units on upstream relations, concatenates them and makes
// them available to downstream relations.
//
// This provides a silly but simple example that
// allows the exploration of relation behaviour in Juju.
//
// For example, here's an example of this being used from the juju-utils repository:
//     JUJU_REPOSITORY=$GOPATH/src/launchpad.net/juju-utils/cmd/gocharm/example-charms
//     export JUJU_REPOSITORY
//     gocharm
//     juju deploy local:concat concattop
//     juju deploy local:concat concat1
//     juju deploy local:concat concat2
//     juju deploy local:concat concatjoin
//     juju add-relation concattop:downstream concat1:upstream
//     juju add-relation concattop:downstream concat2:upstream
//     juju add-relation concat1:downstream concatjoin:upstream
//     juju add-relation concat2:downstream concatjoin:upstream
//     juju set concattop 'val=top'
//     juju set concat1 'val=concat1'
//     juju set concat2 'val=concat2'
//     juju set concatjoin 'val=concatjoin'
//
// The final value of the downstream relation provided by
// by the concatjoin service in this case will be:
//
//     {concatjoin {concat2 {top}} {concat1 {top}}}
//
// Feedback loops can be arranged for further amusement.
package concat

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/gocharm/charmbits/httprelation"
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
	concat.http.Register(r.Clone("httpserver"), "http", false)
	concat.svc.Register(r.Clone("service"), "", startServer)

	r.RegisterContext(concat.setContext, &concat.state)

	// Note that registered hooks are called in order, so the
	// functions below are guaranteed to run after any callbacks
	// triggered by concat.http or concat.svc.

	r.RegisterHook("upgrade-charm", concat.changed)
	r.RegisterHook("config-changed", concat.changed)
	r.RegisterHook("upstream-relation-changed", concat.changed)
	r.RegisterHook("upstream-relation-departed", concat.changed)
	r.RegisterHook("downstream-relation-joined", concat.downstreamJoined)

	// The finall method runs after any other hook, and
	// reconciles any state changed by the hooks.
	r.RegisterHook("*", concat.finally)
}

// localState holds persistent state for the concatenator charm.
type localState struct {
	Val  string
	Port int
}

// concatenator manages the top level logic of the
// concat charm.
type concatenator struct {
	ctxt *hook.Context
	http httprelation.Provider
	svc  service.Service

	// state holds a record of the committed state.
	state localState

	// newState and newPort hold the state as we would like it to be.
	// If we succeed in notifying everything that needs to be
	// notified, this will be committed to state.
	newState localState
}

func (c *concatenator) setContext(ctxt *hook.Context) error {
	c.ctxt = ctxt
	c.newState = c.state
	return nil
}

func (c *concatenator) changed() error {
	var vals []string
	localVal, err := c.ctxt.GetConfigString("val")
	if err != nil {
		return errgo.Mask(err)
	}
	if localVal != "" {
		vals = append(vals, localVal)
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
	c.newState.Val = fmt.Sprintf("{%s}", strings.Join(vals, " "))
	c.newState.Port = c.http.HTTPPort()
	return nil
}

func (c *concatenator) downstreamJoined() error {
	return c.setDownstreamVal(c.ctxt.RelationId, c.state.Val)
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
	if c.newState == c.state {
		c.ctxt.Logf("concat state is unchanged at %#v; doing nothing", c.state)
		return nil
	}
	c.ctxt.Logf("concat state changed from %#v to %#v", c.state, c.newState)
	if err := c.notifyServer(); err != nil {
		return errgo.Mask(err)
	}
	ids := c.ctxt.RelationIds["downstream"]
	for _, id := range ids {
		if err := c.setDownstreamVal(id, c.newState.Val); err != nil {
			return errgo.Notef(err, "cannot set relation %v", id)
		}
	}
	// We've succeeded in notifying everything of the changes, so
	// commit the state.
	c.state = c.newState
	return nil
}

func (c *concatenator) setDownstreamVal(id hook.RelationId, val string) error {
	c.ctxt.Logf("setting downstream relation %q to %q", id, c.newState.Val)
	return c.ctxt.SetRelationWithId(id, "val", val)
}

func (c *concatenator) notifyServer() error {
	if !c.svc.Started() {
		if err := c.svc.Start(c.ctxt.StateDir()); err != nil {
			return errgo.Mask(err)
		}
	}
	err := c.svc.Call("ConcatServer.Set", &ServerState{
		Val:  c.newState.Val,
		Port: c.http.HTTPPort(),
	}, &struct{}{})
	if err != nil {
		return errgo.Notef(err, "cannot set state in server")
	}
	return nil
}
