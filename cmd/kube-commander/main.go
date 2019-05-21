package main

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/AnatolyRugalev/kube-commander/internal/tui"
	_ "github.com/AnatolyRugalev/kube-commander/internal/tui"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "kube-commander",
	Short: "Kube Commander allows you to browse Kubernetes in a casual way!",
	Long: `Get a full-blown Kubernetes dashboard inside your terminal window!
	List pods, scale deployments and more!`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cfg.Apply()
	},
	Run: func(cmd *cobra.Command, args []string) {
		tui.Start()
	},
}

func main() {
	if err := cfg.Setup(rootCmd); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
