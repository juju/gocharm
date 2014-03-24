// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

package golxc_test

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/logging"
	. "launchpad.net/gocheck"

	"launchpad.net/golxc"
)

var lxcfile = `# MIRROR to be used by ubuntu template at container creation:
# Leaving it undefined is fine
#MIRROR="http://archive.ubuntu.com/ubuntu"
# or 
#MIRROR="http://<host-ip-addr>:3142/archive.ubuntu.com/ubuntu"

# LXC_AUTO - whether or not to start containers symlinked under
# /etc/lxc/auto
LXC_AUTO="true"

# Leave USE_LXC_BRIDGE as "true" if you want to use lxcbr0 for your
# containers.  Set to "false" if you'll use virbr0 or another existing
# bridge, or mavlan to your host's NIC.
USE_LXC_BRIDGE="true"

# LXC_BRIDGE and LXC_ADDR are changed against original for
# testing purposes.
# LXC_BRIDGE="lxcbr1"
LXC_BRIDGE="lxcbr9"
# LXC_ADDR="10.0.1.1"
LXC_ADDR="10.0.9.1"
LXC_NETMASK="255.255.255.0"
LXC_NETWORK="10.0.9.0/24"
LXC_DHCP_RANGE="10.0.9.2,10.0.9.254"
LXC_DHCP_MAX="253"
# And for testing LXC_BRIDGE="lxcbr99" and LXC_ADDR="10.0.99.1".

LXC_SHUTDOWN_TIMEOUT=120`

var lxcconf = map[string]string{
	"address": "10.0.9.1",
	"bridge":  "lxcbr9",
}

type ConfigSuite struct{}

var _ = Suite(&ConfigSuite{})

func (s *ConfigSuite) TestReadConf(c *C) {
	// Test reading the configuration.
	cf := filepath.Join(c.MkDir(), "lxc-test")
	c.Assert(ioutil.WriteFile(cf, []byte(lxcfile), 0555), IsNil)

	defer golxc.SetConfPath(golxc.SetConfPath(cf))

	conf, err := golxc.ReadConf()
	c.Assert(err, IsNil)
	c.Assert(conf, DeepEquals, lxcconf)
}

func (s *ConfigSuite) TestReadNotExistingDefaultEnvironment(c *C) {
	// Test reading a not existing environment.
	defer golxc.SetConfPath(golxc.SetConfPath(filepath.Join(c.MkDir(), "foo")))

	_, err := golxc.ReadConf()
	c.Assert(err, ErrorMatches, "open .*: no such file or directory")
}

func (s *ConfigSuite) TestNetworkAttributes(c *C) {
	// Test reading the network attribute form an environment.
	cf := filepath.Join(c.MkDir(), "lxc-test")
	c.Assert(ioutil.WriteFile(cf, []byte(lxcfile), 0555), IsNil)

	defer golxc.SetConfPath(golxc.SetConfPath(cf))

	addr, bridge, err := golxc.NetworkAttributes()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "10.0.9.1")
	c.Assert(bridge, Equals, "lxcbr9")
}

type NetworkSuite struct{}

var _ = Suite(&NetworkSuite{})

func (s *NetworkSuite) SetUpSuite(c *C) {
	u, err := user.Current()
	c.Assert(err, IsNil)
	if u.Uid != "0" {
		// Has to be run as root!
		c.Skip("tests must run as root")
	}
}

func (s *NetworkSuite) TestStartStopNetwork(c *C) {
	// Test starting and stoping of the LXC network.
	initialRunning, err := golxc.IsNetworkRunning()
	c.Assert(err, IsNil)
	defer func() {
		if initialRunning {
			c.Assert(golxc.StartNetwork(), IsNil)
		}
	}()
	c.Assert(golxc.StartNetwork(), IsNil)
	running, err := golxc.IsNetworkRunning()
	c.Assert(err, IsNil)
	c.Assert(running, Equals, true)
	c.Assert(golxc.StopNetwork(), IsNil)
	running, err = golxc.IsNetworkRunning()
	c.Assert(err, IsNil)
	c.Assert(running, Equals, false)
}

func (s *NetworkSuite) TestNotExistingNetworkAttributes(c *C) {
	// Test reading of network attributes from a not existing environment.
	defer golxc.SetConfPath(golxc.SetConfPath(filepath.Join(c.MkDir(), "foo")))

	_, _, err := golxc.NetworkAttributes()
	c.Assert(err, ErrorMatches, "open .*: no such file or directory")
}

type LXCSuite struct {
	factory golxc.ContainerFactory
}

var _ = Suite(&LXCSuite{golxc.Factory()})

func (s *LXCSuite) SetUpSuite(c *C) {
	u, err := user.Current()
	c.Assert(err, IsNil)
	if u.Uid != "0" {
		// Has to be run as root!
		c.Skip("tests must run as root")
	}
}

