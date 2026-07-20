package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"
)

// setupLogging routes all diagnostic output to a log file so nothing corrupts the
// alt-screen while the TUI owns the terminal (stack.md logging rule): the stdlib
// slog default handler is pointed at the file, and klog — which client-go uses and
// which defaults to stderr — is redirected to the same file. The returned *os.File
// is the caller's to Close once the program exits. The log lives under the user
// cache dir (os.UserCacheDir()/kubecom/kubecom.log), created lazily.
func setupLogging() (*os.File, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locating user cache dir: %w", err)
	}
	dir = filepath.Join(dir, "kubecom")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "kubecom.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", path, err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})))
	// Route client-go/klog to the file and keep it off stderr, so a connection or
	// discovery warning never writes over the alt-screen. LogToStderr(false) alone
	// is not enough: klog still copies ERROR-level lines to stderr unless the stderr
	// *threshold* is raised, which is exactly the memcache "couldn't get server API
	// group list" line client-go emits on an unreachable cluster. Configure both
	// through a private flag set (no global flag pollution) and point the file
	// output at our log.
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("alsologtostderr", "false")
	_ = fs.Set("stderrthreshold", "FATAL")
	klog.SetOutput(f)
	return f, nil
}
