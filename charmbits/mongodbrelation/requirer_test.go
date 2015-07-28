package mongodbrelation_test

import (
	"fmt"
	"io/ioutil"
	"net/http"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/gocharm/charmbits/httpservice"
	_ "github.com/juju/gocharm/charmbits/mongodbrelation"
	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
	"github.com/juju/gocharm/hook/hooktest"
)

type suite struct {
	jujutesting.MgoSuite
}

var _ = gc.Suite(&suite{})

func (*suite) TestHTTPService(c *gc.C) {
	// Now create a test runner to actually test the logic.
	runner := &hooktest.Runner{
		HookStateDir: c.MkDir(),
		RegisterHooks: func(r *hook.Registry) {
			var svc httpservice.Service
			type relations struct {
				Session *mgo.Session `httpservice:"mymongo"`
			}
			svc.Register(r.Clone("svc"), "httpservicename", "http", func(_ struct{}, rel *relations) (httpservice.Handler, error) {
				c.Logf("starting test handler")
				return &testHandler{
					session: rel.Session,
				}, nil
			})
			r.RegisterHook("start", func() error {
				return svc.Start(struct{}{})
			})
			r.RegisterHook("stop", func() error {
				return svc.Stop()
			})
		},
		Logger: c,
	}
	service.NewService = hooktest.NewServiceFunc(runner, nil)

	// Put a record into the database.
	session := jujutesting.MgoServer.MustDial()
	defer session.Close()
	const dbVal = "something stored in the database"
	err := testCollection(session).Insert(&mdoc{
		Val: dbVal,
	})
	c.Assert(err, gc.IsNil)

	err = runner.RunHook("install", "", "")
	c.Assert(err, gc.IsNil)

	err = runner.RunHook("start", "", "")
	c.Assert(err, gc.IsNil)

	httpPort := jujutesting.FindTCPPort()
	runner.Config = map[string]interface{}{
		"http-port": httpPort,
	}
	err = runner.RunHook("config-changed", "", "")
	c.Assert(err, gc.IsNil)

	runner.Relations = map[hook.RelationId]map[hook.UnitId]map[string]string{
		"rel0": {
			"somemongodbservice/0": {
				"hostname": "localhost",
				"port":     fmt.Sprint(jujutesting.MgoServer.Port()),
			},
		},
	}
	runner.RelationIds = map[string][]hook.RelationId{
		"mymongo": {"rel0"},
	}
	err = runner.RunHook("mymongo-relation-joined", "rel0", "fooservice/0")
	c.Assert(err, gc.IsNil)

	// Check that the HTTP server is up and running and has access to
	// the database.
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/", httpPort))
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, dbVal)

	err = runner.RunHook("stop", "", "")
	c.Assert(err, gc.IsNil)
}

func testCollection(s *mgo.Session) *mgo.Collection {
	return s.DB("db").C("c")
}

type mdoc struct {
	Val string
}

type testHandler struct {
	session *mgo.Session
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var doc mdoc
	err := testCollection(h.session).Find(nil).One(&doc)
	if err != nil {
		panic(err)
	}
	w.Write([]byte(doc.Val))
}

func (h *testHandler) Close() error {
	h.session.Close()
	return nil
}
