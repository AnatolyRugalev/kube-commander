package main

import (
	"flag"
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app"
	"github.com/AnatolyRugalev/kube-commander/app/builder"
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/executor"
	"github.com/spf13/cobra"
	cmd "k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"os"
	"strconv"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	_ "k8s.io/client-go/plugin/pkg/client/auth/openstack"
)

var version = "unknown"

var rootCmd = &cobra.Command{
	Use:     "kube-commander",
	Version: version,
	Long:    "Browse your Kubernetes clusters in a casual way",
	RunE:    run,
}

var logFlags = flag.NewFlagSet("klog", flag.ExitOnError)

var cfg = struct {
	editor     string
	pager      string
	tail       int
	kubectl    string
	kubeconfig string
	context    string
	namespace  string
	klog       string
}{}

const (
	KubectlEnv   = "KUBECTL"
	EditorEnv    = "EDITOR"
	PagerEnv     = "PAGER"
	TailEnv      = "KUBETAIL"
	ContextEnv   = "KUBECONTEXT"
	NamespaceEnv = "KUBENAMESPACE"
	KLogEnv      = "KUBELOG"
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

func defaultEnvInt(name string, def int) int {
	val := os.Getenv(name)
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}

func init() {
	rootCmd.Flags().StringVarP(&cfg.kubectl, "kubectl", "k", defaultEnv(KubectlEnv, "kubectl"), "kubectl path override")
	rootCmd.Flags().StringVarP(&cfg.editor, "editor", "e", defaultEnv(EditorEnv, ""), "Editor override")
	rootCmd.Flags().StringVarP(&cfg.pager, "pager", "p", defaultEnv(PagerEnv, "less"), "Pager override")
	rootCmd.Flags().IntVarP(&cfg.tail, "tail", "t", defaultEnvInt(TailEnv, 1000), "Number of lines when viewing logs")
	rootCmd.Flags().StringVarP(&cfg.kubeconfig, "kubeconfig", "", os.Getenv(cmd.RecommendedConfigPathEnvVar), "Kubeconfig override")
	rootCmd.Flags().StringVarP(&cfg.context, "context", "c", defaultEnv(ContextEnv, ""), "Context name (default: current context)")
	rootCmd.Flags().StringVarP(&cfg.namespace, "namespace", "n", defaultEnv(NamespaceEnv, ""), "Namespace name to start with (default: from context)")
	rootCmd.Flags().StringVarP(&cfg.klog, "klog", "", defaultEnv(KLogEnv, ""), "Log file for Kubernetes logging library")
	klog.InitFlags(logFlags)
	_ = logFlags.Set("logtostderr", "false")
	_ = logFlags.Set("alsologtostderr", "false")
	_ = logFlags.Set("v", "0")
}

func run(_ *cobra.Command, _ []string) error {
	_ = logFlags.Set("log_file", cfg.klog)
	_ = os.Setenv(cmd.RecommendedConfigPathEnvVar, cfg.kubeconfig)
	conf := client.NewDefaultConfig(cfg.kubeconfig, cfg.context, cfg.namespace)
	cl, err := client.NewClient(conf)
	if err != nil {
		return fmt.Errorf("could not initialize kubernetes client: %w", err)
	}
	b := builder.NewBuilder(conf, cfg.kubectl, cfg.pager, cfg.editor, cfg.tail)
	application := app.NewApp(conf, cl, cl, b, executor.NewOsExecutor(), conf.Namespace())
	return application.Run()
}
