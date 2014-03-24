// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

package golxc

import (
	"fmt"
	"strings"
)

const (
	defaultAddr   = "10.0.3.1"
	defaultBridge = "lxcbr0"
)

// StartNetwork starts the lxc network subsystem.
func StartNetwork() error {
	goal, _, err := networkStatus()
	if err != nil {
		return err
	}
	if goal != "start" {
		_, err := run("start", "lxc-net")
		if err != nil {
			return err
		}
	}
	return nil
}

// StopNetwork stops the lxc network subsystem.
func StopNetwork() error {
	goal, _, err := networkStatus()
	if err != nil {
		return err
	}
	if goal != "stop" {
		_, err := run("stop", "lxc-net")
		if err != nil {
			return err
		}
	}
	return nil
}

// IsNotworkRunning checks if the lxc network subsystem
// is running.
func IsNetworkRunning() (bool, error) {
	_, status, err := networkStatus()
	if err != nil {
		return false, err
	}
	return status == "running", nil
}

// NetworkAttributes returns the lxc network attributes: 
// starting IP address and bridge name.
func NetworkAttributes() (addr, bridge string, err error) {
	config, err := ReadConf()
	if err != nil {
		return "", "", err
	}
	addr = config["address"]
	if addr == "" {
		addr = defaultAddr
	}
	bridge = config["bridge"]
	if bridge == "" {
		bridge = defaultBridge
	}
	return addr, bridge, nil
}

// networkStatus returns the status of the lxc network subsystem.
func networkStatus() (goal, status string, err error) {
	output, err := run("status", "lxc-net")
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("unexpected status output: %q", output)
	}
	fields = strings.Split(fields[1], "/")
	if len(fields) != 2 {
		return "", "", fmt.Errorf("unexpected status output: %q", output)
	}
	return fields[0], fields[1], nil
}
