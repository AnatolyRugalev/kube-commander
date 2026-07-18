// Package version exposes build metadata for the kubecom binary. The values are
// overridden at release time via -ldflags; local builds report "dev".
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic version of the build ("dev" for local builds).
	Version = "dev"
	// Commit is the git commit the binary was built from.
	Commit = "none"
	// Date is the build timestamp (RFC3339) or "unknown".
	Date = "unknown"
)

// Info returns a human-readable one-line build summary.
func Info() string {
	return fmt.Sprintf("kubecom %s (commit %s, built %s, %s)",
		Version, Commit, Date, runtime.Version())
}
