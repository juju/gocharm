package hook_test

// This rest of this file is an near-exact copy of
// github.com/juju/juju/worker/uniter/jujuc/util_test.go

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/context/jujuc"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v4"
	. "launchpad.net/gocheck"
)

func TestPackage(t *testing.T) { TestingT(t) }

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

func GetHookServerContext(c *C, relid int, remote string) *ServerContext {
	rels := map[int]*ServerContextRelation{
		0: {
			id:   0,
			name: "peer0",
			units: map[string]Settings{
				"peer0/0": {"private-address": "peer0-0.example.com"},
				"peer0/1": {"private-address": "peer0-1.example.com"},
			},
		},
		1: {
			id:   1,
			name: "peer1",
			units: map[string]Settings{
				"peer1/0": {"private-address": "peer1-0.example.com"},
				"peer1/1": {"private-address": "peer1-1.example.com"},
			},
		},
	}
	if relid != -1 {
		_, found := rels[relid]
		c.Assert(found, Equals, true)
	}
	return &ServerContext{
		relid:  relid,
		remote: remote,
		rels:   rels,
	}
}

type ServerContext struct {
	jujuc.Context

	ports  set.Strings
	relid  int
	remote string
	rels   map[int]*ServerContextRelation
}

func (s *ServerContext) ActionParams() (map[string]interface{}, error) {
	return map[string]interface{}{
		"actionParam": "something",
	}, nil
}

func (c *ServerContext) OwnerTag() string {
	return "unknown"
}

func (c *ServerContext) UnitName() string {
	return "u/0"
}

func (c *ServerContext) PublicAddress() (string, bool) {
	return "gimli.minecraft.example.com", true
}

func (c *ServerContext) PrivateAddress() (string, bool) {
	return "192.168.0.99", true
}

func (c *ServerContext) OpenPort(protocol string, port int) error {
	c.ports.Add(fmt.Sprintf("%d/%s", port, protocol))
	return nil
}

func (c *ServerContext) ClosePort(protocol string, port int) error {
	c.ports.Remove(fmt.Sprintf("%d/%s", port, protocol))
	return nil
}

func (c *ServerContext) OpenPorts(protocol string, from, to int) error {
	panic("OpenPorts unimplemented")
}

func (c *ServerContext) AddMetric(string, string, time.Time) error {
	return nil
}

func (c *ServerContext) ConfigSettings() (charm.Settings, error) {
	return map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}, nil
}

func (c *ServerContext) HookRelation() (jujuc.ContextRelation, bool) {
	return c.Relation(c.relid)
}

func (c *ServerContext) RemoteUnitName() (string, bool) {
	return c.remote, c.remote != ""
}

func (c *ServerContext) Relation(id int) (jujuc.ContextRelation, bool) {
	r, found := c.rels[id]
	return r, found
}

func (c *ServerContext) RelationIds() []int {
	ids := []int{}
	for id := range c.rels {
		ids = append(ids, id)
	}
	return ids
}

type ServerContextRelation struct {
	id    int
	name  string
	units map[string]Settings
}

func (r *ServerContextRelation) Id() int {
	return r.id
}

func (r *ServerContextRelation) Name() string {
	return r.name
}

func (r *ServerContextRelation) FakeId() string {
	return fmt.Sprintf("%s:%d", r.name, r.id)
}

func (r *ServerContextRelation) Settings() (jujuc.Settings, error) {
	return r.units["u/0"], nil
}

func (r *ServerContextRelation) UnitNames() []string {
	s := []string{}
	for name := range r.units {
		s = append(s, name)
	}
	sort.Strings(s)
	return s
}

func (r *ServerContextRelation) ReadSettings(name string) (params.RelationSettings, error) {
	s, found := r.units[name]
	if !found {
		return nil, fmt.Errorf("unknown unit %s", name)
	}
	return s.Map(), nil
}

type Settings map[string]string

func (s Settings) Get(k string) (interface{}, bool) {
	v, f := s[k]
	return v, f
}

func (s Settings) Set(k string, v string) {
	s[k] = v
}

func (s Settings) Delete(k string) {
	delete(s, k)
}

func (s Settings) Map() params.RelationSettings {
	r := map[string]string{}
	for k, v := range s {
		r[k] = v
	}
	return r
}
