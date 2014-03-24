package testservices

import (
	"errors"
	"launchpad.net/goose/testservices/hook"
	"launchpad.net/goose/testservices/identityservice"
	"net/http"
)

// An HttpService provides the HTTP API for a service double.
type HttpService interface {
	SetupHTTP(mux *http.ServeMux)
}

// A ServiceInstance is an Openstack module, one of nova, swift, glance.
type ServiceInstance struct {
	identityservice.ServiceProvider
	hook.TestService
	IdentityService identityservice.IdentityService
	Scheme          string
	Hostname        string
	VersionPath     string
	TenantId        string
	Region          string
}

// Internal Openstack errors.

var RateLimitExceededError = errors.New("retry limit exceeded")

// NoMoreFloatingIPs corresponds to "HTTP 404 Zero floating ips available."
var NoMoreFloatingIPs = errors.New("zero floating ips available")

// IPLimitExceeded corresponds to "HTTP 413 Maximum number of floating ips exceeded"
var IPLimitExceeded = errors.New("maximum number of floating ips exceeded")
