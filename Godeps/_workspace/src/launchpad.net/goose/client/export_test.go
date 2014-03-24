package client

import (
	"launchpad.net/goose/identity"
	"time"
)

type AuthCleanup func()

func SetAuthenticationTimeout(timeout time.Duration) AuthCleanup {
	origTimeout := authenticationTimeout
	authenticationTimeout = timeout
	return func() {
		authenticationTimeout = origTimeout
	}
}

func SetAuthenticator(client AuthenticatingClient, auth identity.Authenticator) {
	client.(*authenticatingClient).authMode = auth
}
