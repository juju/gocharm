package nova_test

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/errors"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"log"
	"strings"
	"time"
)

const (
	// A made up name we use for the test server instance.
	testImageName = "nova_test_server"
)

func registerOpenStackTests(cred *identity.Credentials, testImageDetails imageDetails) {
	Suite(&LiveTests{
		cred:        cred,
		testImageId: testImageDetails.imageId,
		testFlavor:  testImageDetails.flavor,
		vendor:      testImageDetails.vendor,
	})
}

type LiveTests struct {
	cred          *identity.Credentials
	client        client.AuthenticatingClient
	nova          *nova.Client
	testServer    *nova.Entity
	userId        string
	tenantId      string
	testImageId   string
	testFlavor    string
	testFlavorId  string
	vendor        string
	useNumericIds bool
}

func (s *LiveTests) SetUpSuite(c *C) {
	s.client = client.NewClient(s.cred, identity.AuthUserPass, nil)
	s.nova = nova.New(s.client)
	var err error
	s.testFlavorId, err = s.findFlavorId(s.testFlavor)
	c.Assert(err, IsNil)
	s.testServer, err = s.createInstance(c, testImageName)
	c.Assert(err, IsNil)
	s.waitTestServerToStart(c)
	// These will not be filled in until a client has authorised which will happen creating the instance above.
	s.userId = s.client.UserId()
	s.tenantId = s.client.TenantId()
}

func (s *LiveTests) findFlavorId(flavorName string) (string, error) {
	flavors, err := s.nova.ListFlavors()
	if err != nil {
		return "", err
	}
	var flavorId string
	for _, flavor := range flavors {
		if flavor.Name == flavorName {
			flavorId = flavor.Id
			break
		}
	}
	if flavorId == "" {
		return "", fmt.Errorf("No such flavor %s", flavorName)
	}
	return flavorId, nil
}

func (s *LiveTests) TearDownSuite(c *C) {
	if s.testServer != nil {
		err := s.nova.DeleteServer(s.testServer.Id)
		c.Check(err, IsNil)
	}
}

func (s *LiveTests) SetUpTest(c *C) {
	// noop, called by local test suite.
}

func (s *LiveTests) TearDownTest(c *C) {
	// noop, called by local test suite.
}

func (s *LiveTests) createInstance(c *C, name string) (instance *nova.Entity, err error) {
	opts := nova.RunServerOpts{
		Name:     name,
		FlavorId: s.testFlavorId,
		ImageId:  s.testImageId,
		UserData: nil,
	}
	instance, err = s.nova.RunServer(opts)
	if err != nil {
		return nil, err
	}
	return instance, nil
}

// Assert that the server record matches the details of the test server image.
func (s *LiveTests) assertServerDetails(c *C, sr *nova.ServerDetail) {
	c.Check(sr.Id, Equals, s.testServer.Id)
	c.Check(sr.Name, Equals, testImageName)
	c.Check(sr.Flavor.Id, Equals, s.testFlavorId)
	c.Check(sr.Image.Id, Equals, s.testImageId)
}

func (s *LiveTests) TestListFlavors(c *C) {
	flavors, err := s.nova.ListFlavors()
	c.Assert(err, IsNil)
	if len(flavors) < 1 {
		c.Fatalf("no flavors to list")
	}
	for _, f := range flavors {
		c.Check(f.Id, Not(Equals), "")
		c.Check(f.Name, Not(Equals), "")
		for _, l := range f.Links {
			c.Check(l.Href, Matches, "https?://.*")
			c.Check(l.Rel, Matches, "self|bookmark")
		}
	}
}

func (s *LiveTests) TestListFlavorsDetail(c *C) {
	flavors, err := s.nova.ListFlavorsDetail()
	c.Assert(err, IsNil)
	if len(flavors) < 1 {
		c.Fatalf("no flavors (details) to list")
	}
	for _, f := range flavors {
		c.Check(f.Name, Not(Equals), "")
		c.Check(f.Id, Not(Equals), "")
		if f.RAM < 0 || f.VCPUs < 0 || f.Disk < 0 {
			c.Fatalf("invalid flavor found: %#v", f)
		}
	}
}

func (s *LiveTests) TestListServers(c *C) {
	servers, err := s.nova.ListServers(nil)
	c.Assert(err, IsNil)
	foundTest := false
	for _, sr := range servers {
		c.Check(sr.Id, Not(Equals), "")
		c.Check(sr.Name, Not(Equals), "")
		if sr.Id == s.testServer.Id {
			c.Check(sr.Name, Equals, testImageName)
			foundTest = true
		}
		for _, l := range sr.Links {
			c.Check(l.Href, Matches, "https?://.*")
			c.Check(l.Rel, Matches, "self|bookmark")
		}
	}
	if !foundTest {
		c.Fatalf("test server (%s) not found in server list", s.testServer.Id)
	}
}

