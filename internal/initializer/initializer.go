package initializer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

// Options configure a single initializer.Run() call.
type Options struct {
	// ProjectRoot is the absolute path to the directory being initialized.
	// Required.
	ProjectRoot string

	// ConfigFilename overrides the generated config filename. If empty,
	// the canonical "yard.yaml" is used. Provided as an escape hatch for
	// tests and unusual operator setups.
	ConfigFilename string
}

// Report describes what Run() did. Each entry is one operator-visible
// action — created, skipped, or modified.
type Report struct {
	Entries []ReportEntry
}

// ReportEntry is one line of init output.
type ReportEntry struct {
	Kind    string // "config", "mkdir", "gitignore"
	Path    string // operator-relative path (relative to ProjectRoot)
	Status  string // "created", "skipped", "added <details>"
	Details string // optional extra information for "added" entries
}

// Run bootstraps a project for railway use. It is safe to re-run against
// an already-initialized project — every step is idempotent and
// existing files are preserved.
//
// Run does not change the process working directory.
func Run(ctx context.Context, opts Options) (*Report, error) {
	if strings.TrimSpace(opts.ProjectRoot) == "" {
		return nil, fmt.Errorf("initializer: ProjectRoot is required")
	}
	projectRoot := opts.ProjectRoot
	configFilename := opts.ConfigFilename
	if configFilename == "" {
		configFilename = appconfig.ConfigFilename
	}
	projectName := filepath.Base(projectRoot)

	report := &Report{}

	// 1. Generate yard.yaml from the embedded template.
	configEntry, err := writeConfigFile(projectRoot, projectName, configFilename)
	if err != nil {
		return nil, err
	}
	report.Entries = append(report.Entries, configEntry)

	// 2. mkdir state dir.
	if entry, err := mkdirRelative(projectRoot, appconfig.StateDirName, "mkdir"); err != nil {
		return nil, err
	} else {
		report.Entries = append(report.Entries, entry)
	}

	// 3. mkdir derived and runtime state directories under .yard/.
	for _, sub := range []string{
		filepath.Join("lancedb", "code"),
		filepath.Join("lancedb", "brain"),
		filepath.Join("shunter", "project-memory"),
		"run",
	} {
		if entry, err := mkdirRelative(projectRoot, filepath.Join(appconfig.StateDirName, sub), "mkdir"); err != nil {
			return nil, err
		} else {
			report.Entries = append(report.Entries, entry)
		}
	}

	// 4. Patch .gitignore.
	added, err := EnsureGitignoreEntries(projectRoot)
	if err != nil {
		return nil, err
	}
	gitignoreStatus := "already has entries, skipped"
	gitignoreDetails := ""
	if len(added) > 0 {
		gitignoreStatus = "added"
		gitignoreDetails = strings.Join(added, ", ")
	}
	report.Entries = append(report.Entries, ReportEntry{
		Kind:    "gitignore",
		Path:    ".gitignore",
		Status:  gitignoreStatus,
		Details: gitignoreDetails,
	})

	return report, nil
}

// writeConfigFile renders the embedded yard.yaml template into the project
// root, performing the two known substitutions. Returns a ReportEntry that
// describes what happened.
func writeConfigFile(projectRoot, projectName, configFilename string) (ReportEntry, error) {
	configPath := filepath.Join(projectRoot, configFilename)
	if _, err := os.Stat(configPath); err == nil {
		return ReportEntry{Kind: "config", Path: configFilename, Status: "already exists, skipped"}, nil
	}

	raw, err := readEmbeddedFile(yardYamlTemplatePath)
	if err != nil {
		return ReportEntry{}, err
	}
	rendered := substituteTemplate(string(raw), SubstitutionValues{
		ProjectRoot: projectRoot,
		ProjectName: projectName,
	})
	if err := os.WriteFile(configPath, []byte(rendered), 0o644); err != nil {
		return ReportEntry{}, fmt.Errorf("write %s: %w", configPath, err)
	}
	return ReportEntry{Kind: "config", Path: configFilename, Status: "created"}, nil
}

// mkdirRelative creates the given subpath under projectRoot, recording
// whether the directory was newly created or already existed. Used by
// Run() for every directory it makes.
func mkdirRelative(projectRoot, rel, kind string) (ReportEntry, error) {
	full := filepath.Join(projectRoot, rel)
	if info, err := os.Stat(full); err == nil && info.IsDir() {
		return ReportEntry{Kind: kind, Path: rel, Status: "already exists"}, nil
	}
	if err := os.MkdirAll(full, 0o755); err != nil {
		return ReportEntry{}, fmt.Errorf("create %s: %w", rel, err)
	}
	return ReportEntry{Kind: kind, Path: rel, Status: "created"}, nil
}
