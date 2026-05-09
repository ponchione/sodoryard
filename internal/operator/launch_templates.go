package operator

import (
	"context"
	"strings"
)

const launchReceiptSchemaV1 = "yard.receipt.v1"

var launchTemplates = []LaunchTemplate{
	{
		ID:              "constrained_orchestration",
		Mode:            LaunchModeConstrained,
		Label:           "Constrained Orchestration",
		Description:     "Run the orchestrator with an explicit allowlist of roles.",
		DefaultRoles:    []string{"orchestrator"},
		ReceiptSchema:   launchReceiptSchemaV1,
		PreflightChecks: []string{"task_or_specs", "orchestrator_role", "allowed_roles", "model_tools"},
	},
	{
		ID:              "manual_roster",
		Mode:            LaunchModeManualRoster,
		Label:           "Manual Roster",
		Description:     "Run a fixed sequence of selected roles.",
		ReceiptSchema:   launchReceiptSchemaV1,
		PreflightChecks: []string{"task_or_specs", "roster_roles", "model_tools"},
	},
	{
		ID:              "one_step",
		Mode:            LaunchModeOneStep,
		Label:           "One Step",
		Description:     "Run one selected role for a task or spec set.",
		ReceiptSchema:   launchReceiptSchemaV1,
		PreflightChecks: []string{"task_or_specs", "role", "model_tools"},
	},
	{
		ID:              "sir_topham_decides",
		Mode:            LaunchModeOrchestrator,
		Label:           "Sir Topham Decides",
		Description:     "Run the orchestrator with full role-selection authority.",
		DefaultRoles:    []string{"orchestrator"},
		ReceiptSchema:   launchReceiptSchemaV1,
		PreflightChecks: []string{"task_or_specs", "orchestrator_role", "model_tools"},
	},
}

func (s *Service) ListLaunchTemplates(ctx context.Context) ([]LaunchTemplate, error) {
	_ = ctx
	return ListLaunchTemplates(), nil
}

func ListLaunchTemplates() []LaunchTemplate {
	out := make([]LaunchTemplate, 0, len(launchTemplates))
	for _, template := range launchTemplates {
		out = append(out, cloneLaunchTemplate(template))
	}
	return out
}

func LaunchTemplateForID(id string) (LaunchTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, template := range launchTemplates {
		if template.ID == id {
			return cloneLaunchTemplate(template), true
		}
	}
	return LaunchTemplate{}, false
}

func launchTemplateForMode(mode LaunchMode) (LaunchTemplate, bool) {
	for _, template := range launchTemplates {
		if template.Mode == mode {
			return cloneLaunchTemplate(template), true
		}
	}
	return LaunchTemplate{}, false
}

func cloneLaunchTemplate(template LaunchTemplate) LaunchTemplate {
	template.DefaultRoles = append([]string(nil), template.DefaultRoles...)
	template.PreflightChecks = append([]string(nil), template.PreflightChecks...)
	return template
}
