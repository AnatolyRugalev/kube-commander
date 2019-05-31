package test

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

func init() {
	cfg.AddCommand(&cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {
			Start()
		},
	})
}

func Start() {
	cmd := exec.Command("nano", "-nw")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

}
