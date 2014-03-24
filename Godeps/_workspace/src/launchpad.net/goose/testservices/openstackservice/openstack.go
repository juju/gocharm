package openstackservice

import (
	"fmt"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/testservices/identityservice"
	"launchpad.net/goose/testservices/novaservice"
	"launchpad.net/goose/testservices/swiftservice"
	"net/http"
	"strings"
)

// Openstack provides an Openstack service double implementation.
type Openstack struct {
	Identity identityservice.IdentityService
	Nova     *novaservice.Nova
	Swift    *swiftservice.Swift
}

// New creates an instance of a full Openstack service double.
// An initial user with the specified credentials is registered with the identity service.
func New(cred *identity.Credentials, authMode identity.AuthMode) *Openstack {
	var openstack Openstack
	if authMode == identity.AuthKeyPair {
		openstack = Openstack{
			Identity: identityservice.NewKeyPair(),
		}
	} else {
		openstack = Openstack{
			Identity: identityservice.NewUserPass(),
		}
	}
	userInfo := openstack.Identity.AddUser(cred.User, cred.Secrets, cred.TenantName)
	if cred.TenantName == "" {
		panic("Openstack service double requires a tenant to be specified.")
	}
	openstack.Nova = novaservice.New(cred.URL, "v2", userInfo.TenantId, cred.Region, openstack.Identity)
	// Create the swift service using only the region base so we emulate real world deployments.
	regionParts := strings.Split(cred.Region, ".")
	baseRegion := regionParts[len(regionParts)-1]
	openstack.Swift = swiftservice.New(cred.URL, "v1", userInfo.TenantId, baseRegion, openstack.Identity)
	// Create container and add image metadata endpoint so that product-streams URLs are included
	// in the keystone catalog.
	err := openstack.Swift.AddContainer("imagemetadata")
	if err != nil {
		panic(fmt.Errorf("setting up image metadata container: %v", err))
	}
	url := openstack.Swift.Endpoints()[0].PublicURL
	serviceDef := identityservice.Service{"simplestreams", "product-streams", []identityservice.Endpoint{
		identityservice.Endpoint{PublicURL: url + "/imagemetadata", Region: cred.Region},
	}}
	openstack.Identity.AddService(serviceDef)
	// Add public bucket endpoint so that juju-tools URLs are included in the keystone catalog.
	serviceDef = identityservice.Service{"juju", "juju-tools", []identityservice.Endpoint{
		identityservice.Endpoint{PublicURL: url, Region: cred.Region},
	}}
	openstack.Identity.AddService(serviceDef)
	return &openstack
}

// SetupHTTP attaches all the needed handlers to provide the HTTP API for the Openstack service..
func (openstack *Openstack) SetupHTTP(mux *http.ServeMux) {
	openstack.Identity.SetupHTTP(mux)
	openstack.Nova.SetupHTTP(mux)
	openstack.Swift.SetupHTTP(mux)
}
