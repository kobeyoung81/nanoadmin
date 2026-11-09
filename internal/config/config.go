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
	// InternalToken is the shared secret the platform's own callers present
	// (NANOSEC_BUS_INTERNAL_TOKEN). Empty means the internal endpoints refuse
	// every request rather than serving them unauthenticated.
	InternalToken string
}

// Load reads the configuration from the environment.
func Load() Config {
	return Config{
		Addr:          env("NANOSEC_ADMIN_ADDR", ":8080"),
		MongoURI:      firstNonEmpty(os.Getenv("NANOSEC_ADMIN_MONGO_URI"), os.Getenv("NANOSEC_BUS_DATABASE_URI")),
		InternalToken: os.Getenv("NANOSEC_BUS_INTERNAL_TOKEN"),
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
