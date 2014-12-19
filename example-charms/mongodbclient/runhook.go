// The mongodbclient package implements an example charm that
// acts as the client of the mongodb charm. Note that
// a real charm that used this would pass the mongo
// URL to a service that would make the actual connection
// to mongo.
package mongodbclient

import (
	"github.com/juju/gocharm/charmbits/mongodbrelation"
	"github.com/juju/gocharm/hook"
)

func RegisterHooks(r *hook.Registry) {
	var c charm
	r.RegisterContext(c.setContext, nil)
	c.mongodb.Register(r.Clone("mongodb"), "mongodb")
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
	c.ctxt.Logf("mongo URL is now %q", c.mongodb.URL())
	return nil
}
