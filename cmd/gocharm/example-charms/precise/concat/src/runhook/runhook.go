// The runhook package implements a
// charm that takes all the string values from units on upstream
// relations, concatenates them and makes them available to downstream
// relations.
package runhook

import (
	"fmt"
	"net/http"
	"strings"

	"launchpad.net/errgo/errors"
	"launchpad.net/juju-utils/hook"
	"launchpad.net/juju-utils/charmbits/httpserver"
)

func RegisterHooks(r *hook.Registry) {
	httpserver.Register(r.NewRegistry("httpserver"), func() http.Handler {
		return newHandler()
	})
	register := func(hookName string, f func(*concatenator) error) {
		r.Register(hookName, func(ctxt *hook.Context) error {
			return f(newConcatenator(ctxt))
		})
	}
	r.Register("install", nothing)
	r.Register("start", nothing)
	register("config-changed", (*concatenator).changed)
	register("upstream-relation-changed", (*concatenator).changed)
	register("upstream-relation-departed", (*concatenator).changed)
	register("downstream-relation-joined", (*concatenator).changed)
}

type concatenator struct {
	ctxt *hook.Context
}

func newConcatenator(ctxt *hook.Context) *concatenator {
	return &concatenator{
		ctxt: ctxt,
	}
}

func (c *concatenator) changed() error {
	val, err := c.currentVal()
	if err != nil {
		return errors.Wrap(err)
	}
	return errors.Wrap(c.setCurrentVal(val))
}

func (c *concatenator) currentVal() (string, error) {
	var vals []string
	localVal, err := c.ctxt.GetConfig("val")
	if err != nil {
		return "", errors.Wrap(err)
	}
	if localVal != nil && localVal != "" {
		vals = append(vals, localVal.(string))
	}
	upstreamIds, err := c.ctxt.RelationIds("upstream")
	if err != nil {
		return "", errors.Wrap(err)
	}
	for _, id := range upstreamIds {
		units, err := c.ctxt.RelationUnits(id)
		if err != nil {
			return "", errors.Wrap(err)
		}
		for _, unit := range units {
			val, err := c.ctxt.GetRelationUnit(id, unit, "val")
			if err != nil {
				return "", errors.Wrap(err)
			}
			vals = append(vals, val)
		}
	}
	return fmt.Sprintf("{%s}", strings.Join(vals, " ")), nil
}

func (c *concatenator) setCurrentVal(val string) error {
	ids, err := c.ctxt.RelationIds("downstream")
	if err != nil {
		return errors.Wrap(err)
	}
	c.ctxt.Logf("setting downstream relations %v to %s", ids, val)
	for _, id := range ids {
		if err := c.ctxt.SetRelationWithId(id, "val", val); err != nil {
			return errors.Wrapf(err, "cannot set relation %v", id)
		}
	}
	return nil
}

func (c *concatenator) notifyHTTPServer(currentVal string) error {
	// TODO how to implement this?
	// The problem is that we can't access the charmbits/httpserver
	// state at all - we currently assume a one-to-one correspondence
	// between hook context and hook, so there's no way
	// to create an httpserver.server from a context created for
	// a concatenator hook.
	//
	// Perhaps something like this?
	// // ContextGetter returns a function that transforms an existing
	// // context into a context with state local to r.
	// // If the context is already local to r, it returns its argument.
	// // When the returned context is closed, any local state
	// // is saved.
	// func (r *Registry) ContextGetter() func(*Context) *Context {
	// }
	//
	// BUT! perhaps this might be better if we allow getting local
	// state more than once, for instance by mandating that we
	// pass a pointer-to-pointer into LocalState, so it
	// can update it to point to an existing local state value
	// if need be (no need to panic if called twice).
	//
	// Then we don't need to bother saving state after each hook
	// execution or when a context created as above is closed.
	//
	// When we have the above, we can return a newServer function
	// from httpserver.Register:
	//	func(ctxt *hook.Context) *httpserver.Server
	// and provide:
	//	func (srv *httpServer.Server) LocalURL() (string, error)
	return nil
}

func nothing(ctxt *hook.Context) error {
	return nil
}
