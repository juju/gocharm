package httprelation

import (
	"net"
	"strings"

	"github.com/juju/gocharm/charmbits/simplerelation"
	"github.com/juju/gocharm/hook"
)

// Requirer represents the requirer of an http relation.
type Requirer struct {
	req simplerelation.Requirer
}

// Register registers everything necessary on r for
// running the provider side of an http relation with
// the given relation name.
func (req *Requirer) Register(r *hook.Registry, relationName string) {
	req.req.Register(r, relationName, "http")
}

// URLs returns the URLs of all the provider units.
func (req *Requirer) URLs() []string {
	return req.req.Strings(attrsToURL)
}

func attrsToURL(attrs map[string]string) (string, error) {
	host := attrs["hostname"]
	if host == "" {
		return "", nil
	}
	port := attrs["port"]
	if port == "" {
		port = "80"
	}
	hostPort := net.JoinHostPort(host, port)
	hostPort = strings.TrimSuffix(hostPort, ":80")
	return "http://" + hostPort, nil
}
