package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPermissionPolicyMissingFileDefaultsToAllow(t *testing.T) {
	tempDir := t.TempDir()
	missingPath := filepath.Join(tempDir, "missing-permissions.json")

	policy, err := loadPermissionPolicy(missingPath)
	if err != nil {
		t.Fatalf("expected missing file to default, got error: %v", err)
	}

	if !policy.IsAllowed(string(CmdRunScript)) {
		t.Fatal("expected default policy to allow actions")
	}
}

func TestLoadPermissionPolicyRespectsAllowDenyRules(t *testing.T) {
	tempDir := t.TempDir()
	policyPath := filepath.Join(tempDir, "permissions.json")

	content := `{
  "default_allow": false,
  "allowed_actions": ["run_script", "execute_queue"],
  "denied_actions": ["shutdown_immediately"]
}`

	if err := os.WriteFile(policyPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write policy fixture: %v", err)
	}

	policy, err := loadPermissionPolicy(policyPath)
	if err != nil {
		t.Fatalf("failed to load policy: %v", err)
	}

	if !policy.IsAllowed("run_script") {
		t.Fatal("expected explicitly allowed action to be permitted")
	}

	if policy.IsAllowed("shutdown_immediately") {
		t.Fatal("expected explicitly denied action to be blocked")
	}

	if policy.IsAllowed("mouse_click") {
		t.Fatal("expected unspecified action to follow default_allow=false")
	}
}
