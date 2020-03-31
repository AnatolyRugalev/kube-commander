package main

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app"
	"github.com/AnatolyRugalev/kube-commander/app/builder"
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/executor"
	"github.com/spf13/cobra"
	cmd "k8s.io/client-go/tools/clientcmd"
	"os"
)

var version = "unknown"

var rootCmd = &cobra.Command{
	Use:     "kube-commander",
	Version: version,
	Long:    "Browse your Kubernetes clusters in a casual way",
	RunE:    run,
}

var cfg = struct {
	editor     string
	pager      string
	kubectl    string
	kubeconfig string
	context    string
	namespace  string
}{}

const (
	EditorEnv = "EDITOR"
	PagerEnv  = "PAGER"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func defaultEnv(name string, def string) string {
	val := os.Getenv(name)
	if val == "" {
		return def
	}
	return val
}

func init() {
	rootCmd.Flags().StringVarP(&cfg.kubectl, "kubectl", "k", "kubectl", "kubectl path override")
	rootCmd.Flags().StringVarP(&cfg.editor, "editor", "e", defaultEnv(EditorEnv, "vi"), "Editor override")
	rootCmd.Flags().StringVarP(&cfg.pager, "pager", "p", defaultEnv(PagerEnv, "less"), "Pager override")
	rootCmd.Flags().StringVarP(&cfg.kubeconfig, "kubeconfig", "", os.Getenv(cmd.RecommendedConfigPathEnvVar), "Kubeconfig override")
	rootCmd.Flags().StringVarP(&cfg.context, "context", "c", "", "Context name (default: current context)")
	rootCmd.Flags().StringVarP(&cfg.namespace, "namespace", "n", "", "Namespace name to start with (default: from context)")
}

func run(_ *cobra.Command, _ []string) error {
	_ = os.Setenv(cmd.RecommendedConfigPathEnvVar, cfg.kubeconfig)
	conf := client.NewDefaultConfig(cfg.kubeconfig, cfg.context, cfg.namespace)
	cl, err := client.NewClient(conf)
	if err != nil {
		return err
	}
	b := builder.NewBuilder(conf, cfg.kubectl, cfg.pager, cfg.editor)
	application := app.NewApp(cl, cl, b, executor.NewOsExecutor(), cfg.namespace)
	return application.Run()
}
