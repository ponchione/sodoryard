package main

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/chaininput"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/operator"
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
		return fmt.Errorf("--step-max-turns must not be negative")
	}
	if flags.StepMaxTokens < 0 {
		return fmt.Errorf("--step-max-tokens must not be negative")
	}
	if err := validateYardChainTemplateFlags(flags); err != nil {
		return err
	}
	return nil
}

func validateYardChainTemplateFlags(flags yardChainFlags) error {
	mode, err := yardChainModeFromTemplate(flags.TemplateID)
	if err != nil {
		return err
	}
	role := strings.TrimSpace(flags.Role)
	allowedRoles := yardParseRoles(flags.AllowedRoles)
	roster := yardParseRoster(flags.Roster)

	if len(roster) > 0 && (role != "" || len(allowedRoles) > 0) {
		return fmt.Errorf("--roster cannot be combined with --role or --allowed-roles")
	}
	if len(allowedRoles) > 0 && role != "" {
		return fmt.Errorf("--allowed-roles cannot be combined with --role")
	}
	switch mode {
	case "":
		return nil
	case chainrun.ModeOneStep:
		if role == "" {
			return fmt.Errorf("--template one_step requires --role")
		}
		if len(allowedRoles) > 0 || len(roster) > 0 {
			return fmt.Errorf("--template one_step cannot be combined with --allowed-roles or --roster")
		}
	case chainrun.ModeManualRoster:
		if len(roster) == 0 {
			return fmt.Errorf("--template manual_roster requires --roster")
		}
	case chainrun.ModeConstrained:
		if len(allowedRoles) == 0 && role == "" {
			return fmt.Errorf("--template constrained_orchestration requires --allowed-roles or --role")
		}
		if len(roster) > 0 {
			return fmt.Errorf("--template constrained_orchestration cannot be combined with --roster")
		}
	case chainrun.ModeOrchestrator:
		if role != "" || len(allowedRoles) > 0 || len(roster) > 0 {
			return fmt.Errorf("--template sir_topham_decides cannot be combined with --role, --allowed-roles, or --roster")
		}
	}
	return nil
}

func yardChainOptionsFromFlags(flags yardChainFlags) (chainrun.Options, error) {
	mode, err := yardChainModeFromTemplate(flags.TemplateID)
	if err != nil {
		return chainrun.Options{}, err
	}
	return chainrun.Options{
		ChainID:          flags.ChainID,
		Mode:             mode,
		Role:             strings.TrimSpace(flags.Role),
		AllowedRoles:     yardParseRoles(flags.AllowedRoles),
		Roster:           yardParseRoster(flags.Roster),
		SourceSpecs:      yardParseSpecs(flags.Specs),
		SourceTask:       strings.TrimSpace(flags.Task),
		MaxSteps:         flags.MaxSteps,
		MaxResolverLoops: flags.MaxResolverLoops,
		MaxDuration:      flags.MaxDuration,
		TokenBudget:      flags.TokenBudget,
		StepMaxTurns:     flags.StepMaxTurns,
		StepMaxTokens:    flags.StepMaxTokens,
		DryRun:           flags.DryRun,
	}, nil
}

func yardChainModeFromTemplate(templateID string) (chainrun.Mode, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return "", nil
	}
	template, ok := operator.LaunchTemplateForID(templateID)
	if !ok {
		return "", fmt.Errorf("unknown launch template %q", templateID)
	}
	return chainrun.Mode(template.Mode), nil
}

func applyYardChainOverrides(cfg *appconfig.Config, flags yardChainFlags) {
	if strings.TrimSpace(flags.ProjectRoot) != "" {
		cfg.ProjectRoot = strings.TrimSpace(flags.ProjectRoot)
	}
}

func yardParseSpecs(specs string) []string {
	return chaininput.ParseSpecs(specs)
}

func yardParseRoles(roles string) []string {
	return chaininput.ParseRoleSet(roles)
}

func yardParseRoster(roster string) []chainrun.StepRequest {
	roles := chaininput.ParseRoleList(roster)
	if len(roles) == 0 {
		return nil
	}
	requests := make([]chainrun.StepRequest, 0, len(roles))
	for _, role := range roles {
		requests = append(requests, chainrun.StepRequest{Role: role})
	}
	return requests
}
