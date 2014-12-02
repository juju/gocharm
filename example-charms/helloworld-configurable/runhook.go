package runhook

import (
	"fmt"
	"net/http"

	"gopkg.in/juju/charm.v4"
	"launchpad.net/errgo/errors"

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
		return errors.Wrapf(err, "cannot get message from configuration")
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
