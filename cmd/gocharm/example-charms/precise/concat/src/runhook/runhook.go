package runhook
import (
	"fmt"
	"strings"

	"launchpad.net/juju-utils/hook"
	"launchpad.net/errgo/errors"
)

// This charm takes all the string values from units on upstream
// relations, concatenates them and makes them available to downstream
// relations.

func RegisterHooks() {
	hook.Register("install", install)
	hook.Register("start", start)
	hook.Register("config-changed", changed)
	hook.Register("upstream-relation-changed", changed)
	hook.Register("upstream-relation-departed", changed)
	hook.Register("downstream-relation-joined", changed)
}

func changed(ctxt *hook.Context) error {
	var vals []string
	localVal, err := ctxt.GetConfig("val")
	if err != nil {
		return errors.Wrap(err)
	}
	if localVal != nil && localVal != "" {
		vals = append(vals, localVal.(string))
	}
	upstreamIds, err := ctxt.RelationIds("upstream")
	if err != nil {
		return errors.Wrap(err)
	}
	for _, id := range upstreamIds {
		units, err := ctxt.RelationUnits(id)
		if err != nil {
			return errors.Wrap(err)
		}
		for _, unit := range units {
			val, err := ctxt.GetRelationUnit(id, unit, "val")
			if err != nil {
				return errors.Wrap(err)
			}
			vals = append(vals, val)
		}
	}
	val := fmt.Sprintf("{%s}", strings.Join(vals, " "))
	ids, err := ctxt.RelationIds("downstream")
	if err != nil {
		return errors.Wrap(err)
	}
	ctxt.Logf("setting downstream relations %v to %s", ids, val)
	for _, id := range ids {
		if err := ctxt.SetRelationWithId(id, "val", val); err != nil {
			return errors.Wrapf(err, "cannot set relation %v", id)
		}
	}
	return nil
}

func install(ctxt *hook.Context) error {
	ctxt.Logf("install")
	return nil
}

func start(ctxt *hook.Context) error {
	ctxt.Logf("start")
	return nil
}
