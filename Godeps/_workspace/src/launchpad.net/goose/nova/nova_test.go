package nova_test

import (
	"flag"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"reflect"
	"testing"
)

// imageDetails specify parameters used to start a test machine for the live tests.
type imageDetails struct {
	flavor  string
	imageId string
	vendor  string
}

// Out-of-the-box, we support live testing using Canonistack or HP Cloud.
var testConstraints = map[string]imageDetails{
	"canonistack": imageDetails{
		flavor: "m1.tiny", imageId: "f2ca48ce-30d5-4f1f-9075-12e64510368d"},
	"hpcloud": imageDetails{
		flavor: "standard.xsmall", imageId: "81078"},
}

var live = flag.Bool("live", false, "Include live OpenStack tests")
var vendor = flag.String("vendor", "", "The Openstack vendor to test against")
var imageId = flag.String("image", "", "The image id for which a test service is to be started")
var flavor = flag.String("flavor", "", "The flavor of the test service")

func Test(t *testing.T) {
	if *live {
		// We can either specify a vendor, or imageId and flavor separately.
		var testImageDetails imageDetails
		if *vendor != "" {
			var ok bool
			if testImageDetails, ok = testConstraints[*vendor]; !ok {
				keys := reflect.ValueOf(testConstraints).MapKeys()
				t.Fatalf("Unknown vendor %s. Must be one of %s", *vendor, keys)
			}
			testImageDetails.vendor = *vendor
		} else {
			if *imageId == "" {
				t.Fatalf("Must specify image id to use for test instance, "+
					"eg %s for Canonistack", "-image c876e5fe-abb0-41f0-8f29-f0b47481f523")
			}
			if *flavor == "" {
				t.Fatalf("Must specify flavor to use for test instance, "+
					"eg %s for Canonistack", "-flavor m1.tiny")
			}
			testImageDetails = imageDetails{*flavor, *imageId, ""}
		}
		cred, err := identity.CompleteCredentialsFromEnv()
		if err != nil {
			t.Fatalf("Error setting up test suite: %s", err.Error())
		}
		registerOpenStackTests(cred, testImageDetails)
	}
	registerLocalTests()
	TestingT(t)
}
