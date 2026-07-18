package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestInfo(t *testing.T) {
	got := Info()
	for _, want := range []string{"kubecom", Version, Commit, Date, runtime.Version()} {
		if !strings.Contains(got, want) {
			t.Errorf("Info() = %q, want it to contain %q", got, want)
		}
	}
}
