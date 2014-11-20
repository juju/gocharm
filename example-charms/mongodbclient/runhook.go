package mongodbclient

import (
	"github.com/juju/gocharm/charmbits/mongodbrelation"
	"github.com/juju/gocharm/hook"
)

func RegisterHooks(r *hook.Registry) {
	var c charm
	r.RegisterContext(c.setContext, nil)
	c.mongodb.Register(r.Clone("mongodb"), "mongodb", nil)
	r.RegisterHook("*", c.changed)
}

type charm struct {
	ctxt    *hook.Context
	mongodb mongodbrelation.Requirer
}

func (c *charm) setContext(ctxt *hook.Context) error {
	c.ctxt = ctxt
	return nil
}

func (c *charm) changed() error {
	c.ctxt.Logf("mongo addresses are now %q", c.mongodb.Addresses())
	return nil
}
