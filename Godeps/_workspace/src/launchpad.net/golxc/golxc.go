// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

// golxc - Go package to interact with Linux Containers (LXC).
//
// https://launchpad.net/golxc/
//
package golxc

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("golxc")

// Error reports the failure of a LXC command.
type Error struct {
	Name   string
	Err    error
	Output []string
}

func (e Error) Error() string {
	if e.Output == nil {
		return fmt.Sprintf("error executing %q: %v", e.Name, e.Err)
	}
	if len(e.Output) == 1 {
		return fmt.Sprintf("error executing %q: %v", e.Name, e.Output[0])
	}
	return fmt.Sprintf("error executing %q: %s", e.Name, strings.Join(e.Output, "; "))
}

// State represents a container state.
type State string

const (
	StateUnknown  State = "UNKNOWN"
	StateStopped  State = "STOPPED"
	StateStarting State = "STARTING"
	StateRunning  State = "RUNNING"
	StateAborting State = "ABORTING"
	StateStopping State = "STOPPING"
)

// LogLevel represents a container's log level.
type LogLevel string

const (
	LogDebug    LogLevel = "DEBUG"
	LogInfo     LogLevel = "INFO"
	LogNotice   LogLevel = "NOTICE"
	LogWarning  LogLevel = "WARN"
	LogError    LogLevel = "ERROR"
	LogCritical LogLevel = "CRIT"
	LogFatal    LogLevel = "FATAL"
)

// Container represents a linux container instance and provides
// operations to create, maintain and destroy the container.
type Container interface {

	// Name returns the name of the container.
	Name() string

	// Create creates a new container based on the given template.
	Create(configFile, template string, extraArgs []string, templateArgs []string) error

	// Start runs the container as a daemon.
	Start(configFile, consoleFile string) error

	// Stop terminates the running container.
	Stop() error

	// Clone creates a copy of the container, giving the copy the specified name.
	Clone(name string, extraArgs []string, templateArgs []string) (Container, error)

	// Freeze freezes all the container's processes.
	Freeze() error

	// Unfreeze thaws all frozen container's processes.
	Unfreeze() error

	// Destroy stops and removes the container.
	Destroy() error

	// Wait waits for one of the specified container states.
	Wait(states ...State) error

	// Info returns the status and the process id of the container.
	Info() (State, int, error)

	// IsConstructed checks if the container image exists.
	IsConstructed() bool

	// IsRunning checks if the state of the container is 'RUNNING'.
	IsRunning() bool

	// String returns information about the container, like the name, state,
	// and process id.
	String() string

	// LogFile returns the current filename used for the LogFile.
	LogFile() string

	// LogLevel returns the current logging level (only used if the
	// LogFile is not "").
	LogLevel() LogLevel

	// SetLogFile sets both the LogFile and LogLevel.
	SetLogFile(filename string, level LogLevel)
}

// ContainerFactory represents the methods used to create Containers.
type ContainerFactory interface {
	// New returns a container instance which can then be used for operations
	// like Create(), Start(), Stop() or Destroy().
	New(string) Container

	// List returns all the existing containers on the system.
	List() ([]Container, error)
}

// Factory provides the standard ContainerFactory.
func Factory() ContainerFactory {
	return &containerFactory{}
}

const DefaultLXCDir = "/var/lib/lxc"

var ContainerDir = DefaultLXCDir

type container struct {
	name     string
	logFile  string
	logLevel LogLevel
	// Newer LXC libraries can have containers in non-default locations. The
	// containerDir is the directory that is the 'home' of this container.
	containerDir string
}

type containerFactory struct{}

func (*containerFactory) New(name string) Container {
	return &container{
		name:         name,
		logLevel:     LogWarning,
		containerDir: ContainerDir,
	}
}

// List returns all the existing containers on the system.
func (factory *containerFactory) List() ([]Container, error) {
	out, err := run("lxc-ls", "-1")
	if err != nil {
		return nil, err
	}
	names := nameSet(out)
	containers := make([]Container, len(names))
	for i, name := range names {
		containers[i] = factory.New(name)
	}
	return containers, nil
}

// Name returns the name of the container.
func (c *container) Name() string {
	return c.name
}

// LogFile returns the current filename used for the LogFile.
func (c *container) LogFile() string {
	return c.logFile
}

