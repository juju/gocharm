package novaservice

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/nova"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

// checkGroupsInList checks that every group in groups is in groupList.
func checkGroupsInList(c *C, groups []nova.SecurityGroup, groupList []nova.SecurityGroup) {
	for _, g := range groups {
		for _, gr := range groupList {
			if g.Id == gr.Id {
				c.Assert(g, DeepEquals, gr)
				return
			}
		}
		c.Fail()
	}
}
