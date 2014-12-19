// Package do-nothing implements the simplest possible Go charm.
// It does nothing at all when deployed.
package runhook

import (
	"github.com/juju/gocharm/hook"
)

func RegisterHooks(r *hook.Registry) {
}
