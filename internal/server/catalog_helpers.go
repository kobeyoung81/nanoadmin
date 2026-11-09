package server

import "net/http"

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

// userIDFromQuery reads the user a user-facing call acts for. The portal is the
// authentication boundary and passes it explicitly; this service trusts it, the
// same contract the bus had.
func userIDFromQuery(r *http.Request) string {
	return r.URL.Query().Get("user_id")
}
