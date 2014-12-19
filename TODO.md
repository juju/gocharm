Support godeps
----------------

We should implement the godeps flag as a lighter weight alternative
to the -source mode.

Upgrading
--------

For this to work, we should copy the charm hook
binary somewhere else at install time,
which also makes sense when we're running
the charm binary as an external service.
This can be done by the charmbits/service package.

Is upgrade-charm called before or after the
charm has been upgraded?

We could also check local state for backward
compatibility.

Compressed binary support
----------------------

We could support a gzipped executable and uncompress
at install/upgrade-charm time.
Given that we are not always guaranteed to call upgrade-charm,
we should probably compare runhook.gz and runhook binary
mtimes on every hook execution and uncompress when
runhook.gz is updated.

Testing
------

Support for testing service-based charms.
Change RegisterCommand so that instead of taking
a func(args []string), it takes a func(args []string) (Worker, error),
where: 

	type Worker interface {
		Kill()
		Wait() error
	}

This will enable testing code to stop the service that's
been started (and also potentially allow graceful shutdown
when the service gets a non-lethal signal)

Support for cross-series / architecture compilation.
-----------------------------

The default destination charm series should be taken from the current series.
We should support a -build <architecture> flag to compile for other architectures, and
have some way for the hooks to know what binary to run
depending on the host architecture.

Possible for command line flags for the future:
-----------------------------------

	-deploy
		deploy to an environment directly
	-e [environment]
		name the environment to deploy to.
	-upload
		upload to a charm store.

With -deploy, we can just make a repository in /tmp before deploying it to juju.

Logging
------

Integration of service output with charm output might be nice.
At least some better visisibility of service output would be good.

HTTP service
----------

We should not pass all the arguments in the upstart
file and restart every time anything changes. We could
implement a better scheme that passes
the arguments through the socket and restarts the server
only when necessary.
