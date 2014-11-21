// The elasticsearchrelation package implements a Juju elasticsearch relation.
package elasticsearchrelation

import (
	"net"

	"github.com/juju/gocharm/charmbits/simplerelation"
	"github.com/juju/gocharm/hook"
)

// Requirer represents the requirer side of an elasticsearch
// relation.
type Requirer struct {
	req simplerelation.Requirer
}

// Register registers an elasticsearch requirer relation with the given
// relation name with the given hook registry.
//
// To find out when the host addresses change, register
// a wildcard ("*") hook.
func (req *Requirer) Register(r *hook.Registry, relationName string) {
	req.req.Register(r, relationName, "elasticsearch")
}

// Addresses returns the addresses of the current elasticsearch servers.
func (req *Requirer) Addresses() []string {
	return req.req.Strings(unitAddress)
}

func unitAddress(vals map[string]string) (string, error) {
	host := vals["host"]
	if host == "" {
		return "", nil
	}
	port := vals["port"]
	if port == "" {
		port = "9200"
	}
	return net.JoinHostPort(host, port), nil
}
