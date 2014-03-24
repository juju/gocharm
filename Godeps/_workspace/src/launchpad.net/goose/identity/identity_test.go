package identity

import (
	. "launchpad.net/gocheck"
	goosehttp "launchpad.net/goose/http"
	"launchpad.net/goose/testing/envsuite"
	"os"
)

type CredentialsTestSuite struct {
	// Isolate all of these tests from the real Environ.
	envsuite.EnvSuite
}

type NewAuthenticatorSuite struct{}

var _ = Suite(&CredentialsTestSuite{})
var _ = Suite(&NewAuthenticatorSuite{})

func (s *CredentialsTestSuite) TestCredentialsFromEnv(c *C) {
	var scenarios = []struct {
		summary  string
		env      map[string]string
		username string
		password string
		tenant   string
		region   string
		authURL  string
	}{
		{summary: "Old 'NOVA' style creds",
			env: map[string]string{
				"NOVA_USERNAME":   "test-user",
				"NOVA_PASSWORD":   "test-pass",
				"NOVA_API_KEY":    "test-access-key",
				"EC2_SECRET_KEYS": "test-secret-key",
				"NOVA_PROJECT_ID": "tenant-name",
				"NOVA_REGION":     "region",
			},
			username: "test-user",
			password: "test-pass",
			tenant:   "tenant-name",
			region:   "region",
		},
		{summary: "New 'OS' style environment",
			env: map[string]string{
				"OS_USERNAME":    "test-user",
				"OS_PASSWORD":    "test-pass",
				"OS_ACCESS_KEY":  "test-access-key",
				"OS_SECRET_KEY":  "test-secret-key",
				"OS_TENANT_NAME": "tenant-name",
				"OS_REGION_NAME": "region",
			},
			username: "test-user",
			password: "test-pass",
			tenant:   "tenant-name",
			region:   "region",
		},
	}
	for _, scenario := range scenarios {
		for key, value := range scenario.env {
			os.Setenv(key, value)
		}
		creds := CredentialsFromEnv()
		c.Check(creds.URL, Equals, scenario.authURL)
		c.Check(creds.User, Equals, scenario.username)
		c.Check(creds.Secrets, Equals, scenario.password)
		c.Check(creds.Region, Equals, scenario.region)
		c.Check(creds.TenantName, Equals, scenario.tenant)
	}
}

func (s *CredentialsTestSuite) TestCompleteCredentialsFromEnvValid(c *C) {
	env := map[string]string{
		"OS_AUTH_URL":    "http://auth",
		"OS_USERNAME":    "test-user",
		"OS_PASSWORD":    "test-pass",
		"OS_ACCESS_KEY":  "test-access-key",
		"OS_SECRET_KEY":  "test-secret-key",
		"OS_TENANT_NAME": "tenant-name",
		"OS_REGION_NAME": "region",
	}
	for key, value := range env {
		os.Setenv(key, value)
	}
	creds, err := CompleteCredentialsFromEnv()
	c.Assert(err, IsNil)
	c.Check(creds.URL, Equals, "http://auth")
	c.Check(creds.User, Equals, "test-user")
	c.Check(creds.Secrets, Equals, "test-pass")
	c.Check(creds.Region, Equals, "region")
	c.Check(creds.TenantName, Equals, "tenant-name")
}

// An error is returned if not all required environment variables are set.
func (s *CredentialsTestSuite) TestCompleteCredentialsFromEnvInvalid(c *C) {
	env := map[string]string{
		"OS_AUTH_URL":    "http://auth",
		"OS_USERNAME":    "test-user",
		"OS_ACCESS_KEY":  "test-access-key",
		"OS_TENANT_NAME": "tenant-name",
		"OS_REGION_NAME": "region",
	}
	for key, value := range env {
		os.Setenv(key, value)
	}
	_, err := CompleteCredentialsFromEnv()
	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Matches, "required environment variable not set.*: Secrets")
}

