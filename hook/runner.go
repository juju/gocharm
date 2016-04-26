package hook

import (
	"bytes"
	"net/rpc"
	"os"
	osexec "os/exec"
	"strings"

	"github.com/juju/utils/exec"
	"gopkg.in/errgo.v1"
)

// ToolRunner is used to run hook tools.
type ToolRunner interface {
	// Run runs the hook tool with the given name
	// and arguments, and returns its standard output.
	// If the command is unimplemented, it should
	// return an error with an ErrUnimplemented cause.
	Run(cmd string, args ...string) (stdout []byte, err error)
	Close() error
}

var (
	// execHookTools specifes whether the hook tools should be called
	// using os/exec (the conventional way). When this is false,
	// hook tools are invoked directly through the unix-domain socket
	// (technically breaking abstraction boundaries but 250 times faster
	// and easier to test)
	execHookTools = false

	// jujucSymlinks specifies whether we invoke the hook tools by name.
	// For testing purposes, setting this to false means that an installed
	// version of jujud can be used to test the hook logic without creating
	// symbolic links for all the hook tools.
	//
	// This only has an effect when useJujudSocket is false.
	jujucSymlinks = true
)

type socketToolRunner struct {
	contextId   string
	jujucClient *rpc.Client
}

// newToolRunnerFromEnvironment returns an implementation of ToolRunner
// that uses a direct connection to the unit agent's socket to
// run the tools.
func newToolRunnerFromEnvironment() (ToolRunner, error) {
	if execHookTools {
		return execToolRunner{}, nil
	}
	path := os.Getenv(envSocketPath)
	if path == "" {
		return nil, errgo.New("no juju socket found")
	}
	contextId := os.Getenv(envJujuContextId)
	if contextId == "" {
		return nil, errgo.New("no context id found")
	}
	client, err := rpc.Dial("unix", os.Getenv(envSocketPath))
	if err != nil {
		return nil, errgo.Newf("cannot dial uniter: %v", err)
	}
	return &socketToolRunner{
		contextId:   contextId,
		jujucClient: client,
	}, nil
}

// jujucRequest contains the information necessary to run a Command
// remotely.
//
// It is copied from github.com/juju/juju/worker/uniter/context/jujuc so
// that we can avoid that dependency in non-test code.
type jujucRequest struct {
	ContextId   string
	Dir         string
	CommandName string
	Args        []string

	// StdinSet indicates whether or not the client supplied stdin. This is
	// necessary as Stdin will be nil if the client supplied stdin but it
	// is empty.
	StdinSet bool
	Stdin    []byte
}

func isUnimplemented(errStr string) bool {
	return strings.HasPrefix(errStr, "bad request: unknown command")
}

var ErrUnimplemented = errgo.New("unimplemented hook tool")

func (r *socketToolRunner) Run(cmd string, args ...string) (stdout []byte, err error) {
	req := jujucRequest{
		ContextId: r.contextId,
		// We will never use a command that uses a path name,
		// but jujuc checks for an absolute path.
		Dir:         "/",
		CommandName: cmd,
		Args:        args,
	}
	var resp exec.ExecResponse
	err = r.jujucClient.Call("Jujuc.Main", req, &resp)
	if err != nil {
		if isUnimplemented(err.Error()) {
			return nil, errgo.WithCausef(err, ErrUnimplemented, "")
		}
		return nil, errgo.Newf("cannot call jujuc.Main: %v", err)
	}
	if resp.Code == 0 {
		return resp.Stdout, nil
	}
	errText := strings.TrimSpace(string(resp.Stderr))
	errText = strings.TrimPrefix(errText, "error: ")
	return nil, errgo.New(errText)
}

func (r *socketToolRunner) Close() error {
	return errgo.Mask(r.jujucClient.Close())
}

type execToolRunner struct{}

func (execToolRunner) Run(cmd string, args ...string) ([]byte, error) {
	execCmd := cmd
	if !jujucSymlinks {
		execCmd = "jujud"
	}
	c := osexec.Command(execCmd, args...)
	c.Args[0] = cmd
	var errBuf, outBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf

	if err := c.Run(); err != nil {
		if errBuf.Len() > 0 {
			errText := strings.TrimSpace(errBuf.String())
			errText = strings.TrimPrefix(errText, "error: ")
			if isUnimplemented(errText) {
				return nil, errgo.WithCausef(nil, ErrUnimplemented, "%s", errText)
			}
			return nil, errgo.New(errText)
		}
		return nil, err
	}
	return outBuf.Bytes(), nil
}

func (execToolRunner) Close() error {
	return nil
}
