package server

import (
	"net/http"

	"gitee.com/kobeyoung81/nanoadmin/internal/auth"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

// maskValue is what the console sees in place of a secret; a save carrying it
// back unchanged means "leave the stored value alone".
const maskValue = "***"

// nonNilSlice keeps an empty list rendering as [] rather than null.
func nonNilSlice[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

// maskItems blanks every secret value. A service caller gets the real value (the
// agent and nanoauth read the JWT keypair to do their job); a browser caller
// gets the mask, and a save carrying the mask back means "leave it alone".
func maskItems(items []store.ConfigItem) []store.ConfigItem {
	// Copy: the slice comes from a store call today, but a caller that reuses
	// it (a test fake, a cache) must not lose the real value to masking.
	masked := make([]store.ConfigItem, len(items))
	copy(masked, items)
	for i := range masked {
		if masked[i].ValueType == "secret" {
			masked[i].Value = maskValue
		}
	}
	return masked
}

// wantsMaskedValues reports whether this read must hide secrets. The console
// asks for it explicitly (the portal is the sole caller that reads on behalf of
// a browser and still holds the service token); any caller authenticated by a
// JWT is masked unconditionally.
func wantsMaskedValues(r *http.Request) bool {
	if r.URL.Query().Get("mask") == "1" {
		return true
	}
	if auth.IsServiceCaller(r.Context()) {
		return false
	}
	return true
}
