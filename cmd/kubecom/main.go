// Command kubecom is a fast, keyboard-driven terminal UI for Kubernetes.
//
// This is the M0 groundwork skeleton: it wires the cobra command tree and a
// `version` subcommand. The Bubble Tea UI and the in-process kube layer land in
// later milestones, hanging new subcommands / the default TUI run off this root.
package main

import (
	"fmt"
	"os"

	"github.com/AnatolyRugalev/kube-commander/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the kubecom command tree. It is a function (not a package
// var) so tests can construct an isolated command with its own I/O and args.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kubecom",
		Short: "A fast, keyboard-driven terminal UI for Kubernetes",
		Long: `kubecom is a fast, vim-friendly, zero-deploy Kubernetes terminal UI.

This is the M0 groundwork build: it wires the binary entrypoint and a version
subcommand. The Bubble Tea UI and the in-process kube layer land in later
milestones.`,
		Version: version.Info(),
		// Errors are surfaced by Execute; don't also dump usage on a runtime
		// error, and keep args-validation errors terse.
		SilenceUsage: true,
	}
	// --version prints the build summary verbatim (Info() already leads with
	// "kubecom"), not cobra's default "<name> version <version>" line.
	root.SetVersionTemplate("{{.Version}}\n")
	root.AddCommand(newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version.Info())
			return err
		},
	}
}
