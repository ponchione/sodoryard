package initializer

import (
	"strings"
	"testing"
)

func TestSubstituteReplacesProjectRootAndName(t *testing.T) {
	in := "project_root: {{PROJECT_ROOT}}\nname: {{PROJECT_NAME}}\n"
	out := substituteTemplate(in, SubstitutionValues{
		ProjectRoot: "/home/user/myapp",
		ProjectName: "myapp",
	})
	if !strings.Contains(out, "project_root: /home/user/myapp\n") {
		t.Errorf("PROJECT_ROOT not substituted: %s", out)
	}
	if !strings.Contains(out, "name: myapp\n") {
		t.Errorf("PROJECT_NAME not substituted: %s", out)
	}
}

func TestSubstituteLeavesOtherPlaceholdersAlone(t *testing.T) {
	in := "system_prompt: {{CUSTOM_PROMPT_DIR}}/coder.md\n"
	out := substituteTemplate(in, SubstitutionValues{
		ProjectRoot: "/home/user/myapp",
		ProjectName: "myapp",
	})
	if !strings.Contains(out, "{{CUSTOM_PROMPT_DIR}}/coder.md") {
		t.Errorf("expected {{CUSTOM_PROMPT_DIR}} placeholder to be preserved, got: %s", out)
	}
}

func TestSubstituteIsIdempotent(t *testing.T) {
	in := "project_root: {{PROJECT_ROOT}}\n"
	values := SubstitutionValues{ProjectRoot: "/x/y", ProjectName: "y"}
	once := substituteTemplate(in, values)
	twice := substituteTemplate(once, values)
	if once != twice {
		t.Errorf("substitution not idempotent: once=%q twice=%q", once, twice)
	}
}

func TestSubstituteReplacesMultipleOccurrences(t *testing.T) {
	in := "{{PROJECT_ROOT}}/foo {{PROJECT_ROOT}}/bar"
	out := substituteTemplate(in, SubstitutionValues{ProjectRoot: "/p", ProjectName: "p"})
	if out != "/p/foo /p/bar" {
		t.Errorf("expected both occurrences substituted, got: %q", out)
	}
}
