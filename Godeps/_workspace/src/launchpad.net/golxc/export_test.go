// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

package golxc

// ContainerHome returns the name of the container directory.
func ContainerHome(c Container) string {
	return c.(*container).containerHome()
}

// SetConfPath allows the manipulation of the LXC
// default configuration file.
func SetConfPath(cp string) string {
	orig := confPath
	confPath = cp
	return orig
}
