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
	"launchpad.net/errgo/errors"

	"gopkg.in/juju-utils.v0/charmbits/httpserver"
	"gopkg.in/juju-utils.v0/hook"
)

func RegisterHooks(r *hook.Registry) {
	getServer := httpserver.Register(r.NewRegistry("httpserver"), newHandler)
	register := func(hookName string, f func(*concatenator) error) {
		r.Register(hookName, func(ctxt *hook.Context) error {
			c, err := newConcatenator(ctxt, getServer)
			if err != nil {
				return errors.Wrap(err)
			}
			return f(c)
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
	srv  *httpserver.Server
}

func newConcatenator(ctxt *hook.Context, newServer httpserver.ServerGetter) (*concatenator, error) {
	srv, err := newServer(ctxt)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return &concatenator{
		ctxt: ctxt,
		srv:  srv,
	}, nil
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
	addr, err := c.srv.PrivateAddress()
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
