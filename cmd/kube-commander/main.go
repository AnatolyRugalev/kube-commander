package main

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app"
	"github.com/AnatolyRugalev/kube-commander/app/builder"
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/executor"
	"github.com/spf13/cobra"
	"os"
)

var version = "0.0.0"

var rootCmd = &cobra.Command{
	Use:     "kube-commander",
	Version: version,
	Long:    "Browse your Kubernetes clusters in a casual way",
	RunE:    run,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(_ *cobra.Command, _ []string) error {
	conf := client.NewDefaultConfig()
	cl, err := client.NewClient(conf)
	if err != nil {
		return err
	}
	// TODO: flag based configuration
	// TODO: env based kubectl path
	b := builder.NewBuilder(conf, "kubectl", "less", os.Getenv("EDITOR"))
	application := app.NewApp(cl, cl, b, executor.NewOsExecutor(), "kube-system")
	return application.Run()
}
