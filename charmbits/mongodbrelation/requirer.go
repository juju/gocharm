// The mongodbrelation package implements a Juju mongodb relation.
//
// Importing this package also registers the type
// *mgo.Session with the httpservice package,
// so that it can be used as a relation field with httpservice.Service.Register.
//
// For example, to register a web service that requires a MongoDB connection,
// you might do something like this:
//
//	type relations struct {
//		Session *mgo.Session	`httpservice:"mongodb"`
//	}
//	func RegisterHooks(r *hook.Registry) {
// 		var svc httpservice.Service
//		svc.Register(r, "somename", "http", func(_ struct{}, rel *relations) (httpservice.Handler, error) {
//			// rel.Session contains the actual MongoDB connection.
//			return newHandler(rel)
//		})
//	}
//
// This would create a Juju requirer relation named "mongodb"
// and dial the related MongoDB instance before creating
// the HTTP handler.
package mongodbrelation

import (
	"net"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/gocharm/charmbits/httpservice"
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
		return "", errgo.Newf("mongodb host %q found with no port", host)
	}
	return net.JoinHostPort(host, port), nil
}

func init() {
	httpservice.RegisterRelationType(
		registerField,
		setFieldValue,
	)
}

type relationInfo struct {
	URL string
}

func registerField(r *hook.Registry, tag string) func() (interface{}, error) {
	var req Requirer
	f := strings.Split(tag, ",")
	relationName := "mongodb"
	if f[0] != "" {
		relationName = f[0]
	}
	// get "required" value from tag too?
	req.Register(r, relationName)
	return func() (interface{}, error) {
		if url := req.URL(); url != "" {
			return &relationInfo{
				URL: req.URL(),
			}, nil
		}
		return nil, httpservice.ErrRelationIncomplete
	}
}

func setFieldValue(f **mgo.Session, info *relationInfo) error {
	if *f != nil {
		(*f).Close()
		*f = nil
		return httpservice.ErrRestartNeeded
	}
	if info == nil {
		return nil
	}
	session, err := mgo.Dial(info.URL)
	if err != nil {
		return errgo.Notef(err, "cannot dial mongo at %q", info.URL)
	}
	*f = session
	return nil
}
