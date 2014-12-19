// The helloworld package implements an example charm that
// exposes an HTTP service that returns "hello, world"
// from every endpoint.
//
// To deploy it, first build it:
//
//    gocharm github.com/juju/gocharm/example-charms/helloworld
//
// Then deploy it to a juju environment and expose the service:
//
//	juju deploy local:trusty/helloworld
//	juju expose helloworld
//
// Then, when the helloworld unit has started, find the public
// address of it and check that it works:
//
//	curl http://<address>/
//
// The port that it serves on can be configured with:
//
//	juju set helloworld http-port=12345
//
// It can also be configured to serve on https by
// setting the https-certificate configuration option
// to a PEM-format certificate and private key.
//
// See http://godoc.org/github.com/juju/gocharm/example-charms/helloworld-configurable
// for a slightly more advanced version of this charm
// that allows the message to be configured.
package helloworld

import (
	"fmt"
	"net/http"

	"github.com/juju/gocharm/charmbits/httpservice"
	"github.com/juju/gocharm/hook"
)

func RegisterHooks(r *hook.Registry) {
	var hw helloWorld
	hw.svc.Register(r.Clone("httpservice"), "", "webserver", hw.handler)
	r.RegisterHook("*", hw.start)
}

type helloWorld struct {
	svc httpservice.Service
}

func (hw *helloWorld) start() error {
	return hw.svc.Start(struct{}{})
}

func (hw *helloWorld) handler(struct{}) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello, world")
	}), nil
}