func (s *LXCSuite) createContainer(c *C) golxc.Container {
	container := s.factory.New("golxc")
	c.Assert(container.IsConstructed(), Equals, false)
	err := container.Create("", "ubuntu", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(container.IsConstructed(), Equals, true)
	return container
}

func (s *LXCSuite) TestCreateDestroy(c *C) {
	// Test clean creation and destroying of a container.
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	home := golxc.ContainerHome(lc)
	_, err := os.Stat(home)
	c.Assert(err, ErrorMatches, "stat .*: no such file or directory")
	err = lc.Create("", "ubuntu", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(lc.IsConstructed(), Equals, true)
	defer func() {
		err = lc.Destroy()
		c.Assert(err, IsNil)
		_, err = os.Stat(home)
		c.Assert(err, ErrorMatches, "stat .*: no such file or directory")
	}()
	fi, err := os.Stat(golxc.ContainerHome(lc))
	c.Assert(err, IsNil)
	c.Assert(fi.IsDir(), Equals, true)
}

func (s *LXCSuite) TestCreateTwice(c *C) {
	// Test that a container cannot be created twice.
	lc1 := s.createContainer(c)
	c.Assert(lc1.IsConstructed(), Equals, true)
	defer func() {
		c.Assert(lc1.Destroy(), IsNil)
	}()
	lc2 := s.factory.New("golxc")
	err := lc2.Create("", "ubuntu", nil, nil)
	c.Assert(err, ErrorMatches, "container .* is already created")
}

func (s *LXCSuite) TestCreateIllegalTemplate(c *C) {
	// Test that a container creation fails correctly in
	// case of an illegal template.
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	err := lc.Create("", "name-of-a-not-existing-template-for-golxc", nil, nil)
	c.Assert(err, ErrorMatches, `error executing "lxc-create": .*bad template.*`)
	c.Assert(lc.IsConstructed(), Equals, false)
}

func (s *LXCSuite) TestDestroyNotCreated(c *C) {
	// Test that a non-existing container can't be destroyed.
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	err := lc.Destroy()
	c.Assert(err, ErrorMatches, "container .* is not yet created")
}

func contains(lcs []golxc.Container, lc golxc.Container) bool {
	for _, clc := range lcs {
		if clc.Name() == lc.Name() {
			return true
		}
	}
	return false
}

func (s *LXCSuite) TestList(c *C) {
	// Test the listing of created containers.
	lcs, err := s.factory.List()
	oldLen := len(lcs)
	c.Assert(err, IsNil)
	c.Assert(oldLen >= 0, Equals, true)
	lc := s.createContainer(c)
	defer func() {
		c.Assert(lc.Destroy(), IsNil)
	}()
	lcs, _ = s.factory.List()
	newLen := len(lcs)
	c.Assert(newLen == oldLen+1, Equals, true)
	c.Assert(contains(lcs, lc), Equals, true)
}

func (s *LXCSuite) TestClone(c *C) {
	// Test the cloning of an existing container.
	lc1 := s.createContainer(c)
	defer func() {
		c.Assert(lc1.Destroy(), IsNil)
	}()
	lcs, _ := s.factory.List()
	oldLen := len(lcs)
	lc2, err := lc1.Clone("golxcclone", nil, nil)
	c.Assert(err, IsNil)
	c.Assert(lc2.IsConstructed(), Equals, true)
	defer func() {
		c.Assert(lc2.Destroy(), IsNil)
	}()
	lcs, _ = s.factory.List()
	newLen := len(lcs)
	c.Assert(newLen == oldLen+1, Equals, true)
	c.Assert(contains(lcs, lc1), Equals, true)
	c.Assert(contains(lcs, lc2), Equals, true)
}

func (s *LXCSuite) TestCloneNotCreated(c *C) {
	// Test the cloning of a non-existing container.
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	_, err := lc.Clone("golxcclone", nil, nil)
	c.Assert(err, ErrorMatches, "container .* is not yet created")
}

func (s *LXCSuite) TestStartStop(c *C) {
	// Test starting and stopping a container.
	lc := s.createContainer(c)
	defer func() {
		c.Assert(lc.Destroy(), IsNil)
	}()
	c.Assert(lc.Start("", ""), IsNil)
	c.Assert(lc.IsRunning(), Equals, true)
	c.Assert(lc.Stop(), IsNil)
	c.Assert(lc.IsRunning(), Equals, false)
}

func (s *LXCSuite) TestStartNotCreated(c *C) {
	// Test that a non-existing container can't be started.
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	c.Assert(lc.Start("", ""), ErrorMatches, "container .* is not yet created")
}

func (s *LXCSuite) TestStopNotRunning(c *C) {
	// Test that a not running container can't be stopped.
	lc := s.createContainer(c)
	defer func() {
		c.Assert(lc.Destroy(), IsNil)
	}()
	c.Assert(lc.Stop(), IsNil)
}

func (s *LXCSuite) TestWait(c *C) {
	// Test waiting for one of a number of states of a container.
	// ATTN: Using a not reached state blocks the test until timeout!
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	c.Assert(lc.Wait(), ErrorMatches, "no states specified")
	c.Assert(lc.Wait(golxc.StateStopped), IsNil)
	c.Assert(lc.Wait(golxc.StateStopped, golxc.StateRunning), IsNil)
	c.Assert(lc.Create("", "ubuntu", nil, nil), IsNil)
	defer func() {
		c.Assert(lc.Destroy(), IsNil)
	}()
	go func() {
		c.Assert(lc.Start("", ""), IsNil)
	}()
	c.Assert(lc.Wait(golxc.StateRunning), IsNil)
}

func (s *LXCSuite) TestFreezeUnfreeze(c *C) {
	// Test the freezing and unfreezing of a started container.
	lc := s.createContainer(c)
	defer func() {
		c.Assert(lc.Destroy(), IsNil)
	}()
	c.Assert(lc.Start("", ""), IsNil)
	defer func() {
		c.Assert(lc.Stop(), IsNil)
	}()
	c.Assert(lc.IsRunning(), Equals, true)
	c.Assert(lc.Freeze(), IsNil)
	c.Assert(lc.IsRunning(), Equals, false)
	c.Assert(lc.Unfreeze(), IsNil)
	c.Assert(lc.IsRunning(), Equals, true)
}

func (s *LXCSuite) TestFreezeNotStarted(c *C) {
	// Test that a not running container can't be frozen.
	lc := s.createContainer(c)
	defer func() {
		c.Assert(lc.Destroy(), IsNil)
	}()
	c.Assert(lc.Freeze(), ErrorMatches, "container .* is not running")
}

func (s *LXCSuite) TestFreezeNotCreated(c *C) {
	// Test that a non-existing container can't be frozen.
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	c.Assert(lc.Freeze(), ErrorMatches, "container .* is not yet created")
}

func (s *LXCSuite) TestUnfreezeNotCreated(c *C) {
	// Test that a non-existing container can't be unfrozen.
	lc := s.factory.New("golxc")
	c.Assert(lc.IsConstructed(), Equals, false)
	c.Assert(lc.Unfreeze(), ErrorMatches, "container .* is not yet created")
}

func (s *LXCSuite) TestUnfreezeNotFrozen(c *C) {
	// Test that a running container can't be unfrozen.
	lc := s.createContainer(c)
	defer func() {
		c.Assert(lc.Destroy(), IsNil)
	}()
	c.Assert(lc.Start("", ""), IsNil)
	defer func() {
		c.Assert(lc.Stop(), IsNil)
	}()
	c.Assert(lc.Unfreeze(), ErrorMatches, "container .* is not frozen")
}

type commandArgs struct {
	logging.LoggingSuite
}

var _ = Suite(&commandArgs{})

func (s *commandArgs) TestCreateArgs(c *C) {
	s.PatchValue(&golxc.ContainerDir, c.MkDir())
	testing.PatchExecutableAsEchoArgs(c, s, "lxc-create")

	factory := golxc.Factory()
	container := factory.New("test")
	err := container.Create(
		"config-file", "template",
		[]string{"extra-1", "extra-2"},
		[]string{"template-1", "template-2"},
	)
	c.Assert(err, IsNil)
	testing.AssertEchoArgs(
		c, "lxc-create",
		"-n", "test",
		"-t", "template",
		"-f", "config-file",
		"extra-1", "extra-2",
		"--", "template-1", "template-2")
}

func (s *commandArgs) TestCloneArgs(c *C) {
	dir := c.MkDir()
	s.PatchValue(&golxc.ContainerDir, dir)
	// Patch lxc-info too as clone checks to see if it is running.
	testing.PatchExecutableAsEchoArgs(c, s, "lxc-info")
	testing.PatchExecutableAsEchoArgs(c, s, "lxc-clone")

	factory := golxc.Factory()
	container := factory.New("test")
	// Make the rootfs for the "test" container so it thinks it is created.
	rootfs := filepath.Join(dir, "test", "rootfs")
	err := os.MkdirAll(rootfs, 0755)
	c.Assert(err, IsNil)
	c.Assert(rootfs, jc.IsDirectory)
	c.Assert(container.IsConstructed(), jc.IsTrue)
	clone, err := container.Clone(
		"name",
		[]string{"extra-1", "extra-2"},
		[]string{"template-1", "template-2"},
	)
	c.Assert(err, IsNil)
	testing.AssertEchoArgs(
		c, "lxc-clone",
		"-o", "test",
		"-n", "name",
		"extra-1", "extra-2",
		"--", "template-1", "template-2")
	c.Assert(clone.Name(), Equals, "name")
}
