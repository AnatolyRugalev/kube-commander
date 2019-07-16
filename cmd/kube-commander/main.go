package main

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/AnatolyRugalev/kube-commander/internal/tui"
	_ "github.com/AnatolyRugalev/kube-commander/internal/tui"
	"github.com/spf13/cobra"
	"os"
)

var version = "unknown"

var rootCmd = &cobra.Command{
	Use:     "kube-commander",
	Version: version,
	Short:   "kube-commander allows you to browse Kubernetes in a casual way!",
	Long:    `Get a full-blown Kubernetes dashboard inside your terminal window!`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cfg.Apply()
	},
	Run: func(cmd *cobra.Command, args []string) {
		tui.Start()
	},
}

func main() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Shows kube-commander version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "env",
		Short: "Shows kube-commander version",
		Run: func(cmd *cobra.Command, args []string) {
			for _, pair := range os.Environ() {
				fmt.Println(pair)
			}
		},
	})
	if err := cfg.Setup(rootCmd); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
