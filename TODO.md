Different build modes.
----------------

- build in advance (what we do currently)
- build at deploy time, pulling in deps using godeps
- build at deploy time, using vendored deps
- rebuild on every hook execution (config option?)

Also, it may well be better to support a "deploy to"
mode rather than changing the charm in place
as it does currently.

    gocharm upload local:trusty/mycharm

	- we have to be careful about this, as it may
	require overwriting the target.

    gocharm upload cs:trusty/mycharm

Generate start and install hooks
------------------------

Currently we require them to be specified.

Upgrading
--------

For this to work, we should copy the charm hook
binary somewhere else at install time,
which also makes sense when we're running
the charm binary as an external service.

Is upgrade-charm called before or after the
charm has been upgraded?

We could also check local state for backward
compatibility.

Misc
----

Use gopkg.in/errgo.v1 package.

Avoid mutation of charms in place
---------------------------

Just use normal Go package. Perhaps the package name must be "runhook"
(along the same lines as "main").

	gocharm [-series $series1,$series2...] [-f] [-build architecture] [-vendor] [-godeps] [-name $charmname] $packagepath 

Make new charm directory, $d

cp -r $packagepath to $d/src/$packagepath

except for:
	assets -> $d/assets
	README.md -> $d/README.md
	metadata.yaml -> $d/metadata.yaml (with relations added)

runhook.go gets generated and put into $d/runhook.go, importing
from $packagepath.

runhook gets compiled and put into $d/bin/runhook

We put the resulting charm in $JUJU_REPOSITORY by default.
If there's anything in the target directory that's not in:

	src/...
	bin/...
	assets/...
	README*
	metadata.yaml
	config.yaml

then we abort; otherwise we wipe out everything from the
target directory and replace it with stuff copied from the charm
package as specified above.


Possible for command line flags for the future:
-----------------------------------

	-deploy
		deploy to an environment directly
	-e [environment]
		name the environment to deploy to.
	-upload
		upload to a charm store.

With -deploy, we can just make a repository in /tmp before deploying it to juju.


Simplify LocalState
---------------

If we remove the "name" argument, we make it so that
any failure to call NewRegistry when passing a registry
to other modules will fail more quickly.

We could even make it so that LocalState is called
at initial Registry time; perhaps make the local state
an argument to RegisterContext. Then all the checking
can happen at gocharm time.

Is this general enough?

Possible scenarios where it might not be:
	we want an arbitrary number of pieces of local state.
	- this can always be emulated by use of a map.

It would be nice to lose the need for the name argument
to NewRegistry.

Rename NewRegistry
----------------

	func (r *Registry) Clone(name string) *Registry
