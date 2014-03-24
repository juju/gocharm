package identityservice

import "net/http"

// An IdentityService provides user authentication for an Openstack instance.
type IdentityService interface {
	AddUser(user, secret, tenant string) *UserInfo
	FindUser(token string) (*UserInfo, error)
	RegisterServiceProvider(name, serviceType string, serviceProvider ServiceProvider)
	AddService(service Service)
	SetupHTTP(mux *http.ServeMux)
}

// A ServiceProvider is an Openstack module which has service endpoints.
type ServiceProvider interface {
	Endpoints() []Endpoint
}
