package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type PermissionPolicy struct {
	DefaultAllow bool
	allowed      map[string]struct{}
	denied       map[string]struct{}
}

type permissionPolicyFile struct {
	DefaultAllow   *bool    `json:"default_allow"`
	AllowedActions []string `json:"allowed_actions"`
	DeniedActions  []string `json:"denied_actions"`
}

func defaultPermissionPolicy() *PermissionPolicy {
	return &PermissionPolicy{
		DefaultAllow: true,
		allowed:      map[string]struct{}{},
		denied:       map[string]struct{}{},
	}
}

func loadPermissionPolicy(path string) (*PermissionPolicy, error) {
	if path == "" {
		return defaultPermissionPolicy(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultPermissionPolicy(), nil
		}
		return nil, fmt.Errorf("failed to read permissions file %s: %w", path, err)
	}

	var parsed permissionPolicyFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse permissions file %s: %w", path, err)
	}

	policy := defaultPermissionPolicy()
	if parsed.DefaultAllow != nil {
		policy.DefaultAllow = *parsed.DefaultAllow
	}

	for _, action := range parsed.AllowedActions {
		policy.allowed[action] = struct{}{}
	}
	for _, action := range parsed.DeniedActions {
		policy.denied[action] = struct{}{}
	}

	return policy, nil
}

func (p *PermissionPolicy) IsAllowed(action string) bool {
	if _, denied := p.denied[action]; denied {
		return false
	}

	if _, allowed := p.allowed[action]; allowed {
		return true
	}

	return p.DefaultAllow
}
