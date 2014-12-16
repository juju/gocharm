package service

import (
	"strings"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
)

// OSServiceParams holds the parameters for
// creating a new service.
type OSServiceParams struct {
	// Name holds the name of the service.
	Name string

	// Description holds the description of the service.
	Description string

	// Output holds the file where output will be put.
	Output string

	// Exe holds the name of the executable to run.
	Exe string

	// Args holds any arguments to the executable,
	// which should be OK to to pass to the shell
	// without quoting.
	Args []string
}

// NewService is used to create a new service.
// It is defined as a variable so that it can be
// replaced for testing purposes.
var NewService = func(p OSServiceParams) OSService {
	cmd := p.Exe + " " + strings.Join(p.Args, " ")
	return &upstart.Service{
		Name: p.Name,
		Conf: common.Conf{
			InitDir: "/etc/init",
			Desc:    p.Description,
			Cmd:     cmd,
			Out:     p.Output,
		},
	}
}
