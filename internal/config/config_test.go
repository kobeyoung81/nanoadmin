package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("NANO_ADMIN_ADDR", "")
	t.Setenv("NANO_ADMIN_MONGO_URI", "")
	t.Setenv("NANOSEC_BUS_DATABASE_URI", "")
	t.Setenv("NANOSEC_BUS_INTERNAL_TOKEN", "")

	cfg := Load()
	if cfg.Addr != ":8080" {
		t.Errorf("addr = %q, want :8080", cfg.Addr)
	}
	if cfg.MongoURI != "" || cfg.InternalToken != "" {
		t.Errorf("expected empty uri/token, got %q / %q", cfg.MongoURI, cfg.InternalToken)
	}
}

// The deployment already carries the platform URI under the bus's name; a
// second variable for the same database would be a second thing to get wrong.
func TestLoadFallsBackToThePlatformURI(t *testing.T) {
	t.Setenv("NANO_ADMIN_MONGO_URI", "")
	t.Setenv("NANOSEC_BUS_DATABASE_URI", "mongodb://host/nanosec")
	cfg := Load()
	if cfg.MongoURI != "mongodb://host/nanosec" {
		t.Fatalf("uri = %q", cfg.MongoURI)
	}
}

func TestLoadPrefersItsOwnURI(t *testing.T) {
	t.Setenv("NANO_ADMIN_MONGO_URI", "mongodb://own/nanosec")
	t.Setenv("NANOSEC_BUS_DATABASE_URI", "mongodb://platform/nanosec")
	cfg := Load()
	if cfg.MongoURI != "mongodb://own/nanosec" {
		t.Fatalf("uri = %q", cfg.MongoURI)
	}
}
