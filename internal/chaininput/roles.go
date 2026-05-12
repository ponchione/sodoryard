package chaininput

import "strings"

func ParseRoleSet(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return NormalizeRoleSet(strings.Split(value, ","))
}

func ParseRoleList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return NormalizeRoleList(strings.Split(value, ","))
}

func NormalizeRoleSet(roles []string) []string {
	normalized := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		key := strings.ToLower(role)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, role)
	}
	return normalized
}

func NormalizeRoleList(roles []string) []string {
	normalized := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		normalized = append(normalized, role)
	}
	return normalized
}
