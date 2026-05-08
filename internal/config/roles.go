package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ponchione/sodoryard/internal/embeddedprompts"
)

// ResolveAgentRole returns the canonical config key and role config for a
// role reference. The reference may be either the configured role key or the
// built-in persona name associated with that role's prompt.
func (c *Config) ResolveAgentRole(reference string) (string, AgentRoleConfig, error) {
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return "", AgentRoleConfig{}, fmt.Errorf("agent role is required")
	}
	if c == nil || len(c.AgentRoles) == 0 {
		return "", AgentRoleConfig{}, fmt.Errorf("agent role %q not found in config", reference)
	}
	if roleCfg, ok := c.AgentRoles[trimmed]; ok {
		return trimmed, roleCfg, nil
	}

	normalized := normalizeAgentRoleReference(trimmed)
	keyMatches := map[string]AgentRoleConfig{}
	for roleName, roleCfg := range c.AgentRoles {
		if normalizeAgentRoleReference(roleName) == normalized {
			keyMatches[roleName] = roleCfg
		}
	}
	if name, roleCfg, ok, err := oneAgentRoleMatch(reference, keyMatches); ok || err != nil {
		return name, roleCfg, err
	}

	matches := map[string]AgentRoleConfig{}
	for roleName, roleCfg := range c.AgentRoles {
		for _, alias := range agentRoleAliases(roleName, roleCfg) {
			if normalizeAgentRoleReference(alias) == normalized {
				matches[roleName] = roleCfg
				break
			}
		}
	}

	if len(matches) == 0 {
		return "", AgentRoleConfig{}, fmt.Errorf("agent role %q not found in config", reference)
	}
	name, roleCfg, _, err := oneAgentRoleMatch(reference, matches)
	return name, roleCfg, err
}

func oneAgentRoleMatch(reference string, matches map[string]AgentRoleConfig) (string, AgentRoleConfig, bool, error) {
	if len(matches) == 0 {
		return "", AgentRoleConfig{}, false, nil
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for name := range matches {
			names = append(names, name)
		}
		sort.Strings(names)
		return "", AgentRoleConfig{}, true, fmt.Errorf("agent role %q is ambiguous; matches configured roles: %s", reference, strings.Join(names, ", "))
	}
	for name, roleCfg := range matches {
		return name, roleCfg, true, nil
	}
	return "", AgentRoleConfig{}, false, nil
}

func agentRoleAliases(roleName string, roleCfg AgentRoleConfig) []string {
	aliases := []string{}
	if builtinRole := builtinRoleForAgentRole(roleName, roleCfg); builtinRole != "" {
		aliases = append(aliases, embeddedprompts.PersonaAliases(builtinRole)...)
	}

	trimmedPrompt := strings.TrimSpace(roleCfg.SystemPrompt)
	if trimmedPrompt != "" && !strings.HasPrefix(trimmedPrompt, "builtin:") {
		base := strings.TrimSuffix(filepath.Base(trimmedPrompt), filepath.Ext(trimmedPrompt))
		if base != "" {
			aliases = append(aliases, base)
		}
	}
	return aliases
}

func builtinRoleForAgentRole(roleName string, roleCfg AgentRoleConfig) string {
	trimmedPrompt := strings.TrimSpace(roleCfg.SystemPrompt)
	if strings.HasPrefix(trimmedPrompt, "builtin:") {
		return strings.TrimSpace(strings.TrimPrefix(trimmedPrompt, "builtin:"))
	}
	trimmedRole := strings.TrimSpace(roleName)
	if trimmedPrompt == "" && embeddedprompts.Has(trimmedRole) {
		return trimmedRole
	}
	return ""
}

func normalizeRoleMutationClass(roleName string, roleCfg AgentRoleConfig) MutationClass {
	if trimmed := strings.TrimSpace(string(roleCfg.MutationClass)); trimmed != "" {
		return MutationClass(trimmed)
	}
	if builtinRole := builtinRoleForAgentRole(roleName, roleCfg); builtinRole != "" {
		if mutationClass := builtinRoleMutationClass(builtinRole); mutationClass != "" {
			return mutationClass
		}
	}
	if mutationClass := builtinRoleMutationClass(roleName); mutationClass != "" {
		return mutationClass
	}
	return inferMutationClassFromTools(roleCfg)
}

func builtinRoleMutationClass(roleName string) MutationClass {
	switch strings.TrimSpace(roleName) {
	case "orchestrator":
		return MutationClassOrchestrator
	case "coder", "resolver", "test-writer":
		return MutationClassSourceWrite
	case "correctness-auditor", "quality-auditor", "performance-auditor", "security-auditor", "integration-auditor":
		return MutationClassReadOnly
	case "planner", "epic-decomposer", "task-decomposer", "docs-arbiter":
		return MutationClassBrainWrite
	default:
		return ""
	}
}

func inferMutationClassFromTools(roleCfg AgentRoleConfig) MutationClass {
	for _, group := range roleCfg.Tools {
		switch strings.TrimSpace(group) {
		case "file", "shell", "test", "sqlc":
			return MutationClassSourceWrite
		}
	}
	if len(roleCfg.BrainWritePaths) > 0 {
		return MutationClassBrainWrite
	}
	for _, group := range roleCfg.Tools {
		if strings.TrimSpace(group) == "brain" {
			return MutationClassBrainWrite
		}
	}
	return MutationClassReadOnly
}

func IsSourceWritingRole(roleName string, roleCfg AgentRoleConfig) bool {
	return normalizeRoleMutationClass(roleName, roleCfg) == MutationClassSourceWrite
}

func normalizeAgentRoleReference(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, lower)
}
