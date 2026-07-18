// Command kubecom is a fast, keyboard-driven terminal UI for Kubernetes.
//
// This is the M0 groundwork skeleton: it wires the binary entrypoint and a
// `version` subcommand. The Bubble Tea UI and the in-process kube layer land in
// later milestones; cobra replaces this hand-rolled dispatch in M0-02. The
// legacy 2020 app remains reachable via cmd/kube-commander until it is ported.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/AnatolyRugalev/kube-commander/internal/version"
)

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kubecom:", err)
		os.Exit(1)
	}
}

const usage = `kubecom — a Kubernetes terminal UI (groundwork build)

Usage:
  kubecom version    print build information
  kubecom help       show this message`

func run(out io.Writer, args []string) error {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "version", "--version", "-v":
		_, err := fmt.Fprintln(out, version.Info())
		return err
	case "", "help", "--help", "-h":
		_, err := fmt.Fprintln(out, usage)
		return err
	default:
		return fmt.Errorf("unknown command %q (try \"kubecom help\")", cmd)
	}
}
