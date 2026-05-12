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

var openYardDegradedOperator = func(ctx context.Context, configPath string, cause error) (*operator.Service, error) {
	return operator.Open(ctx, operator.Options{
		ConfigPath:      configPath,
		BuildRuntime:    buildYardChainRuntime,
		ProcessSignaler: signalYardOperatorProcess,
		ReadOnly:        true,
		StartupWarnings: []operator.RuntimeWarning{{
			Message: degradedOperatorWarning(cause),
		}},
	})
}

func signalYardOperatorProcess(pid int) error {
	err := interruptYardChainPID(pid)
	if errors.Is(err, errYardChainPIDNotRunning) {
		return operator.ErrProcessNotRunning
	}
	return err
}

func degradedOperatorWarning(cause error) string {
	if cause == nil {
		return "opened operator in degraded read-only mode"
	}
	return "opened operator in degraded read-only mode because full runtime startup failed: " + cause.Error()
}
