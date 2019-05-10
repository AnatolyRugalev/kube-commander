package commands

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/spf13/cobra"
)

func init() {
	cfg.AddCommand(&cobra.Command{
		Use:   "overview",
		Short: "Shows overview screen with general cluster information",
		RunE:  overview,
	})
}

func overview(cmd *cobra.Command, args []string) error {
	client, err := kube.GetClient()
	if err != nil {
		return err
	}
	pods, err := client.GetPods("")
	if err != nil {
		return err
	}
	cmd.Print(pods)
	return nil
}
