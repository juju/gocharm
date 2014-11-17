package hook

import (
	"net/rpc"
	"os"
	"strings"

	"github.com/juju/juju/worker/uniter/context/jujuc"
	"github.com/juju/utils/exec"
	"launchpad.net/errgo/errors"
)

// ToolRunner is used to run hook tools.
type ToolRunner interface {
	// Run runs the hook tool with the given name
	// and arguments, and returns its standard output.
	Run(cmd string, args ...string) (stdout []byte, err error)
	Close() error
}

type socketToolRunner struct {
	contextId   string
	jujucClient *rpc.Client
}

// newToolRunnerFromEnvironment returns an implementation of ToolRunner
// that uses a direct connection to the unit agent's socket to
// run the tools.
func newToolRunnerFromEnvironment() (ToolRunner, error) {
	path := os.Getenv(envSocketPath)
	if path == "" {
		return nil, errors.New("no juju socket found")
	}
	contextId := os.Getenv(envJujuContextId)
	if contextId == "" {
		return nil, errors.New("no context id found")
	}
	client, err := rpc.Dial("unix", os.Getenv(envSocketPath))
	if err != nil {
		return nil, errors.Newf("cannot dial uniter: %v", err)
	}
	return &socketToolRunner{
		contextId:   contextId,
		jujucClient: client,
	}, nil
}

func (r *socketToolRunner) Run(cmd string, args ...string) (stdout []byte, err error) {
	req := jujuc.Request{
		ContextId: r.contextId,
		// We will never use a command that uses a path name,
		// but jujuc checks for an absolute path.
		Dir:         "/",
		CommandName: cmd,
		Args:        args,
	}
	// log.Printf("run req %#v", req)
	var resp exec.ExecResponse
	err = r.jujucClient.Call("Jujuc.Main", req, &resp)
	if err != nil {
		return nil, errors.Newf("cannot call jujuc.Main: %v", err)
	}
	if resp.Code == 0 {
		return resp.Stdout, nil
	}
	errText := strings.TrimSpace(string(resp.Stderr))
	if strings.HasPrefix(errText, "error: ") {
		errText = errText[len("error: "):]
	}
	return nil, errors.New(errText)
}

func (r *socketToolRunner) Close() error {
	return errors.Wrap(r.jujucClient.Close())
}