func (s *LiveTests) TestListServersWithFilter(c *C) {
	inst, err := s.createInstance(c, "filtered_server")
	c.Assert(err, IsNil)
	defer s.nova.DeleteServer(inst.Id)
	filter := nova.NewFilter()
	filter.Set(nova.FilterServer, "filtered_server")
	servers, err := s.nova.ListServers(filter)
	c.Assert(err, IsNil)
	found := false
	for _, sr := range servers {
		if sr.Id == inst.Id {
			c.Assert(sr.Name, Equals, "filtered_server")
			found = true
		}
	}
	if !found {
		c.Fatalf("server (%s) not found in filtered server list %v", inst.Id, servers)
	}
}

func (s *LiveTests) TestListServersDetail(c *C) {
	servers, err := s.nova.ListServersDetail(nil)
	c.Assert(err, IsNil)
	if len(servers) < 1 {
		c.Fatalf("no servers to list (expected at least 1)")
	}
	foundTest := false
	for _, sr := range servers {
		// not checking for Addresses, because it could be missing
		c.Check(sr.Created, Matches, `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*`)
		c.Check(sr.Updated, Matches, `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*`)
		c.Check(sr.Id, Not(Equals), "")
		c.Check(sr.HostId, Not(Equals), "")
		c.Check(sr.TenantId, Equals, s.tenantId)
		c.Check(sr.UserId, Equals, s.userId)
		c.Check(sr.Status, Not(Equals), "")
		c.Check(sr.Name, Not(Equals), "")
		if sr.Id == s.testServer.Id {
			s.assertServerDetails(c, &sr)
			foundTest = true
		}
		for _, l := range sr.Links {
			c.Check(l.Href, Matches, "https?://.*")
			c.Check(l.Rel, Matches, "self|bookmark")
		}
		c.Check(sr.Flavor.Id, Not(Equals), "")
		for _, f := range sr.Flavor.Links {
			c.Check(f.Href, Matches, "https?://.*")
			c.Check(f.Rel, Matches, "self|bookmark")
		}
		c.Check(sr.Image.Id, Not(Equals), "")
		for _, i := range sr.Image.Links {
			c.Check(i.Href, Matches, "https?://.*")
			c.Check(i.Rel, Matches, "self|bookmark")
		}
	}
	if !foundTest {
		c.Fatalf("test server (%s) not found in server list (details)", s.testServer.Id)
	}
}

func (s *LiveTests) TestListServersDetailWithFilter(c *C) {
	inst, err := s.createInstance(c, "filtered_server")
	c.Assert(err, IsNil)
	defer s.nova.DeleteServer(inst.Id)
	filter := nova.NewFilter()
	filter.Set(nova.FilterServer, "filtered_server")
	servers, err := s.nova.ListServersDetail(filter)
	c.Assert(err, IsNil)
	found := false
	for _, sr := range servers {
		if sr.Id == inst.Id {
			c.Assert(sr.Name, Equals, "filtered_server")
			found = true
		}
	}
	if !found {
		c.Fatalf("server (%s) not found in filtered server details %v", inst.Id, servers)
	}
}

func (s *LiveTests) TestListSecurityGroups(c *C) {
	groups, err := s.nova.ListSecurityGroups()
	c.Assert(err, IsNil)
	if len(groups) < 1 {
		c.Fatalf("no security groups found (expected at least 1)")
	}
	for _, g := range groups {
		c.Check(g.TenantId, Equals, s.tenantId)
		c.Check(g.Name, Not(Equals), "")
		c.Check(g.Description, Not(Equals), "")
		c.Check(g.Rules, NotNil)
	}
}

func (s *LiveTests) TestCreateAndDeleteSecurityGroup(c *C) {
	group, err := s.nova.CreateSecurityGroup("test_secgroup", "test_desc")
	c.Assert(err, IsNil)
	c.Check(group.Name, Equals, "test_secgroup")
	c.Check(group.Description, Equals, "test_desc")

	groups, err := s.nova.ListSecurityGroups()
	found := false
	for _, g := range groups {
		if g.Id == group.Id {
			found = true
			break
		}
	}
	if found {
		err = s.nova.DeleteSecurityGroup(group.Id)
		c.Check(err, IsNil)
	} else {
		c.Fatalf("test security group (%d) not found", group.Id)
	}
}

func (s *LiveTests) TestDuplicateSecurityGroupError(c *C) {
	group, err := s.nova.CreateSecurityGroup("test_dupgroup", "test_desc")
	c.Assert(err, IsNil)
	defer s.nova.DeleteSecurityGroup(group.Id)
	group, err = s.nova.CreateSecurityGroup("test_dupgroup", "test_desc")
	c.Assert(errors.IsDuplicateValue(err), Equals, true)
}

