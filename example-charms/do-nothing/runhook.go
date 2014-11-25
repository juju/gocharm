// Package mycharm implements the simplest possible Go charm.
// It does nothing at all when deployed.
package mycharm

import (
	"github.com/juju/gocharm/hook"
)

type nothing struct {
	ctxt *hook.Context
}

func RegisterHooks(r *hook.Registry) {
	var n nothing
	r.RegisterContext(n.setContext, nil)
	r.RegisterHook("install", n.hook)
	r.RegisterHook("start", n.hook)
	r.RegisterHook("config-changed", n.hook)
}

func (n *nothing) setContext(ctxt *hook.Context) error {
	n.ctxt = ctxt
	return nil
}

func (n *nothing) hook() error {
	n.ctxt.Logf("hook %s is doing nothing at all", n.ctxt.HookName)
	return nil
}
