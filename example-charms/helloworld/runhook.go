package runhook

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
