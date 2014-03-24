package nova_test

import (
	"encoding/json"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/nova"
)

type JsonSuite struct {
}

var _ = Suite(&JsonSuite{})

func (s *JsonSuite) SetUpSuite(c *C) {
	nova.UseNumericIds(true)
}

func (s *JsonSuite) assertMarshallRoundtrip(c *C, value interface{}, unmarshalled interface{}) {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(data, &unmarshalled)
	if err != nil {
		panic(err)
	}
	c.Assert(unmarshalled, DeepEquals, value)
}

// The following tests all check that unmarshalling of Ids with values > 1E6
// works properly.

func (s *JsonSuite) TestMarshallEntityLargeIntId(c *C) {
	entity := nova.Entity{Id: "2000000", Name: "test"}
	var unmarshalled nova.Entity
	s.assertMarshallRoundtrip(c, &entity, &unmarshalled)
}

func (s *JsonSuite) TestMarshallFlavorDetailLargeIntId(c *C) {
	fd := nova.FlavorDetail{Id: "2000000", Name: "test"}
	var unmarshalled nova.FlavorDetail
	s.assertMarshallRoundtrip(c, &fd, &unmarshalled)
}

func (s *JsonSuite) TestMarshallServerDetailLargeIntId(c *C) {
	fd := nova.Entity{Id: "2000000", Name: "test"}
	im := nova.Entity{Id: "2000000", Name: "test"}
	sd := nova.ServerDetail{Id: "2000000", Name: "test", Flavor: fd, Image: im}
	var unmarshalled nova.ServerDetail
	s.assertMarshallRoundtrip(c, &sd, &unmarshalled)
}

func (s *JsonSuite) TestMarshallFloatingIPLargeIntId(c *C) {
	id := "3000000"
	fip := nova.FloatingIP{Id: "2000000", InstanceId: &id}
	var unmarshalled nova.FloatingIP
	s.assertMarshallRoundtrip(c, &fip, &unmarshalled)
}
