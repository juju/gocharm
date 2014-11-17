package mongodbcharm

import (
	"net"
	"sort"

	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"

	"github.com/juju/gocharm/hook"
)

// Interface holds the value that can be informed of the
// addresses of a set of mongoDB servers.
// The addresses are formatted as addresses
// required by the net package, except that the
// port part is optional.
type Interface interface {
	MongoDBAddressesChanged(addrs []string) error
}

type Requirer struct {
	ctxt         *hook.Context
	relationName string
	intf         Interface
	state        localState
}

type localState struct {
	Addresses []string
}

// Register registers a mongodb requirer relation with the given
// relation name with the given hook registry. When the
// mongo addresses change, intf.MongoDBAddresses will
// be called with the new addresses.
func (req *Requirer) Register(r *hook.Registry, relationName string, intf Interface) {
	req.relationName = relationName
	req.intf = intf
	r.RegisterContext(req.setContext, &req.state)
	r.RegisterRelation(charm.Relation{
		Name:      relationName,
		Interface: "mongodb",
		Role:      charm.RoleRequirer,
	})
	r.RegisterHook("relation-"+req.relationName+"-joined", req.changed)
	r.RegisterHook("relation-"+req.relationName+"-changed", req.changed)
	r.RegisterHook("relation-"+req.relationName+"-left", req.changed)
}

func (req *Requirer) setContext(ctxt *hook.Context) error {
	req.ctxt = ctxt
	return nil
}

func (req *Requirer) changed() error {
	addrs, err := req.currentAddresses()
	if err != nil {
		return errors.Wrapf(err, "cannot get current addresses")
	}
	if stringsEqual(addrs, req.state.Addresses) {
		return nil
	}
	req.state.Addresses = addrs
	if err := req.intf.MongoDBAddressesChanged(addrs); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (req *Requirer) Addresses() []string {
	return req.state.Addresses
}

func (req *Requirer) currentAddresses() ([]string, error) {
	ids := req.ctxt.RelationIds[req.relationName]
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 1 {
		req.ctxt.Logf("more than one provider for the %s relation", req.relationName)
		return nil, nil
	}
	id := ids[0]
	var addrs []string
	for _, vals := range req.ctxt.Relations[id] {
		host := vals["hostname"]
		if host == "" {
			continue
		}
		port := vals["port"]
		if port == "" {
			req.ctxt.Logf("mongo host %q found with no port", host)
			continue
		}
		addrs = append(addrs, net.JoinHostPort(host, port))
	}
	sort.Strings(addrs)
	return addrs, nil
}

func stringsEqual(s, t []string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := range s {
		if s[i] != t[i] {
			return false
		}
	}
	return true
}
