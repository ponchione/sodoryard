package localservices

import (
	"context"
	"fmt"
	"sort"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args []string, dir string) (stdout string, stderr string, err error)
}

type ServiceStatus struct {
	Name        string `json:"name"`
	Healthy     bool   `json:"healthy"`
	Reachable   bool   `json:"reachable"`
	ModelsReady bool   `json:"models_ready"`
	Required    bool   `json:"required"`
	BaseURL     string `json:"base_url"`
	HealthURL   string `json:"health_url"`
	ModelsURL   string `json:"models_url"`
	Detail      string `json:"detail,omitempty"`
}

type StackStatus struct {
	Mode              string          `json:"mode"`
	ComposeFile       string          `json:"compose_file"`
	ProjectDir        string          `json:"project_dir"`
	DockerAvailable   bool            `json:"docker_available"`
	DaemonAvailable   bool            `json:"daemon_available"`
	ComposeAvailable  bool            `json:"compose_available"`
	ComposeFileExists bool            `json:"compose_file_exists"`
	NetworkStatus     map[string]bool `json:"network_status"`
	Services          []ServiceStatus `json:"services"`
	RequiredServices  []string        `json:"required_services,omitempty"`
	Problems          []string        `json:"problems,omitempty"`
	Remediation       []string        `json:"remediation,omitempty"`
}

func (s StackStatus) AllRequiredHealthy() bool {
	for _, service := range s.Services {
		if service.Required && !service.Healthy {
			return false
		}
	}
	return true
}

type ManagerError struct {
	Op     string
	Status StackStatus
}

func (e *ManagerError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Status.Problems) == 0 {
		return fmt.Sprintf("local services %s failed", e.Op)
	}
	return fmt.Sprintf("local services %s failed: %s", e.Op, e.Status.Problems[0])
}

type sleepFunc func(time.Duration)

type Manager struct {
	runner CommandRunner
	client HealthHTTPClient
	sleep  sleepFunc
}

func NewManager(runner CommandRunner) *Manager {
	if runner == nil {
		runner = shellRunner{}
	}
	return &Manager{
		runner: runner,
		client: defaultHTTPClient(),
		sleep:  time.Sleep,
	}
}

func NewManagerWithDeps(runner CommandRunner, client HealthHTTPClient, sleep sleepFunc) *Manager {
	if runner == nil {
		runner = shellRunner{}
	}
	if client == nil {
		client = defaultHTTPClient()
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	return &Manager{runner: runner, client: client, sleep: sleep}
}

func ConfiguredServiceNames(cfg appconfig.LocalServicesConfig) []string {
	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
