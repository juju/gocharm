// Package deploy implements functionality for
// building and deploying Go charms.
//
// See the example for how the pieces fit together.
//
// Once built, a gocharm command can build itself
// (with the -build-charm flag) and deploy itself
// (with the -deploy-charm flag).
package deploy

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/juju/gocharm/hook"
	errgo "gopkg.in/errgo.v1"
)

var (
	deployFlag     string
	buildFlag      string
	runHookFlag    string
	noCompressFlag bool
)

// MainFlags adds charm flags to the global flags.
func MainFlags() {
	flag.StringVar(&deployFlag, "deploy-charm", "", "deploy as Juju charm - argument is name of service to use")
	flag.StringVar(&buildFlag, "build-charm", "", "build Juju charm - argument is path to directory to write charm to")
	flag.StringVar(&runHookFlag, "run-hook", "", "run as charm hook")
	flag.BoolVar(&noCompressFlag, "no-charm-compress", false, "disable charm binary compression")
}

var (
	errUsage           = errgo.New("usage error")
	errNotCharmCommand = errgo.New("not a charm command")
)

// RunMain registers any additional hooks needed for all charms and runs
// any charm build, deploy or hook logic. It assumes that
// MainFlags and flag.Parse has been called previously.
//
// If any charm-specific flags are set, it will not return - it just
// exits, otherwise it returns having done nothing else.
func RunMain(r *hook.Registry) {
	err := runMain(r)
	switch errgo.Cause(err) {
	case errUsage:
		flag.Usage()
		os.Exit(2)
	case errNotCharmCommand:
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runMain(r *hook.Registry) error {
	hook.RegisterMainHooks(r)
	switch {
	case runHookFlag != "":
		if err := hookMain(r, runHookFlag, flag.Args()); err != nil {
			return errgo.Mask(err)
		}
	case deployFlag == "" && buildFlag == "":
		return errNotCharmCommand
	case deployFlag != "" && buildFlag != "":
		return errUsage
	case deployFlag != "":
		dir, err := ioutil.TempDir("", "")
		if err != nil {
			return errgo.Notef(err, "cannot make temp dir")
		}
		exe, err := os.Executable()
		if err != nil {
			return errgo.Notef(err, "cannot find executable")
		}
		if err := BuildCharm(BuildCharmParams{
			Registry:   r,
			CharmDir:   dir,
			HookBinary: exe,
			NoCompress: noCompressFlag,
		}); err != nil {
			return errgo.Notef(err, "cannot build charm")
		}
		if err := deployCharm(dir, deployFlag); err != nil {
			return errgo.Notef(err, "cannot deploy charm")
		}
	case buildFlag != "":
		exe, err := os.Executable()
		if err != nil {
			return errgo.Notef(err, "cannot find executable")
		}
		if err := BuildCharm(BuildCharmParams{
			Registry:   r,
			CharmDir:   buildFlag,
			HookBinary: exe,
			NoCompress: noCompressFlag,
		}); err != nil {
			return errgo.Notef(err, "cannot build charm")
		}
	}
	return nil
}

func hookMain(r *hook.Registry, hookName string, args []string) error {
	// TODO would /etc/init be a better place for local state?
	ctxt, state, err := hook.NewContextFromEnvironment(r, "/var/lib/juju-localstate", hookName, args)
	if err != nil {
		return errgo.Notef(err, "cannot create context")
	}
	defer ctxt.Close()
	cmd, err := hook.Main(r, ctxt, state)
	if err != nil {
		return errgo.Mask(err)
	}
	if cmd == nil {
		return nil
	}
	if err := cmd.Wait(); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func deployCharm(charmDir string, appName string) error {
	cmd := exec.Command("juju", "deploy", charmDir, appName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errgo.Notef(err, "cannot deploy charm")
	}
	return nil
}
