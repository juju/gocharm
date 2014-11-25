Gocharm - write your Juju charms in Go.
----------------------

Gocharm is a framework that makes it easy to write charms in
[Go](http://golang.org), with the benefits that Go brings: static typing,
minimal dependencies and fun!

See the hook [package docs](http://godoc.org/github.com/juju/gocharm/hook)
for documentation on the core Go API and the gocharm [(tool docs](http://godoc.org/github.com/juju/gocharm/cmd/gocharm)
for how to use the gocharm tool itself.

Its features include:

- all hooks are automatically generated. No need to create a hook file
ever again.

- config.yaml and the relations in metadata.yaml are also automatically
generated.

- built in support for persistent state in hooks.

- packages implementing common relation interface types.

- support for using the deployed Go charm binary as the running charm
service itself.

- a choice between deployment in "binary only" mode or with all source
included in the charm and a Go development environment installed to aid
in hook debugging.

A simple example charm
------------------------

This example assumes you already have a working Go development
environment and $GOPATH set up. It also assumes a current
working Juju environment.

Define a Go charm by creating a Go package containing the following
code. Choose any name for the package. For this example, I'll
name it github.com/juju/gocharm/example-charms/donothing
and store the code in runhook.go.

	package mycharm
	import (
		"github.com/juju/gocharm/hook"
	)
	func RegisterHooks(r *hook.Registry) {
		// Here we will register all any code to be called
		// when the charm runs, and any relations or
		// configuration values defined by the charm.
	}

Like the main function for main packages, the RegisterHooks function is the
top entry point for a charm. This function will be called in two contexts,
when the charm is built by the gocharm command, and when the
charm actually executes as part of a deployed unit. Thus the
code in RegisterHooks must not assume that it is part of a deployed
unit (which goes also for any packages or code called by
RegisterHooks).

This particular charm will do absolutely nothing when run,
because it registers no hooks. It is a valid charm package nonetheless.

We also need a metadata.yaml file in the package directory to hold the description and
summary (and other metadata files too, such as tags) of the charm.
Unlike a normal charm metadata.yaml, this will *not* hold any relations declared by the
charm - they will be added at build time.

	name: do-nothing
	summary: 'any example that does nothing at all'
	description: |
	    This example charm does nothing.


To deploy it as a charm, we'll need to install it in our local
juju charm repository. This is configured by setting the $JUJU_REPOSITORY
environment variable.

For example, in your shell:

	$ export JUJU_REPOSITORY=$HOME/charms

You will need the gocharm command:

	$ go get github.com/juju/gocharm/cmd/gocharm

Then, in your package directory, run the gocharm command:

	$ cd $GOPATH/src/github.com/juju/gocharm/example-charms/gosimple
	$ gocharm
	local:trusty/do-nothing

This has now compiled the charm and deployed it to your local
juju charm repository.

	$ tree $JUJU_REPOSITORY/trusty/do-nothing
	/home/rog/charms/trusty/do-nothing
	├── bin
	│   └── runhook
	├── hooks
	│   ├── install
	│   └── start
	├── metadata.yaml
	└── src
	    ├── github.com
	    │   └── juju
	    │       └── gocharm
	    │           └── example-charms
	    │               └── do-nothing
	    │                   ├── metadata.yaml
	    │                   └── runhook.go
	    └── runhook
	        └── runhook.go
	
	9 directories, 7 files

Note that the source code for the charm (but not that of all dependencies)
has been copied to the charm directory, which is now a valid root for
a GOPATH directory. The compiled binary which will run the hooks has
been installed into bin/runhook.  The hooks directory has been created
and populated with an install and a start hook, neither of which will
do anything when run.

	$ cat $JUJU_REPOSITORY/trusty/do-nothing/hooks/install
	#!/bin/sh
	set -ex
	
	$CHARM_DIR/bin/runhook install

The charm is now ready to be deployed.

	$ juju deploy local:trusty/do-nothing
	Added charm "local:trusty/do-nothing-0" to the environment.

After a little while, we see that the charm has deployed OK:

	% juju status do-nothing
	environment: local
	machines:
	  "2":
	    agent-state: started
	    agent-version: 1.22-alpha1.1
	    dns-name: 10.0.3.184
	    instance-id: rog-local-machine-2
	    series: trusty
	    hardware: arch=amd64
	services:
	  do-nothing:
	    charm: local:trusty/do-nothing-0
	    exposed: false
	    units:
	      do-nothing/0:
	        agent-state: started
	        agent-version: 1.22-alpha1.1
	        machine: "2"
	        public-address: 10.0.3.184

Of course, since the charm does nothing, there's not much to observe
that it's working correctly.

Let's change runhook.go to register some hooks and to send a log message so that
we can see something from the charm.

The start of the charm package remains the same.

	// Package mycharm implements the simplest possible Go charm.
	// It does nothing at all when deployed.
	package mycharm
	
	import (
		"github.com/juju/gocharm/hook"
	)

We declare a type for the charm implementation. Although
not strictly necessary, this is a common pattern. When the
charm is running a hook, the ctxt field will hold the running
hook context.
	
	type nothing struct {
		ctxt *hook.Context
	}

We change the RegisterHooks function so that it actually registers some
things. We declare an instance of the nothing type (there will actually
only ever be one instance of this type at runtime), and register some
of its methods to be called when the hook runs.

	func RegisterHooks(r *hook.Registry) {
		var n nothing
		r.RegisterContext(n.setContext, nil)
		r.RegisterHook("install", n.hook)
		r.RegisterHook("start", n.hook)
		r.RegisterHook("config-changed", n.hook)
	}

When the hook runs, this setContext method is called first.
It lets the charm code know about the running hook context,
which enables it to use all the usual charm tools and enquire
about configuration and relation settings.

	func (n *nothing) setContext(ctxt *hook.Context) error {
		n.ctxt = ctxt
		return nil
	}

Here is the actual hook code that runs in response to the hook.
We just log a message to show that something is happening:

	func (n *nothing) hook() error {
		n.ctxt.Logf("hook %s is doing nothing at all", n.ctxt.HookName)
		return nil
	}

Having saved the file, we run gocharm again, and we can see that
another hook file has been generated because we registered the
"config-changed" hook.

	$ gocharm
	$  tree $JUJU_REPOSITORY/trusty/do-nothing/hooks
	/home/rog/charms/trusty/do-nothing/hooks
	├── config-changed
	├── install
	└── start
	
	0 directories, 3 files

We can upgrade the existing service in place:

	$ juju upgrade-charm do-nothing
	Added charm "local:trusty/do-nothing-1" to the environment.

After a little while we can verify that the charm has changed:

	$  juju ssh do-nothing/0 'sudo grep "doing nothing" /var/log/juju/unit-do-nothing-0.log'
	Warning: Permanently added '10.0.3.184' (ECDSA) to the list of known hosts.
	2014-11-25 13:08:46 INFO unit.do-nothing/0.juju-log cmd.go:247 hook config-changed is doing nothing at all
	Connection to 10.0.3.184 closed.

Note that because we upgraded the charm, neither the install or start hooks
are called again - only the config-changed hook.

TODO using persistent state

TODO importing relation packages

TODO using service package.
