// Command kubecom is a fast, keyboard-driven terminal UI for Kubernetes.
//
// The root command builds a live cluster client from the kubeconfig/context/
// namespace flags and runs the Bubble Tea browse UI (see run.go); `version` and
// `keys` hang off it as subcommands.
package main

import (
	"fmt"
	"os"

	"github.com/neuroplastio/kubecom/internal/version"
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
	var opts runOptions
	root := &cobra.Command{
		Use:   "kubecom",
		Short: "A fast, keyboard-driven terminal UI for Kubernetes",
		Long: `kubecom is a fast, vim-friendly, zero-deploy Kubernetes terminal UI.

Run bare "kubecom" to launch the browse UI against your current cluster; the
kubeconfig, context, and namespace are selectable with the flags below. The
"version" and "keys" subcommands report build info and the resolved keybindings.`,
		Version: version.Info(),
		// Bare kubecom (no subcommand) launches the TUI; NoArgs keeps an unknown
		// command a clear error rather than a positional argument to the launcher.
		Args: cobra.NoArgs,
		// Errors are surfaced by Execute; don't also dump usage on a runtime
		// error, and keep args-validation errors terse.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Record whether -n was passed explicitly: an explicit namespace wins for
			// this run over the stored last-namespace (D91), and "" from the flag's
			// default must be distinguishable from "" the user typed.
			opts.namespaceSet = cmd.Flags().Changed("namespace")
			return runTUI(opts)
		},
	}
	// --version prints the build summary verbatim (Info() already leads with
	// "kubecom"), not cobra's default "<name> version <version>" line.
	root.SetVersionTemplate("{{.Version}}\n")
	// Launch flags are local to the root run (the keys subcommand has its own
	// --config), so they don't leak onto subcommands as persistent flags would.
	f := root.Flags()
	f.StringVar(&opts.kubeconfig, "kubeconfig", "",
		"path to the kubeconfig file (default: $KUBECONFIG, else ~/.kube/config)")
	f.StringVar(&opts.context, "context", "",
		"kubeconfig context to use (default: the file's current-context)")
	f.StringVarP(&opts.namespace, "namespace", "n", "",
		"namespace to scope the initial view to (default: all namespaces)")
	f.StringVar(&opts.configPath, "config", "",
		"kubecom config file to load (default: user config dir /kubecom/config.yaml)")
	root.AddCommand(newVersionCmd())
	root.AddCommand(newKeysCmd())
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
