// Package log holds this service's logging conventions.
package log

import "log/slog"

// LevelVerbose is a custom level below DEBUG for high-frequency internals.
// Enable with LOG_LEVEL=VERBOSE.
const LevelVerbose = slog.Level(-8)
