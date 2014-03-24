// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

package golxc

import (
	"io/ioutil"
	"regexp"
)

var (
	confPath string         = "/etc/default/lxc"
	addrRE   *regexp.Regexp = regexp.MustCompile(`\n\s*LXC_ADDR="(\d+.\d+.\d+.\d+)"`)
	bridgeRE *regexp.Regexp = regexp.MustCompile(`\n\s*LXC_BRIDGE="(\w+)"`)
)

// ReadConf reads the LXC network address and bridge interface
// out of the configuration file /etc/default/lxc.
func ReadConf() (map[string]string, error) {
	conf := make(map[string]string)
	confData, err := ioutil.ReadFile(confPath)
	if err != nil {
		return nil, err
	}
	fetchValue := func(field string, re *regexp.Regexp) {
		groups := re.FindStringSubmatch(string(confData))
		if len(groups) == 2 {
			conf[field] = groups[1]
		}
	}
	fetchValue("address", addrRE)
	fetchValue("bridge", bridgeRE)
	return conf, nil
}