func (s *LiveTests) TestCreateAndDeleteSecurityGroupRules(c *C) {
	group1, err := s.nova.CreateSecurityGroup("test_secgroup1", "test_desc")
	c.Assert(err, IsNil)
	group2, err := s.nova.CreateSecurityGroup("test_secgroup2", "test_desc")
	c.Assert(err, IsNil)

	// First type of rule - port range + protocol
	ri := nova.RuleInfo{
		IPProtocol:    "tcp",
		FromPort:      1234,
		ToPort:        4321,
		Cidr:          "10.0.0.0/8",
		ParentGroupId: group1.Id,
	}
	rule, err := s.nova.CreateSecurityGroupRule(ri)
	c.Assert(err, IsNil)
	c.Check(*rule.FromPort, Equals, 1234)
	c.Check(*rule.ToPort, Equals, 4321)
	c.Check(rule.ParentGroupId, Equals, group1.Id)
	c.Check(*rule.IPProtocol, Equals, "tcp")
	c.Check(rule.Group, Equals, nova.SecurityGroupRef{})
	err = s.nova.DeleteSecurityGroupRule(rule.Id)
	c.Check(err, IsNil)

	// Second type of rule - inherited from another group
	ri = nova.RuleInfo{
		GroupId:       &group2.Id,
		ParentGroupId: group1.Id,
	}
	rule, err = s.nova.CreateSecurityGroupRule(ri)
	c.Assert(err, IsNil)
	c.Check(rule.ParentGroupId, Equals, group1.Id)
	c.Check(rule.Group, NotNil)
	c.Check(rule.Group.TenantId, Equals, s.tenantId)
	c.Check(rule.Group.Name, Equals, "test_secgroup2")
	err = s.nova.DeleteSecurityGroupRule(rule.Id)
	c.Check(err, IsNil)

	err = s.nova.DeleteSecurityGroup(group1.Id)
	c.Check(err, IsNil)
	err = s.nova.DeleteSecurityGroup(group2.Id)
	c.Check(err, IsNil)
}

func (s *LiveTests) TestGetServer(c *C) {
	server, err := s.nova.GetServer(s.testServer.Id)
	c.Assert(err, IsNil)
	s.assertServerDetails(c, server)
}

func (s *LiveTests) waitTestServerToStart(c *C) {
	// Wait until the test server is actually running
	c.Logf("waiting the test server %s to start...", s.testServer.Id)
	for {
		server, err := s.nova.GetServer(s.testServer.Id)
		c.Assert(err, IsNil)
		if server.Status == nova.StatusActive {
			break
		}
		// We dont' want to flood the connection while polling the server waiting for it to start.
		c.Logf("server has status %s, waiting 10 seconds before polling again...", server.Status)
		time.Sleep(10 * time.Second)
	}
	c.Logf("started")
}

func (s *LiveTests) TestServerAddGetRemoveSecurityGroup(c *C) {
	group, err := s.nova.CreateSecurityGroup("test_server_secgroup", "test desc")
	if err != nil {
		c.Assert(errors.IsDuplicateValue(err), Equals, true)
		group, err = s.nova.SecurityGroupByName("test_server_secgroup")
		c.Assert(err, IsNil)
	}

	s.waitTestServerToStart(c)
	err = s.nova.AddServerSecurityGroup(s.testServer.Id, group.Name)
	c.Assert(err, IsNil)
	groups, err := s.nova.GetServerSecurityGroups(s.testServer.Id)
	c.Assert(err, IsNil)
	found := false
	for _, g := range groups {
		if g.Id == group.Id || g.Name == group.Name {
			found = true
			break
		}
	}
	err = s.nova.RemoveServerSecurityGroup(s.testServer.Id, group.Name)
	c.Check(err, IsNil)

	err = s.nova.DeleteSecurityGroup(group.Id)
	c.Assert(err, IsNil)

	if !found {
		c.Fail()
	}
}

func (s *LiveTests) TestFloatingIPs(c *C) {
	ip, err := s.nova.AllocateFloatingIP()
	c.Assert(err, IsNil)
	defer s.nova.DeleteFloatingIP(ip.Id)
	c.Check(ip.IP, Not(Equals), "")
	c.Check(ip.FixedIP, IsNil)
	c.Check(ip.InstanceId, IsNil)

	ips, err := s.nova.ListFloatingIPs()
	c.Assert(err, IsNil)
	if len(ips) < 1 {
		c.Errorf("no floating IPs found (expected at least 1)")
	} else {
		found := false
		for _, i := range ips {
			c.Check(i.IP, Not(Equals), "")
			if i.Id == ip.Id {
				c.Check(i.IP, Equals, ip.IP)
				c.Check(i.Pool, Equals, ip.Pool)
				found = true
			}
		}
		if !found {
			c.Errorf("expected to find added floating IP: %#v", ip)
		}

		fip, err := s.nova.GetFloatingIP(ip.Id)
		c.Assert(err, IsNil)
		c.Check(fip.Id, Equals, ip.Id)
		c.Check(fip.IP, Equals, ip.IP)
		c.Check(fip.Pool, Equals, ip.Pool)
	}
}

