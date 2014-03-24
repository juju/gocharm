package identity

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/testing/httpsuite"
	"launchpad.net/goose/testservices/identityservice"
)

type UserPassTestSuite struct {
	httpsuite.HTTPSuite
}

var _ = Suite(&UserPassTestSuite{})

func (s *UserPassTestSuite) TestAuthAgainstServer(c *C) {
	service := identityservice.NewUserPass()
	service.SetupHTTP(s.Mux)
	userInfo := service.AddUser("joe-user", "secrets", "tenant")
	var l Authenticator = &UserPass{}
	creds := Credentials{User: "joe-user", URL: s.Server.URL + "/tokens", Secrets: "secrets"}
	auth, err := l.Auth(&creds)
	c.Assert(err, IsNil)
	c.Assert(auth.Token, Equals, userInfo.Token)
	c.Assert(auth.TenantId, Equals, userInfo.TenantId)
}

// Test that the region -> service endpoint map is correctly populated.
func (s *UserPassTestSuite) TestRegionMatch(c *C) {
	service := identityservice.NewUserPass()
	service.SetupHTTP(s.Mux)
	userInfo := service.AddUser("joe-user", "secrets", "tenant")
	serviceDef := identityservice.Service{"swift", "object-store", []identityservice.Endpoint{
		identityservice.Endpoint{PublicURL: "http://swift", Region: "RegionOne"},
	}}
	service.AddService(serviceDef)
	serviceDef = identityservice.Service{"nova", "compute", []identityservice.Endpoint{
		identityservice.Endpoint{PublicURL: "http://nova", Region: "zone1.RegionOne"},
	}}
	service.AddService(serviceDef)
	serviceDef = identityservice.Service{"nova", "compute", []identityservice.Endpoint{
		identityservice.Endpoint{PublicURL: "http://nova2", Region: "zone2.RegionOne"},
	}}
	service.AddService(serviceDef)

	creds := Credentials{
		User:    "joe-user",
		URL:     s.Server.URL + "/tokens",
		Secrets: "secrets",
		Region:  "zone1.RegionOne",
	}
	var l Authenticator = &UserPass{}
	auth, err := l.Auth(&creds)
	c.Assert(err, IsNil)
	c.Assert(auth.RegionServiceURLs["RegionOne"]["object-store"], Equals, "http://swift")
	c.Assert(auth.RegionServiceURLs["zone1.RegionOne"]["compute"], Equals, "http://nova")
	c.Assert(auth.RegionServiceURLs["zone2.RegionOne"]["compute"], Equals, "http://nova2")
	c.Assert(auth.Token, Equals, userInfo.Token)
	c.Assert(auth.TenantId, Equals, userInfo.TenantId)
}
