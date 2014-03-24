// +build gccgo

package hook

import (
	"runtime"
	"strings"
)

// currentServiceMethodName returns the method executing on the service when ProcessControlHook was invoked.
func (s *TestService) currentServiceMethodName() string {
	// We have to go deeper into the stack with gccgo because in a situation like:
	// type Inner { }
	// func (i *Inner) meth {}
	// type Outer { Inner }
	// o = &Outer{}
	// o.meth()
	// gccgo generates a method called "meth" on *Outer, and this shows up
	// on the stack as seen by runtime.Caller (this might be a gccgo bug).

	pc, _, _, ok := runtime.Caller(3)
	if !ok {
		panic("current method name cannot be found")
	}
	return unqualifiedMethodName(pc)
}

func unqualifiedMethodName(pc uintptr) string {
	f := runtime.FuncForPC(pc)
	fullName := f.Name()
	// This is very fragile.  fullName will be something like:
	// launchpad.net_goose_testservices_novaservice.removeServer.pN49_launchpad.net_goose_testservices_novaservice.Nova
	// so if the number of dots in the full package path changes,
	// this will need to too...
	nameParts := strings.Split(fullName, ".")
	return nameParts[2]
}
