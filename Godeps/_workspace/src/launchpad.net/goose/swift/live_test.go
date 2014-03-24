package swift_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/errors"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/swift"
	"net/http"
)

func registerOpenStackTests(cred *identity.Credentials) {
	Suite(&LiveTests{
		cred: cred,
	})
	Suite(&LiveTestsPublicContainer{
		cred: cred,
	})
}

func randomName() string {
	buf := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

type LiveTests struct {
	cred          *identity.Credentials
	client        client.AuthenticatingClient
	swift         *swift.Client
	containerName string
}

func (s *LiveTests) SetUpSuite(c *C) {
	s.containerName = "test_container" + randomName()
	s.client = client.NewClient(s.cred, identity.AuthUserPass, nil)
	s.swift = swift.New(s.client)
}

func (s *LiveTests) TearDownSuite(c *C) {
	// noop, called by local test suite.
}

func (s *LiveTests) SetUpTest(c *C) {
	assertCreateContainer(c, s.containerName, s.swift, swift.Private)
}

func (s *LiveTests) TearDownTest(c *C) {
	err := s.swift.DeleteContainer(s.containerName)
	c.Check(err, IsNil)
}

func assertCreateContainer(c *C, container string, s *swift.Client, acl swift.ACL) {
	// The test container may exist already, so try and delete it.
	// If the result is a NotFound error, we don't care.
	err := s.DeleteContainer(container)
	if err != nil {
		c.Check(errors.IsNotFound(err), Equals, true)
	}
	err = s.CreateContainer(container, acl)
	c.Assert(err, IsNil)
}

func (s *LiveTests) TestObject(c *C) {
	object := "test_obj1"
	data := "...some data..."
	err := s.swift.PutObject(s.containerName, object, []byte(data))
	c.Check(err, IsNil)
	objdata, err := s.swift.GetObject(s.containerName, object)
	c.Check(err, IsNil)
	c.Check(string(objdata), Equals, data)
	err = s.swift.DeleteObject(s.containerName, object)
	c.Assert(err, IsNil)
}

func (s *LiveTests) TestObjectReader(c *C) {
	object := "test_obj2"
	data := "...some data..."
	err := s.swift.PutReader(s.containerName, object, bytes.NewReader([]byte(data)), int64(len(data)))
	c.Check(err, IsNil)
	r, err := s.swift.GetReader(s.containerName, object)
	c.Check(err, IsNil)
	readData, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	r.Close()
	c.Check(string(readData), Equals, data)
	err = s.swift.DeleteObject(s.containerName, object)
	c.Assert(err, IsNil)
}

func (s *LiveTests) TestList(c *C) {
	data := "...some data..."
	var files []string = make([]string, 2)
	var fileNames map[string]bool = make(map[string]bool)
	for i := 0; i < 2; i++ {
		files[i] = fmt.Sprintf("test_obj%d", i)
		fileNames[files[i]] = true
		err := s.swift.PutObject(s.containerName, files[i], []byte(data))
		c.Check(err, IsNil)
	}
	items, err := s.swift.List(s.containerName, "", "", "", 0)
	c.Check(err, IsNil)
	c.Check(len(items), Equals, len(files))
	for _, item := range items {
		c.Check(fileNames[item.Name], Equals, true)
	}
	for i := 0; i < len(files); i++ {
		s.swift.DeleteObject(s.containerName, files[i])
	}
}

func (s *LiveTests) TestURL(c *C) {
	object := "test_obj1"
	data := "...some data..."
	err := s.swift.PutObject(s.containerName, object, []byte(data))
	c.Check(err, IsNil)
	url, err := s.swift.URL(s.containerName, object)
	c.Check(err, IsNil)
	httpClient := http.DefaultClient
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Add("X-Auth-Token", s.client.Token())
	c.Check(err, IsNil)
	resp, err := httpClient.Do(req)
	defer resp.Body.Close()
	c.Check(err, IsNil)
	c.Check(resp.StatusCode, Equals, http.StatusOK)
	objdata, err := ioutil.ReadAll(resp.Body)
	c.Check(err, IsNil)
	c.Check(string(objdata), Equals, data)
	err = s.swift.DeleteObject(s.containerName, object)
	c.Assert(err, IsNil)
}

type LiveTestsPublicContainer struct {
	cred          *identity.Credentials
	client        client.AuthenticatingClient
	publicClient  client.Client
	swift         *swift.Client
	publicSwift   *swift.Client
	containerName string
}

func (s *LiveTestsPublicContainer) SetUpSuite(c *C) {
	s.containerName = "test_container" + randomName()
	s.client = client.NewClient(s.cred, identity.AuthUserPass, nil)
	s.swift = swift.New(s.client)
}

func (s *LiveTestsPublicContainer) TearDownSuite(c *C) {
	// noop, called by local test suite.
}

func (s *LiveTestsPublicContainer) SetUpTest(c *C) {
	err := s.client.Authenticate()
	c.Assert(err, IsNil)
	baseURL, err := s.client.MakeServiceURL("object-store", nil)
	c.Assert(err, IsNil)
	s.publicClient = client.NewPublicClient(baseURL, nil)
	s.publicSwift = swift.New(s.publicClient)
	assertCreateContainer(c, s.containerName, s.swift, swift.PublicRead)
}

func (s *LiveTestsPublicContainer) TearDownTest(c *C) {
	err := s.swift.DeleteContainer(s.containerName)
	c.Check(err, IsNil)
}

func (s *LiveTestsPublicContainer) TestPublicObjectReader(c *C) {
	object := "test_obj2"
	data := "...some data..."
	err := s.swift.PutReader(s.containerName, object, bytes.NewReader([]byte(data)), int64(len(data)))
	c.Check(err, IsNil)
	r, err := s.publicSwift.GetReader(s.containerName, object)
	c.Check(err, IsNil)
	readData, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	r.Close()
	c.Check(string(readData), Equals, data)
	err = s.swift.DeleteObject(s.containerName, object)
	c.Assert(err, IsNil)
}

func (s *LiveTestsPublicContainer) TestPublicList(c *C) {
	data := "...some data..."
	var files []string = make([]string, 2)
	var fileNames map[string]bool = make(map[string]bool)
	for i := 0; i < 2; i++ {
		files[i] = fmt.Sprintf("test_obj%d", i)
		fileNames[files[i]] = true
		err := s.swift.PutObject(s.containerName, files[i], []byte(data))
		c.Check(err, IsNil)
	}
	items, err := s.publicSwift.List(s.containerName, "", "", "", 0)
	c.Check(err, IsNil)
	c.Check(len(items), Equals, len(files))
	for _, item := range items {
		c.Check(fileNames[item.Name], Equals, true)
	}
	for i := 0; i < len(files); i++ {
		s.swift.DeleteObject(s.containerName, files[i])
	}
}

func (s *LiveTestsPublicContainer) TestPublicURL(c *C) {
	object := "test_obj1"
	data := "...some data..."
	err := s.swift.PutObject(s.containerName, object, []byte(data))
	c.Check(err, IsNil)
	url, err := s.swift.URL(s.containerName, object)
	c.Check(err, IsNil)
	httpClient := http.DefaultClient
	req, err := http.NewRequest("GET", url, nil)
	c.Check(err, IsNil)
	resp, err := httpClient.Do(req)
	defer resp.Body.Close()
	c.Check(err, IsNil)
	c.Check(resp.StatusCode, Equals, http.StatusOK)
	objdata, err := ioutil.ReadAll(resp.Body)
	c.Check(err, IsNil)
	c.Check(string(objdata), Equals, data)
	err = s.swift.DeleteObject(s.containerName, object)
	c.Assert(err, IsNil)
}
