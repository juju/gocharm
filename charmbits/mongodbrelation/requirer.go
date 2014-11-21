// The mongodbrelation package implements a Juju mongodb relation.
package mongodbrelation

import (
	"net"
	"strings"

	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/charmbits/simplerelation"
	"github.com/juju/gocharm/hook"
)

// Requirer represents the requirer side of an mongodb relation.
type Requirer struct {
	req simplerelation.Requirer
}

// Register registers an mongodb requirer relation with the given
// relation name with the given hook registry.
//
// To find out when the host addresses change, register
// a wildcard ("*") hook.
func (req *Requirer) Register(r *hook.Registry, relationName string) {
	req.req.Register(r, relationName, "mongodb")
}

// Addresses returns the addresses of the current mongodb servers.
func (req *Requirer) Addresses() []string {
	return req.req.Strings(unitAddress)
}

// URL returns a URL suitable for passing to mgo.Dial.
// If there are no current addresses, it returns the
// empty string.
// TODO does this work with IPv6?
func (req *Requirer) URL() string {
	addrs := req.Addresses()
	if len(addrs) == 0 {
		return ""
	}
	return "mongodb://" + strings.Join(addrs, ",")
}

func unitAddress(vals map[string]string) (string, error) {
	host := vals["hostname"]
	if host == "" {
		return "", nil
	}
	port := vals["port"]
	if port == "" {
		return "", errors.Newf("mongodb host %q found with no port", host)
	}
	return net.JoinHostPort(host, port), nil
}
