// The runhook package implements a
// charm that takes all the string values from units on upstream
// relations, concatenates them and makes them available to downstream
// relations.
package runhook

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/juju/utils"
	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/charmbits/httpcharm"
	"github.com/juju/gocharm/hook"
)

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
	concat.http.Register(r.NewRegistry("httpserver"), "http", newHandler)
	r.RegisterContext(concat.setContext)
	r.RegisterHook("upstream-relation-changed", concat.changed)
	r.RegisterHook("upstream-relation-departed", concat.changed)
	r.RegisterHook("downstream-relation-joined", concat.changed)
	r.RegisterHook("config-changed", concat.changed)
}

type concatenator struct {
	ctxt *hook.Context
	http  httpcharm.Provider
}

func (c *concatenator) setContext(ctxt *hook.Context) error {
	c.ctxt = ctxt
	return nil
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

var shortAttempt = utils.AttemptStrategy{
	Total: 50 * time.Millisecond,
	Delay: 5 * time.Millisecond,
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
	// The web server may be notionally started not be actually
	// running yet, so try for a short while if it fails.
	for a := shortAttempt.Start(); a.Next(); {
		err := c.notifyHTTPServer(val)
		if err == nil {
			return nil
		}
		if !a.HasNext() {
			return errors.Wrap(err)
		}
	}
	panic("unreachable")
}

func (c *concatenator) notifyHTTPServer(currentVal string) error {
	c.ctxt.Logf("notifyHTTPServer of %q", currentVal)
	addr, err := c.http.PrivateAddress()
	if err != nil {
		return errors.Wrap(err)
	}
	if addr == "" {
		c.ctxt.Logf("cannot notify local http server because it's not running")
		return nil
	}
	req, err := http.NewRequest("PUT", fmt.Sprintf("http://%s/val", addr), strings.NewReader(currentVal))
	if err != nil {
		return errors.Wrap(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Newf("http request failed: %v", resp.Status)
	}
	return nil
}

func nothing(ctxt *hook.Context) error {
	return nil
}
