// The helloworld-configurable package implements an example
// charm similar to http://godoc.org/github.com/juju/gocharm/example-charms/helloworld
// but which allows the message to be configured, demonstrating how
// changing configuration options can affect a running service.
//
// Once deployed, the message can be changed with:
// 	juju set helloworld-configurable message='my new message'
package runhook

import (
	"fmt"
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/gocharm/charmbits/httpservice"
	"github.com/juju/gocharm/hook"
)

func RegisterHooks(r *hook.Registry) {
	var hw helloWorld
	hw.svc.Register(r.Clone("httpservice"), "", "webserver", hw.handler)
	r.RegisterHook("config-changed", func() error { return nil })
	r.RegisterConfig("message", charm.Option{
		Type:        "string",
		Description: "message to serve",
		Default:     "hello, world",
	})
	r.RegisterHook("*", hw.changed)
	r.RegisterContext(hw.setContext, nil)
}

type helloWorld struct {
	ctxt *hook.Context
	svc  httpservice.Service
}

func (hw *helloWorld) setContext(ctxt *hook.Context) error {
	hw.ctxt = ctxt
	return nil
}

func (hw *helloWorld) changed() error {
	message, err := hw.ctxt.GetConfigString("message")
	if err != nil {
		return errgo.Notef(err, "cannot get message from configuration")
	}
	return hw.svc.Start(&params{
		Message: message,
	})
}

type params struct {
	Message string
}

func (hw *helloWorld) handler(p *params) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, p.Message)
	}), nil
}
