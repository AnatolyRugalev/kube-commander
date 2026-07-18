// Command kubecom is a fast, keyboard-driven terminal UI for Kubernetes.
//
// This is the M0 groundwork skeleton: it wires the binary entrypoint and a
// `version` subcommand on cobra (M0-02). The Bubble Tea UI and the in-process
// kube layer land in later milestones. The legacy 2020 app remains reachable
// via cmd/kube-commander until it is ported.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/AnatolyRugalev/kube-commander/internal/version"
)

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kubecom:", err)
		os.Exit(1)
	}
}

// run builds the root command, points its output at out, and executes it
// with args. Kept as a seam so tests can drive the CLI without touching
// os.Stdout/os.Args.
func run(out io.Writer, args []string) error {
	root := newRootCmd()
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	return root.Execute()
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kubecom",
		Short: "kubecom — a Kubernetes terminal UI (groundwork build)",
		// Errors/usage are reported by main() itself (D-style parity with the
		// pre-cobra dispatch); don't let cobra print them a second time.
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	// Out of scope for the M0 groundwork CLI; drop the default completion
	// subcommand until shell completion is an explicit task.
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version.Info())
			return err
		},
	}
}