// LogLevel returns the current logging level, this is only used if the
// LogFile is not "".
func (c *container) LogLevel() LogLevel {
	return c.logLevel
}

// SetLogFile sets both the LogFile and LogLevel.
func (c *container) SetLogFile(filename string, level LogLevel) {
	c.logFile = filename
	c.logLevel = level
}

// Create creates a new container based on the given template.
func (c *container) Create(configFile, template string, extraArgs []string, templateArgs []string) error {
	if c.IsConstructed() {
		return fmt.Errorf("container %q is already created", c.Name())
	}
	args := []string{
		"-n", c.name,
		"-t", template,
	}
	if configFile != "" {
		args = append(args, "-f", configFile)
	}
	if len(extraArgs) != 0 {
		args = append(args, extraArgs...)
	}
	if len(templateArgs) != 0 {
		// Must be done in two steps due to current language implementation details.
		args = append(args, "--")
		args = append(args, templateArgs...)
	}
	_, err := run("lxc-create", args...)
	if err != nil {
		return err
	}
	return nil
}

// Start runs the container as a daemon.
func (c *container) Start(configFile, consoleFile string) error {
	if !c.IsConstructed() {
		return fmt.Errorf("container %q is not yet created", c.name)
	}
	args := []string{
		"--daemon",
		"-n", c.name,
	}
	if configFile != "" {
		args = append(args, "-f", configFile)
	}
	if consoleFile != "" {
		args = append(args, "-c", consoleFile)
	}
	if c.logFile != "" {
		args = append(args, "-o", c.logFile, "-l", string(c.logLevel))
	}
	_, err := run("lxc-start", args...)
	if err != nil {
		return err
	}
	if err := c.Wait(StateRunning, StateStopped); err != nil {
		return err
	}
	if !c.IsRunning() {
		return fmt.Errorf("container failed to start")
	}
	return nil
}

// Stop terminates the running container.
func (c *container) Stop() error {
	if !c.IsConstructed() {
		return fmt.Errorf("container %q is not yet created", c.name)
	}
	// If the container is not running, we are done.
	if !c.IsRunning() {
		return nil
	}
	args := []string{
		"-n", c.name,
	}
	if c.logFile != "" {
		args = append(args, "-o", c.logFile, "-l", string(c.logLevel))
	}
	_, err := run("lxc-stop", args...)
	if err != nil {
		return err
	}
	return c.Wait(StateStopped)
}

// Clone creates a copy of the container, it gets the given name.
func (c *container) Clone(name string, extraArgs []string, templateArgs []string) (Container, error) {
	if !c.IsConstructed() {
		return nil, fmt.Errorf("container %q is not yet created", c.name)
	}
	if c.IsRunning() {
		return nil, fmt.Errorf("cannot clone a running container")
	}
	cc := &container{
		name:         name,
		logLevel:     c.logLevel,
		containerDir: c.containerDir,
	}
	if cc.IsConstructed() {
		return cc, nil
	}
	args := []string{
		"-o", c.name,
		"-n", name,
	}
	if len(extraArgs) != 0 {
		args = append(args, extraArgs...)
	}
	if len(templateArgs) != 0 {
		// Must be done in two steps due to current language implementation details.
		args = append(args, "--")
		args = append(args, templateArgs...)
	}
	_, err := run("lxc-clone", args...)
	if err != nil {
		return nil, err
	}
	return cc, nil
}

// Freeze freezes all the container's processes.
func (c *container) Freeze() error {
	if !c.IsConstructed() {
		return fmt.Errorf("container %q is not yet created", c.name)
	}
	if !c.IsRunning() {
		return fmt.Errorf("container %q is not running", c.name)
	}
	args := []string{
		"-n", c.name,
	}
	if c.logFile != "" {
		args = append(args, "-o", c.logFile, "-l", string(c.logLevel))
	}
	_, err := run("lxc-freeze", args...)
	if err != nil {
		return err
	}
	return nil
}