func (s *CredentialsTestSuite) TestCompleteCredentialsFromEnvKeypair(c *C) {
	env := map[string]string{
		"OS_AUTH_URL":    "http://auth",
		"OS_USERNAME":    "",
		"OS_PASSWORD":    "",
		"OS_ACCESS_KEY":  "test-access-key",
		"OS_SECRET_KEY":  "test-secret-key",
		"OS_TENANT_NAME": "tenant-name",
		"OS_REGION_NAME": "region",
	}
	for key, value := range env {
		os.Setenv(key, value)
	}
	creds, err := CompleteCredentialsFromEnv()
	c.Assert(err, IsNil)
	c.Check(creds.URL, Equals, "http://auth")
	c.Check(creds.User, Equals, "test-access-key")
	c.Check(creds.Secrets, Equals, "test-secret-key")
	c.Check(creds.Region, Equals, "region")
	c.Check(creds.TenantName, Equals, "tenant-name")
}

func (s *CredentialsTestSuite) TestCompleteCredentialsFromEnvKeypairCompatibleEnvVars(c *C) {
	env := map[string]string{
		"OS_AUTH_URL":     "http://auth",
		"OS_USERNAME":     "",
		"OS_PASSWORD":     "",
		"NOVA_API_KEY":    "test-access-key",
		"EC2_SECRET_KEYS": "test-secret-key",
		"OS_TENANT_NAME":  "tenant-name",
		"OS_REGION_NAME":  "region",
	}
	for key, value := range env {
		os.Setenv(key, value)
	}
	creds, err := CompleteCredentialsFromEnv()
	c.Assert(err, IsNil)
	c.Check(creds.URL, Equals, "http://auth")
	c.Check(creds.User, Equals, "test-access-key")
	c.Check(creds.Secrets, Equals, "test-secret-key")
	c.Check(creds.Region, Equals, "region")
	c.Check(creds.TenantName, Equals, "tenant-name")
}

func (s *NewAuthenticatorSuite) TestUserPassNoHTTPClient(c *C) {
	auth := NewAuthenticator(AuthUserPass, nil)
	userAuth, ok := auth.(*UserPass)
	c.Assert(ok, Equals, true)
	c.Assert(userAuth.client, NotNil)
}

func (s *NewAuthenticatorSuite) TestUserPassCustomHTTPClient(c *C) {
	httpClient := goosehttp.New()
	auth := NewAuthenticator(AuthUserPass, httpClient)
	userAuth, ok := auth.(*UserPass)
	c.Assert(ok, Equals, true)
	c.Assert(userAuth.client, Equals, httpClient)
}

func (s *NewAuthenticatorSuite) TestKeyPairNoHTTPClient(c *C) {
	auth := NewAuthenticator(AuthKeyPair, nil)
	keyPairAuth, ok := auth.(*KeyPair)
	c.Assert(ok, Equals, true)
	c.Assert(keyPairAuth.client, NotNil)
}

func (s *NewAuthenticatorSuite) TestKeyPairCustomHTTPClient(c *C) {
	httpClient := goosehttp.New()
	auth := NewAuthenticator(AuthKeyPair, httpClient)
	keyPairAuth, ok := auth.(*KeyPair)
	c.Assert(ok, Equals, true)
	c.Assert(keyPairAuth.client, Equals, httpClient)
}

func (s *NewAuthenticatorSuite) TestLegacyNoHTTPClient(c *C) {
	auth := NewAuthenticator(AuthLegacy, nil)
	legacyAuth, ok := auth.(*Legacy)
	c.Assert(ok, Equals, true)
	c.Assert(legacyAuth.client, NotNil)
}

func (s *NewAuthenticatorSuite) TestLegacyCustomHTTPClient(c *C) {
	httpClient := goosehttp.New()
	auth := NewAuthenticator(AuthLegacy, httpClient)
	legacyAuth, ok := auth.(*Legacy)
	c.Assert(ok, Equals, true)
	c.Assert(legacyAuth.client, Equals, httpClient)
}

func (s *NewAuthenticatorSuite) TestUnknownMode(c *C) {
	c.Assert(func() { NewAuthenticator(1235, nil) },
		PanicMatches, "Invalid identity authorisation mode: 1235")
}
