package app

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
)

type appExecutor struct {
	executor commander.CommandExecutor
	app      *app
}

func NewAppExecutor(a *app, executor commander.CommandExecutor) *appExecutor {
	return &appExecutor{
		app:      a,
		executor: executor,
	}
}

func (s appExecutor) Pipe(command ...*commander.Command) error {
	return s.app.Interrupt(func() error {
		return s.executor.Pipe(command...)
	})
}
