package main

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui"
	"github.com/spf13/cobra"
	"os"
)

var version = "0.0.0"

var rootCmd = &cobra.Command{
	Use:     "kube-commander",
	Version: version,
	Long:    "Browse your Kubernetes clusters in a casual way",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cfg.Apply()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := kube.InitClient()
		if err != nil {
			return err
		}
		tui.Start()
		return nil
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
