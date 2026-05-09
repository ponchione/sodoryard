package tui

import (
	"testing"

	"github.com/ponchione/sodoryard/internal/operator"
)

func TestSlashLaunchRequestAcceptsTemplateID(t *testing.T) {
	req, err := slashLaunchRequest(slashCommand{
		Name: "preview",
		Flags: map[string][]string{
			"template": []string{"one_step"},
			"role":     []string{"coder"},
			"task":     []string{"fix tests"},
		},
	})
	if err != nil {
		t.Fatalf("slashLaunchRequest returned error: %v", err)
	}
	if req.TemplateID != "one_step" || req.Role != "coder" || req.SourceTask != "fix tests" {
		t.Fatalf("request = %+v, want template one-step request", req)
	}
}

func TestSlashLaunchRequestKeepsModeForCompatibility(t *testing.T) {
	req, err := slashLaunchRequest(slashCommand{
		Name: "preview",
		Flags: map[string][]string{
			"mode": []string{"manual_roster"},
			"task": []string{"ship roster"},
		},
	})
	if err != nil {
		t.Fatalf("slashLaunchRequest returned error: %v", err)
	}
	if req.Mode != operator.LaunchModeManualRoster || req.TemplateID != "" {
		t.Fatalf("request = %+v, want manual roster mode without template", req)
	}
}

func TestSlashLaunchRequestAcceptsApprovalWait(t *testing.T) {
	req, err := slashLaunchRequest(slashCommand{
		Name: "start",
		Flags: map[string][]string{
			"role":                {"coder"},
			"task":                {"risky work"},
			"allow-approval-wait": {"true"},
		},
	})
	if err != nil {
		t.Fatalf("slashLaunchRequest returned error: %v", err)
	}
	if !req.AllowApprovalWait {
		t.Fatalf("AllowApprovalWait = false, want true")
	}
}
