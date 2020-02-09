package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	app "github.com/AnatolyRugalev/kube-commander/internal/tcell/app"
)

var Application = &struct {
	Debug bool `mapstructure:"debug"`

	Kubeconfig string `mapstructure:"kubeconfig"`
	Context    string `mapstructure:"context"`
	Namespace  string `mapstructure:"namespace"`
}{}

func init() {
	cfg.AddPkg(&cfg.Pkg{
		Struct: Application,
		PersistentFlags: cfg.FlagsDeclaration{
			"debug":      {false, "Enables debug to STDERR", "KUBEDEBUG"},
			"kubeconfig": {"", "Kubernetes kubeconfig path", ""},
			"context":    {"", "Kubernetes context to use", "KUBECONTEXT"},
			"namespace":  {"default", "Kubernetes context to use", "KUBENAMESPACE"},
		},
	})
}

var screen *Screen

func Start() error {
	c, err := client.NewClient(client.NewCmdConfigProvider(
		Application.Kubeconfig,
		Application.Context,
	))
	if err != nil {
		return err
	}
	a := app.New(c, Application.Namespace)
	err = a.InitScreen()
	if err != nil {
		return err
	}
	return a.Run()
}
