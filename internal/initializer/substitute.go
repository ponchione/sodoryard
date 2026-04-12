package initializer

import "strings"

// SubstitutionValues holds the values that get substituted into a template
// at copy time. Two fields, two corresponding {{PLACEHOLDERS}}.
type SubstitutionValues struct {
	// ProjectRoot is the absolute path to the project being initialized.
	// Substituted for {{PROJECT_ROOT}}.
	ProjectRoot string

	// ProjectName is the basename of ProjectRoot. Used in the yaml header
	// comment. Substituted for {{PROJECT_NAME}}.
	ProjectName string
}

// substituteTemplate performs exact-string replacement of the two known
// placeholders in s. Other {{...}} tokens are left untouched.
func substituteTemplate(s string, v SubstitutionValues) string {
	s = strings.ReplaceAll(s, "{{PROJECT_ROOT}}", v.ProjectRoot)
	s = strings.ReplaceAll(s, "{{PROJECT_NAME}}", v.ProjectName)
	return s
}
