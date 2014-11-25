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

Support for cross-series / architecture compilation.
-----------------------------

The default destination charm series should be taken from the current series.
We should support a -arch flag to compile for other architectures, and
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

Naming
------

Perhaps rename charmbits/*charm to charmbits/*relation
e.g. httprelation.Provider.