func (s *LiveTests) TestServerFloatingIPs(c *C) {
	ip, err := s.nova.AllocateFloatingIP()
	c.Assert(err, IsNil)
	defer s.nova.DeleteFloatingIP(ip.Id)
	c.Check(ip.IP, Matches, `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)

	s.waitTestServerToStart(c)
	err = s.nova.AddServerFloatingIP(s.testServer.Id, ip.IP)
	c.Assert(err, IsNil)
	// TODO (wallyworld) - 2013-02-11 bug=1121666
	// where we are creating a real server, test that the IP address created above
	// can be used to connect to the server
	defer s.nova.RemoveServerFloatingIP(s.testServer.Id, ip.IP)

	fip, err := s.nova.GetFloatingIP(ip.Id)
	c.Assert(err, IsNil)
	c.Check(*fip.FixedIP, Matches, `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	c.Check(*fip.InstanceId, Equals, s.testServer.Id)

	err = s.nova.RemoveServerFloatingIP(s.testServer.Id, ip.IP)
	c.Check(err, IsNil)
	fip, err = s.nova.GetFloatingIP(ip.Id)
	c.Assert(err, IsNil)
	c.Check(fip.FixedIP, IsNil)
	c.Check(fip.InstanceId, IsNil)
}

// TestRateLimitRetry checks that when we make too many requests and receive a Retry-After response, the retry
// occurs and the request ultimately succeeds.
func (s *LiveTests) TestRateLimitRetry(c *C) {
	if s.vendor != "canonistack" {
		c.Skip("TestRateLimitRetry is only run for Canonistack")
	}
	// Capture the logged output so we can check for retry messages.
	var logout bytes.Buffer
	logger := log.New(&logout, "", log.LstdFlags)
	client := client.NewClient(s.cred, identity.AuthUserPass, logger)
	novaClient := nova.New(client)
	// Delete the artifact if it already exists.
	testGroup, err := novaClient.SecurityGroupByName("test_group")
	if err != nil {
		c.Assert(errors.IsNotFound(err), Equals, true)
	} else {
		novaClient.DeleteSecurityGroup(testGroup.Id)
		c.Assert(err, IsNil)
	}
	// Create some artifacts a number of times in succession and ensure each time is successful,
	// even with retries being required. As soon as we see a retry message, the test has passed
	// and we exit.
	for i := 0; i < 50; i++ {
		testGroup, err = novaClient.CreateSecurityGroup("test_group", "test")
		c.Assert(err, IsNil)
		novaClient.DeleteSecurityGroup(testGroup.Id)
		c.Assert(err, IsNil)
		output := logout.String()
		if strings.Contains(output, "Too many requests, retrying in") == true {
			return
		}
	}
	// No retry message logged so test has failed.
	c.Fail()
}

func (s *LiveTests) TestRegexpInstanceFilters(c *C) {
	serverNames := []string{
		"foobar123",
		"foo123baz",
		"123barbaz",
	}
	for _, name := range serverNames {
		inst, err := s.createInstance(c, name)
		c.Assert(err, IsNil)
		defer s.nova.DeleteServer(inst.Id)
	}
	filter := nova.NewFilter()
	filter.Set(nova.FilterServer, `foo.*baz`)
	servers, err := s.nova.ListServersDetail(filter)
	c.Assert(err, IsNil)
	c.Assert(servers, HasLen, 1)
	c.Assert(servers[0].Name, Equals, serverNames[1])
	filter.Set(nova.FilterServer, `[0-9]+[a-z]+`)
	servers, err = s.nova.ListServersDetail(filter)
	c.Assert(err, IsNil)
	c.Assert(servers, HasLen, 2)
	if servers[0].Name != serverNames[1] {
		servers[0], servers[1] = servers[1], servers[0]
	}
	c.Assert(servers[0].Name, Equals, serverNames[1])
	c.Assert(servers[1].Name, Equals, serverNames[2])
}

func (s *LiveTests) TestListNetworks(c *C) {
	networks, err := s.nova.ListNetworks()
	c.Assert(err, IsNil)
	for _, network := range networks {
		c.Check(network.Id, Not(Equals), "")
		c.Check(network.Label, Not(Equals), "")
		c.Assert(network.Cidr, Matches, `\d{1,3}(\.+\d{1,3}){3}\/\d+`)
	}
}
