package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju"
	_ "github.com/juju/juju/provider/all"
)

var help = `
juju-wait waits for the unit with the given name to reach a status
matching the given anchored regular expression.  The pattern matches
against the status code followed by a colon, a space and the status information
if there is some status information.

For example, a unit that encountered an error running the install hook
would match the regular expression 'error: hook failed: "install"'.

If the unit is removed, the status is 'removed'.

Juju-wait returns a non-zero exit status if there is an error connecting
to the juju environment, if the unit does not exist when juju-wait starts,
or if the unit is removed and the pattern does not match "removed".

For example: to wait for a unit to enter a stable state:

juju-wait wordpress/0 'error:.*|started'
`

var envName = flag.String("e", "", "environment name")
var verbose = flag.Bool("v", false, "print non-matching states to standard error as they are seen")
var debug = flag.Bool("debug", false, "print debugging messages to standard error")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: juju-wait [flags] unit-name regexp\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "%s", help)
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
	}
	unitName := flag.Arg(0)
	statusPattern, err := regexp.Compile("^(" + flag.Arg(1) + ")$")
	if err != nil {
		fatalf("invalid status regular expression: %v", err)
	}
	if *debug {
		//		log.SetTarget(stdlog.New(os.Stderr, "", stdlog.LstdFlags))
	}
	if err := juju.InitJujuHome(); err != nil {
		fatalf("cannot initialise juju home: %v", err)
	}
	client, err := juju.NewAPIClientFromName("")
	if err != nil {
		fatalf("cannot open API: %v", err)
	}
	w, err := client.WatchAll()
	if err != nil {
		fatalf("cannot watch all: %v", err)
	}
	defer w.Stop()
	unit, err := wait(unitName, statusPattern, w)
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Printf("%s\n", status(unit))
}

type watcher interface {
	Next() ([]params.Delta, error)
}

func wait(unitName string, statusPattern *regexp.Regexp, w watcher) (*params.UnitInfo, error) {
	var unit *params.UnitInfo
	for {
		deltas, err := w.Next()
		if err != nil {
			return nil, fmt.Errorf("cannot get next deltas: %v", err)
		}
		var found *params.UnitInfo
		var removed bool
		for _, d := range deltas {
			if u, ok := d.Entity.(*params.UnitInfo); ok && u.Name == unitName {
				found = u
				removed = d.Removed
			}
		}
		if found == nil {
			// The first set of deltas should contain information about
			// all entities in the environment.
			if unit == nil {
				return nil, fmt.Errorf("unit %q does not exist", unitName)
			}
			continue
		}
		unit = found
		if removed {
			unit.Status, unit.StatusInfo = "removed", ""
		}
		if statusPattern.MatchString(status(unit)) {
			break
		}
		if *verbose {
			fmt.Fprintf(os.Stderr, "%s\n", status(unit))
		}
		if removed {
			return nil, fmt.Errorf("unit was removed")
		}
	}
	return unit, nil
}

func status(unit *params.UnitInfo) string {
	s := string(unit.Status)
	if unit.StatusInfo != "" {
		s += ": " + unit.StatusInfo
	}
	return s
}

func fatalf(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "juju-wait: %s\n", fmt.Sprintf(f, args...))
	os.Exit(2)
}
