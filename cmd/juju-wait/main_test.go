package main

import (
	"errors"
	"regexp"
	stdtesting "testing"

	"github.com/juju/juju/apiserver/params"
	. "launchpad.net/gocheck"
)

type suite struct{}

var _ = Suite(suite{})

func Test(t *stdtesting.T) {
	TestingT(t)
}

func (suite) TestWait(c *C) {
	for i, test := range waiterTests {
		c.Logf("test %d. %s", i, test.about)
		pat := regexp.MustCompile("^(" + test.pattern + ")$")
		unit, err := wait(test.unit, pat, &testWatcher{test.changes})
		if test.err != "" {
			c.Check(err, ErrorMatches, test.err)
			c.Assert(unit, IsNil)
		} else {
			c.Assert(err, IsNil)
			c.Assert(status(unit), Equals, test.final)
		}
	}
}

var waiterTests = []struct {
	about   string
	unit    string
	pattern string
	changes [][]params.Delta
	err     string
	final   string
}{{
	about:   "not found in first delta",
	unit:    "wordpress/0",
	pattern: "started",
	changes: [][]params.Delta{
		{{
			Entity: &params.MachineInfo{Id: "0"},
		}, {
			Entity: &params.UnitInfo{Name: "logging/1"},
		}},
	},
	err: `unit "wordpress/0" does not exist`,
}, {
	about:   "next gives error",
	unit:    "wordpress/0",
	pattern: "started",
	err:     "cannot get next deltas: " + errNoMore.Error(),
}, {
	about:   "status comes right in the end",
	unit:    "wordpress/0",
	pattern: "started",
	changes: [][]params.Delta{
		{{
			Entity: &params.UnitInfo{Name: "wordpress/0"},
		}, {
			Entity: &params.UnitInfo{Name: "wordpress/1"},
		}}, {{
			Entity: &params.UnitInfo{
				Name:   "wordpress/0",
				Status: "installed",
			},
		}, {
			Entity: &params.UnitInfo{
				Name:   "wordpress/1",
				Status: "started",
			},
		}}, {{
			Entity: &params.UnitInfo{
				Name:   "wordpress/0",
				Status: "started",
			},
		}},
	},
	final: "started",
}, {
	about:   "error doesn't match error with info",
	unit:    "wordpress/0",
	pattern: "error",
	changes: [][]params.Delta{{{
		Entity: &params.UnitInfo{
			Name:       "wordpress/0",
			Status:     "error",
			StatusInfo: "some info",
		},
	}}},
	err: "cannot get next deltas: " + errNoMore.Error(),
}, {
	about:   "error can match info",
	unit:    "wordpress/0",
	pattern: "error: some info",
	changes: [][]params.Delta{{{
		Entity: &params.UnitInfo{
			Name:       "wordpress/0",
			Status:     "error",
			StatusInfo: "some info",
		},
	}}},
	final: "error: some info",
}, {
	about:   "removed status matches 'removed'",
	unit:    "wordpress/0",
	pattern: "removed",
	changes: [][]params.Delta{
		{{
			Entity: &params.UnitInfo{Name: "wordpress/0"},
		}}, {{
			Removed: true,
			Entity: &params.UnitInfo{
				Name:   "wordpress/0",
				Status: "installed",
			},
		}},
	},
	final: "removed",
}, {
	about:   "removed unit gives error if not matching",
	unit:    "wordpress/0",
	pattern: "installed",
	changes: [][]params.Delta{
		{{
			Entity: &params.UnitInfo{Name: "wordpress/0"},
		}}, {{
			Removed: true,
			Entity: &params.UnitInfo{
				Name:   "wordpress/0",
				Status: "installed",
			},
		}},
	},
	err: "unit was removed",
}, {
	about:   "stable state example part 1",
	unit:    "wordpress/0",
	pattern: "error:.*|started",
	changes: [][]params.Delta{{{
		Entity: &params.UnitInfo{
			Name:       "wordpress/0",
			Status:     "error",
			StatusInfo: "some info",
		},
	}}},
	final: "error: some info",
}, {
	about:   "stable state example part 2",
	unit:    "wordpress/0",
	pattern: "error:.*|started",
	changes: [][]params.Delta{{{
		Entity: &params.UnitInfo{
			Name:   "wordpress/0",
			Status: "started",
		},
	}}},
	final: "started",
}, {
	about:   "error or started",
	unit:    "wordpress/0",
	pattern: "started|error",
	changes: [][]params.Delta{{{
		Entity: &params.UnitInfo{
			Name:       "wordpress/0",
			Status:     "pending",
			StatusInfo: "no error",
		},
	}}},
	err: "cannot get next deltas: " + errNoMore.Error(),
}}

type testWatcher struct {
	changes [][]params.Delta
}

var errNoMore = errors.New("end of deltas")

func (w *testWatcher) Next() ([]params.Delta, error) {
	if len(w.changes) == 0 {
		return nil, errNoMore
	}
	deltas := w.changes[0]
	w.changes = w.changes[1:]
	return deltas, nil
}
