package operator

import (
	"context"
	"encoding/json"
	"strings"
)

const launchReceiptSchemaV1 = "yard.receipt.v1"

var launchTemplates = []LaunchTemplate{
	{
		ID:              "constrained_orchestration",
		Mode:            LaunchModeConstrained,
		Label:           "Constrained Orchestration",
		Description:     "Run the orchestrator with an explicit allowlist of roles.",
		InputSchema:     constrainedLaunchInputSchema,
		DefaultRoles:    []string{"orchestrator"},
		ReceiptSchema:   launchReceiptSchemaV1,
		PreflightChecks: []string{"task_or_specs", "orchestrator_role", "allowed_roles", "model_tools"},
	},
	{
		ID:              "manual_roster",
		Mode:            LaunchModeManualRoster,
		Label:           "Manual Roster",
		Description:     "Run a fixed sequence of selected roles.",
		InputSchema:     manualRosterLaunchInputSchema,
		ReceiptSchema:   launchReceiptSchemaV1,
		PreflightChecks: []string{"task_or_specs", "roster_roles", "model_tools"},
	},
	{
		ID:              "one_step",
		Mode:            LaunchModeOneStep,
		Label:           "One Step",
		Description:     "Run one selected role for a task or spec set.",
		InputSchema:     oneStepLaunchInputSchema,
		ReceiptSchema:   launchReceiptSchemaV1,
		PreflightChecks: []string{"task_or_specs", "role", "model_tools"},
	},
	{
		ID:              "sir_topham_decides",
		Mode:            LaunchModeOrchestrator,
		Label:           "Sir Topham Decides",
		Description:     "Run the orchestrator with full role-selection authority.",
		InputSchema:     orchestratorLaunchInputSchema,
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
	template.InputSchema = append(json.RawMessage(nil), template.InputSchema...)
	template.DefaultRoles = append([]string(nil), template.DefaultRoles...)
	template.PreflightChecks = append([]string(nil), template.PreflightChecks...)
	return template
}

var commonLaunchInputProperties = `"task":{"type":"string","description":"Free-form task description."},"specs":{"type":"array","items":{"type":"string"},"description":"Brain-relative spec document paths."},"max_steps":{"type":"integer","minimum":1},"max_resolver_loops":{"type":"integer","minimum":0},"max_duration":{"type":"string","description":"Go duration string such as 10m."},"token_budget":{"type":"integer","minimum":1}`

var oneStepLaunchInputSchema = json.RawMessage(`{"type":"object","properties":{` + commonLaunchInputProperties + `,"role":{"type":"string","description":"Single agent role to run."}},"required":["role"],"anyOf":[{"required":["task"]},{"required":["specs"]}],"additionalProperties":false}`)

var manualRosterLaunchInputSchema = json.RawMessage(`{"type":"object","properties":{` + commonLaunchInputProperties + `,"roster":{"type":"array","items":{"type":"string"},"minItems":1,"description":"Ordered agent roles to run."}},"required":["roster"],"anyOf":[{"required":["task"]},{"required":["specs"]}],"additionalProperties":false}`)

var constrainedLaunchInputSchema = json.RawMessage(`{"type":"object","properties":{` + commonLaunchInputProperties + `,"allowed_roles":{"type":"array","items":{"type":"string"},"minItems":1,"description":"Allowed roles for orchestrator-spawned steps."}},"required":["allowed_roles"],"anyOf":[{"required":["task"]},{"required":["specs"]}],"additionalProperties":false}`)

var orchestratorLaunchInputSchema = json.RawMessage(`{"type":"object","properties":{` + commonLaunchInputProperties + `},"anyOf":[{"required":["task"]},{"required":["specs"]}],"additionalProperties":false}`)