// Unfreeze thaws all frozen container's processes.
func (c *container) Unfreeze() error {
	if !c.IsConstructed() {
		return fmt.Errorf("container %q is not yet created", c.name)
	}
	if c.IsRunning() {
		return fmt.Errorf("container %q is not frozen", c.name)
	}
	args := []string{
		"-n", c.name,
	}
	if c.logFile != "" {
		args = append(args, "-o", c.logFile, "-l", string(c.logLevel))
	}
	_, err := run("lxc-unfreeze", args...)
	if err != nil {
		return err
	}
	return nil
}

// Destroy stops and removes the container.
func (c *container) Destroy() error {
	if !c.IsConstructed() {
		return fmt.Errorf("container %q is not yet created", c.name)
	}
	if err := c.Stop(); err != nil {
		return err
	}
	_, err := run("lxc-destroy", "-n", c.name)
	if err != nil {
		return err
	}
	return nil
}

// Wait waits for one of the specified container states.
func (c *container) Wait(states ...State) error {
	if len(states) == 0 {
		return fmt.Errorf("no states specified")
	}
	stateStrs := make([]string, len(states))
	for i, state := range states {
		stateStrs[i] = string(state)
	}
	waitStates := strings.Join(stateStrs, "|")
	_, err := run("lxc-wait", "-n", c.name, "-s", waitStates)
	if err != nil {
		return err
	}
	return nil
}

// Info returns the status and the process id of the container.
func (c *container) Info() (State, int, error) {
	out, err := run("lxc-info", "-n", c.name)
	if err != nil {
		return StateUnknown, -1, err
	}
	kv := keyValues(out, ": ")
	state := State(kv["state"])
	pid, err := strconv.Atoi(kv["pid"])
	if err != nil {
		return StateUnknown, -1, fmt.Errorf("cannot read the pid: %v", err)
	}
	return state, pid, nil
}

// IsConstructed checks if the container image exists.
func (c *container) IsConstructed() bool {
	fi, err := os.Stat(c.rootfs())
	if err != nil {
		return false
	}
	return fi.IsDir()
}

// IsRunning checks if the state of the container is 'RUNNING'.
func (c *container) IsRunning() bool {
	state, _, err := c.Info()
	if err != nil {
		return false
	}
	return state == StateRunning
}

// String returns information about the container.
func (c *container) String() string {
	state, pid, err := c.Info()
	if err != nil {
		return fmt.Sprintf("cannot retrieve container info for %q: %v", c.name, err)
	}
	return fmt.Sprintf("container %q (%s, pid %d)", c.name, state, pid)
}

// containerHome returns the name of the container directory.
func (c *container) containerHome() string {
	return path.Join(c.containerDir, c.name)
}

// rootfs returns the name of the directory containing the
// root filesystem of the container.
func (c *container) rootfs() string {
	return path.Join(c.containerHome(), "rootfs")
}

// run executes the passed command and returns the out.
func run(name string, args ...string) (string, error) {
	logger := loggo.GetLogger(fmt.Sprintf("golxc.run.%s", name))
	logger.Tracef("run: %s %v", name, args)
	cmd := exec.Command(name, args...)
	// LXC tools do not use stdout and stderr in a predictable
	// way; based on experimentation, the most convenient
	// solution is to combine them and leave the client to
	// determine sanity as best it can.
	out, err := cmd.CombinedOutput()
	result := string(out)
	if err != nil {
		logger.Tracef("run failed output: %s", result)
		return "", runError(name, err, out)
	}
	logger.Tracef("run successful output: %s", result)
	return result, nil
}

// runError creates an error if run fails.
func runError(name string, err error, out []byte) error {
	e := &Error{name, err, nil}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, name+": ") {
			// LXC tools do not always print their output with
			// the command name as prefix. The name is part of
			// the error struct, so stip it from the output if
			// printed.
			l = l[len(name)+2:]
		}
		if l != "" {
			e.Output = append(e.Output, l)
		}
	}
	return e
}

// keyValues retrieves key/value pairs out of a command out.
func keyValues(raw string, sep string) map[string]string {
	kv := map[string]string{}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, sep, 2)
		if len(parts) == 2 {
			kv[strings.ToLower(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return kv
}

// nameSet retrieves a set of names out of a command out.
func nameSet(raw string) []string {
	collector := map[string]struct{}{}
	set := []string{}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			collector[name] = struct{}{}
		}
	}
	for name := range collector {
		set = append(set, name)
	}
	return set
}
