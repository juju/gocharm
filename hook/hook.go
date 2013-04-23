// The hook package provides a Go interface to the
// charm hook commands.
package hook
import (
	"encoding/json"
	"errors"
	"fmt"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"net/rpc"
	"os"
	"strings"
	"sync"
)

// Variables valid for all hooks
var (
	EnvUUID = os.Getenv("JUJU_ENV_UUID")
	Unit = os.Getenv("JUJU_UNIT_NAME")
	CharmDir = os.Getenv("CHARM_DIR")
)

// Variables valid for relation-related hooks.
var (
	RelationName = os.Getenv("JUJU_RELATION")
	RelationId = os.Getenv("JUJU_RELATION_ID")
	RemoteUnit = os.Getenv("JUJU_REMOTE_UNIT")
)

var (
	dialJujucOnce sync.Once
	jujucClient *rpc.Client
	jujucContextId = os.Getenv("JUJU_CONTEXT_ID")
)

func IsRelationHook() bool {
	return RelationName != ""
}

func OpenPort(proto string, port int) error {
	_, err := run("open-port", fmt.Sprintf("%d/%s", port, proto))
	return err
}

func ClosePort(proto string, port int) error {
	_, err := run("close-port", fmt.Sprintf("%d/%s", port, proto))
	return err
}

// PrivateAddress returns the public address of the local unit.
func PublicAddress() (string, error) {
	out, err := run("unit-get", "public-address")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// PrivateAddress returns the private address of the local unit.
func PrivateAddress()  (string, error) {
	out, err := run("unit-get", "private-address")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Log logs a message through the juju logging facility.
func Log(msg string) error {
	_, err := run("juju-log", msg)
	return err
}

// GetRelation returns the value with the given key from the
// relation and unit that triggered the hook execution.
// It is equivalent to GetRelationUnit(key, RelationId, RemoteUnit).
func GetRelation(key string) (string, error) {
	return GetRelationUnit(key, RelationId, RemoteUnit)
}

// GetRelationUnit returns the value with the given key
// from the given unit associated with the relation with the
// given id.
func GetRelationUnit(key string, relationId, unit string) (string, error) {
	var val string
	if err := runJson(&val, "relation-get", "--format", "json", "--", key, unit); err != nil {
		return "", err
	}
	return val, nil
}

// GetAllRelation returns all the settings for the relation
// and unit that triggered the hook execution.
// It is equivalent to GetAllRelationUnit(RelationId, RemoteUnit).
func GetAllRelation() (map[string] string, error) {
	return GetAllRelationUnit(RelationId, RemoteUnit)
}

// GetAllRelationUnit returns all the settings from the given unit associated
// with the relation with the given id.
func GetAllRelationUnit(relationId, unit string) (map[string] string, error) {
	var val map[string] string
	if err := runJson(&val, "relation-get", "-r", relationId, "--format", "json", "--", "-", unit); err != nil {
		return nil, err
	}
	return val, nil
}

// RelationIds returns all the relation ids associated
// with the relation with the given name,
func RelationIds(relationName string) ([]string, error) {
	var val []string
	if err := runJson(&val, "relation-ids", "--format", "json", "--", relationName); err != nil {
		return nil, err
	}
	return val, nil
}

func RelationUnits(relationId string) ([]string, error) {
	var val []string
	if err := runJson(&val, "relation-list", "--format", "json", "--", relationId); err != nil {
		return nil, err
	}
	return val, nil
}

// AllRelationUnits returns a map from all the relation ids
// for the relation with the given name to all the
// units with that name
func AllRelationUnits(relationName string) (map[string] []string, error) {
	allUnits := make(map[string] []string)
	ids, err := RelationIds(relationName)
	if err != nil {
		return nil, fmt.Errorf("cannot get relation ids: %v", err)
	}
	for _, id := range ids {
		units, err := RelationUnits(id)
		if err != nil {
			return nil, fmt.Errorf("cannot get relation units for id %q: %v", id, err)
		}
		allUnits[id] = units
	}
	return allUnits, nil
}

// SetRelation sets the given key-value pairs on the current relation instance.
func SetRelation(keyvals ...string) error {
	return SetRelationWithId(RelationId, keyvals...)
}

// SetRelationWithId sets the given key-value pairs
// on the relation with the given id.
func SetRelationWithId(relationId string, keyvals ...string) error {
	if len(keyvals) % 2 != 0 {
		return fmt.Errorf("invalid key/value count")
	}
	args := make([]string, 0, 3+len(keyvals)/2)
	args = append(args, "-r", relationId, "--")
	for i := 0; i < len(keyvals); i += 2 {
		args = append(args, fmt.Sprintf("%s=%s", keyvals[i], keyvals[i+1]))
	}
	_, err := run("relation-set", args...)
	return err
}

func GetConfig(key string) (interface{}, error) {
	var val interface{}
	if err := runJson(&val, "config-get", "--format", "json", "--", key); err != nil {
		return nil, err
	}
	return val, nil
}

func GetAllConfig() (map[string]interface{}, error) {
	var val map[string]interface{}
	if err := runJson(&val, "config-get", "--format", "json"); err != nil {
		return nil, err
	}
	return val, nil
}

func dialJujuc() {
	socketPath := os.Getenv("JUJU_AGENT_SOCKET")
	if socketPath == "" || jujucContextId == "" {
		panic("launchpad.net/juju-utils/hook used in non-hook context")
	}
	client, err := rpc.Dial("unix", socketPath)
	if err != nil {
		panic(fmt.Errorf("cannot dial uniter: %v", err))
	}
	jujucClient = client
}

func run(cmd string, args ...string) (stdout []byte, err error) {
	dialJujucOnce.Do(dialJujuc)
	req := jujuc.Request{
		ContextId:   jujucContextId,
		// We will never use a command that uses a path name,
		// but jujuc checks for an absolute path.
		Dir:         "/",
		CommandName: cmd,
		Args:        args[1:],
	}
	var resp jujuc.Response
	err = jujucClient.Call("Jujuc.Main", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("cannot call jujuc.Main: %v", err)
	}
	if resp.Code == 0 {
		return resp.Stdout, nil
	}
	errText := strings.TrimSpace(string(resp.Stderr))
	return nil, errors.New(errText)
}

func runJson(dst interface{}, cmd string, args ...string) error {
	out, err := run(cmd, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, dst); err != nil {
		return fmt.Errorf("cannot parse command output %q into %T: %v", out, dst, err)
	}
	return nil
}
