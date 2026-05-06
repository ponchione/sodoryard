package main

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/chaininput"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

func validateYardChainFlags(flags yardChainFlags) error {
	if strings.TrimSpace(flags.Task) == "" && strings.TrimSpace(flags.Specs) == "" && strings.TrimSpace(flags.ChainID) == "" {
		return fmt.Errorf("one of --task or --specs is required")
	}
	if flags.MaxSteps <= 0 {
		return fmt.Errorf("--max-steps must be > 0")
	}
	if flags.MaxResolverLoops < 0 {
		return fmt.Errorf("--max-resolver-loops must be >= 0")
	}
	if flags.MaxDuration <= 0 {
		return fmt.Errorf("--max-duration must be > 0")
	}
	if flags.TokenBudget <= 0 {
		return fmt.Errorf("--token-budget must be > 0")
	}
	if flags.StepMaxTurns < 0 {
		return fmt.Errorf("--step-max-turns must be > 0 when supplied")
	}
	if flags.StepMaxTokens < 0 {
		return fmt.Errorf("--step-max-tokens must be > 0 when supplied")
	}
	return nil
}

func applyYardChainOverrides(cfg *appconfig.Config, flags yardChainFlags) {
	if strings.TrimSpace(flags.ProjectRoot) != "" {
		cfg.ProjectRoot = strings.TrimSpace(flags.ProjectRoot)
	}
}

func yardParseSpecs(specs string) []string {
	return chaininput.ParseSpecs(specs)
}
