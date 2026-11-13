// Package config defines configuration for the nanoadmin service.
package config

import "os"

// Config is the whole runtime configuration of this service.
//
// Everything is an environment variable: this module owns no connection-level
// settings a console would need to change, and the values it does read are the
// ones the deployment already gives every service.
type Config struct {
	// Addr is the listen address (NANOSEC_ADMIN_ADDR, default ":8080").
	Addr string
	// MongoURI names the platform database (NANOSEC_ADMIN_MONGO_URI, falling back
	// to the bus's variable so a deployment only has to fill in one URI).
	MongoURI string
	// BusURL is the message bus: it owns the agent rows, so deleting a type has
	// to ask it to clear the references first.
	BusURL string
	// ProvisionerURL is the container queue's owner. Restart, dispose and
	// reprovision are desired-state writes there; this service is where the
	// console calls them.
	ProvisionerURL string
	// InternalToken is the shared secret the platform's own callers present
	// (NANOSEC_BUS_INTERNAL_TOKEN). Empty means the internal endpoints refuse
	// every request rather than serving them unauthenticated.
	InternalToken string
	// JWKSURL is nanoauth's key set (NANOAUTH_JWKS_URL). The console will be
	// routed to this service directly, so it verifies browser tokens itself; an
	// empty URL leaves only the internal token, which is the rolling-upgrade
	// case.
	JWKSURL string
}

// Load reads the configuration from the environment.
func Load() Config {
	return Config{
		Addr:           env("NANOSEC_ADMIN_ADDR", ":8080"),
		MongoURI:       firstNonEmpty(os.Getenv("NANOSEC_ADMIN_MONGO_URI"), os.Getenv("NANOSEC_BUS_DATABASE_URI")),
		BusURL:         env("NANOSEC_BUS_URL", "http://nanobus:9000"),
		ProvisionerURL: env("NANOSEC_ADMIN_PROVISIONER_URL", "http://nanorunner:8080"),
		InternalToken:  os.Getenv("NANOSEC_BUS_INTERNAL_TOKEN"),
		JWKSURL:        env("NANOAUTH_JWKS_URL", "http://nanoauth:8081/auth/v1/.well-known/jwks"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
