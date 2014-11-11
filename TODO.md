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

Support for relation interfaces
-----------------------

mongodb requirer
elasticsearch requirer
http requirer

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

