package main

func inspectRegisteredHooks() {
	

var inspectCode = `
package main
import (
	"launchpad.net/juju-utils/hook"
	_ "hooks"
)

func main() {
	for _, name := range hook.RegisteredHooks() {
		fmt.Printf("%s\n", name)
	}
}
`

