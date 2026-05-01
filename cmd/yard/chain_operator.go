package main

import (
	"context"
	"errors"

	"github.com/ponchione/sodoryard/internal/operator"
)

var openYardOperator = func(ctx context.Context, configPath string) (*operator.Service, error) {
	return operator.Open(ctx, operator.Options{
		ConfigPath:      configPath,
		BuildRuntime:    buildYardChainRuntime,
		ProcessSignaler: signalYardOperatorProcess,
	})
}

var openYardReadOnlyOperator = func(ctx context.Context, configPath string) (*operator.Service, error) {
	return operator.Open(ctx, operator.Options{
		ConfigPath:      configPath,
		BuildRuntime:    buildYardChainRuntime,
		ProcessSignaler: signalYardOperatorProcess,
		ReadOnly:        true,
	})
}

func signalYardOperatorProcess(pid int) error {
	err := interruptYardChainPID(pid)
	if errors.Is(err, errYardChainPIDNotRunning) {
		return operator.ErrProcessNotRunning
	}
	return err
}
