package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// PermissionScope groups actions for Vedal's capability management UI.
type PermissionScope string

const (
	ScopeInput      PermissionScope = "input"
	ScopeFilesystem PermissionScope = "filesystem"
	ScopeProcess    PermissionScope = "process"
	ScopeNetwork    PermissionScope = "network"
	ScopeSystem     PermissionScope = "system"
	ScopeVision     PermissionScope = "vision"
	// ScopeGame covers the high-level game interface (playing a game that has
	// no dedicated integration).
	ScopeGame PermissionScope = "game"
	// ScopeShell covers running command lines (headless machines, terminal work).
	ScopeShell PermissionScope = "shell"
)

// ScopeLimits are the per-scope knobs the dashboard exposes. Only the rate
// limit is enforced today; it is persisted so a policy survives a round trip
// through the UI unchanged.
type ScopeLimits struct {
	MaxActionsPerMinute int `json:"max_actions_per_minute,omitempty"`
}

type ScopeConfig struct {
	Allowed bool        `json:"allowed"`
	Limits  ScopeLimits `json:"limits,omitempty"`
}

// UnmarshalJSON accepts both the documented shape ({"allowed": true}) and the
// shorthand people reach for when hand-editing a policy ("game": true).
func (s *ScopeConfig) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "true" {
		s.Allowed = true
		return nil
	}
	if trimmed == "false" {
		s.Allowed = false
		return nil
	}

	var raw struct {
		Allowed *flexibleBool `json:"allowed"`
		Limits  ScopeLimits   `json:"limits"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("a scope must be true/false or {\"allowed\": true/false}")
	}
	if raw.Allowed != nil {
		s.Allowed = bool(*raw.Allowed)
	}
	if raw.Limits.MaxActionsPerMinute < 0 {
		return fmt.Errorf("max_actions_per_minute must not be negative")
	}
	s.Limits = raw.Limits
	return nil
}

// flexibleBool accepts true/false and the string spellings people use in configs.
type flexibleBool bool

func (b *flexibleBool) UnmarshalJSON(data []byte) error {
	var asBool bool
	if err := json.Unmarshal(data, &asBool); err == nil {
		*b = flexibleBool(asBool)
		return nil
	}

	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		switch strings.ToLower(strings.TrimSpace(asString)) {
		case "true", "yes", "1", "on", "allow", "allowed":
			*b = true
			return nil
		case "false", "no", "0", "off", "deny", "denied":
			*b = false
			return nil
		}
	}

	return fmt.Errorf("expected true/false (or a boolean-like string)")
}

type PermissionPolicy struct {
	DefaultAllow bool
	allowed      map[string]struct{}
	denied       map[string]struct{}
	scopes       map[PermissionScope]ScopeConfig
	// hardDeny comes from NEURO_DENY_ACTIONS and outranks both lists: it is the
	// operator's "not even if the dashboard says so".
	hardDeny map[string]bool
}

type permissionPolicyFile struct {
	// Version is accepted but never parsed: it is documentation for humans, and
	// a policy using "1" as well as "1.0.0" must both load.
	Version        json.RawMessage                 `json:"version"`
	DefaultAllow   *flexibleBool                   `json:"default_allow"`
	AllowedActions []string                        `json:"allowed_actions"`
	DeniedActions  []string                        `json:"denied_actions"`
	Scopes         map[PermissionScope]ScopeConfig `json:"scopes"`
}

// actionScope maps Neuro action names to capability scopes.
var actionScope = map[string]PermissionScope{
	string(CmdMouseMove):           ScopeInput,
	string(CmdMouseClick):          ScopeInput,
	string(CmdKeyPress):            ScopeInput,
	string(CmdTypeText):            ScopeInput,
	string(CmdRunScript):           ScopeInput,
	string(CmdExecuteQueue):        ScopeInput,
	string(CmdClearActionQueue):    ScopeInput,
	string(EnableLLControls):       ScopeInput,
	string(DisableLLControls):      ScopeInput,
	string(CmdOpenWindowsMenu):     ScopeInput,
	string(CmdShowDesktop):         ScopeInput,
	string(CmdMinimizeAll):         ScopeInput,
	string(CmdCloseForeground):     ScopeInput,
	string(CmdOpenTaskManager):     ScopeProcess,
	string(CmdCloseAllApps):        ScopeProcess,
	string(CmdOpenExplorer):        ScopeFilesystem,
	string(CmdOpenRunDialog):       ScopeProcess,
	string(CmdOpenSearch):          ScopeInput,
	string(CmdSnapWindowLeft):      ScopeInput,
	string(CmdSnapWindowRight):     ScopeInput,
	string(CmdOpenSettings):        ScopeSystem,
	string(CmdOpenNotification):    ScopeInput,
	string(CmdOpenClipboard):       ScopeInput,
	string(CmdLockWorkstation):     ScopeSystem,
	string(CmdSwitchAppNext):       ScopeInput,
	string(CmdSwitchAppPrevious):   ScopeInput,
	string(CmdOpenPowerUserMenu):   ScopeSystem,
	string(CmdTakeScreenSnip):      ScopeVision,
	string(CmdListCatalogItems):    ScopeNetwork,
	string(CmdFindCatalogItems):    ScopeNetwork,
	string(CmdGetCatalogItem):      ScopeNetwork,
	string(CmdGetDesktopContext):   ScopeVision,
	string(CmdSendDesktopContext):  ScopeVision,
	string(CmdListInstalledExts):   ScopeFilesystem,
	string(CmdInstallExtension):    ScopeFilesystem,
	string(CmdUninstallExtension):  ScopeFilesystem,
	string(CmdEnableExtension):     ScopeFilesystem,
	string(CmdDisableExtension):    ScopeFilesystem,
	string(CmdGetStatus):           ScopeVision,
	string(CmdShutdownGracefully):  ScopeSystem,
	string(CmdShutdownImmediately): ScopeSystem,

	// The guide is read-only information, so it rides the vision scope (on by
	// default) — a weak model must always be able to ask how to use this thing.
	string(CmdDesktopGuide): ScopeVision,

	// Shell: denied in the shipped example policy, and additionally fenced by
	// the shell allowlist/denylist firewall.
	string(CmdShellCommand): ScopeShell,

	// Game interface. Observing is read-only (vision); driving the game is
	// input; launching a game is system (denied by default).
	string(CmdGameListProfiles): ScopeGame,
	string(CmdGameDetect):       ScopeVision,
	string(CmdGameStartSession): ScopeGame,
	string(CmdGameEndSession):   ScopeGame,
	string(CmdGameStatus):       ScopeVision,
	string(CmdGameMove):         ScopeGame,
	string(CmdGameLook):         ScopeGame,
	string(CmdGameAction):       ScopeGame,
	string(CmdGamePress):        ScopeGame,
	string(CmdGameReleaseAll):   ScopeGame,
	string(CmdGameObserve):      ScopeVision,
	string(CmdGameLaunch):       ScopeSystem,
}

func defaultPermissionPolicy() *PermissionPolicy {
	// Safer default: deny unless allowed. Missing policy file still uses this
	// for production-minded installs; set default_allow true explicitly for labs.
	return &PermissionPolicy{
		DefaultAllow: false,
		allowed:      map[string]struct{}{},
		denied:       map[string]struct{}{},
		scopes: map[PermissionScope]ScopeConfig{
			ScopeInput:      {Allowed: true},
			ScopeFilesystem: {Allowed: false},
			ScopeProcess:    {Allowed: true},
			ScopeNetwork:    {Allowed: true},
			ScopeSystem:     {Allowed: false},
			ScopeVision:     {Allowed: true},
			ScopeGame:       {Allowed: true},
			// A default install must not hand out a shell.
			ScopeShell: {Allowed: false},
		},
	}
}

func loadPermissionPolicy(path string) (*PermissionPolicy, error) {
	if path == "" {
		policy := defaultPermissionPolicy()
		policy.applyHardDeny(hardDeniedActions())
		return policy, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			policy := defaultPermissionPolicy()
			policy.applyHardDeny(hardDeniedActions())
			return policy, nil
		}
		return nil, fmt.Errorf("failed to read permissions file %s: %w", path, err)
	}

	var parsed permissionPolicyFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse permissions file %s: %w", path, err)
	}

	policy := defaultPermissionPolicy()
	policy.applyHardDeny(hardDeniedActions())
	if parsed.DefaultAllow != nil {
		policy.DefaultAllow = bool(*parsed.DefaultAllow)
	}

	for _, action := range parsed.AllowedActions {
		policy.allowed[strings.TrimSpace(action)] = struct{}{}
	}
	for _, action := range parsed.DeniedActions {
		policy.denied[strings.TrimSpace(action)] = struct{}{}
	}
	for scope, cfg := range parsed.Scopes {
		if !knownScope(scope) {
			// A typo ("games" instead of "game") would otherwise silently grant
			// or drop a capability; say so instead.
			fmt.Fprintf(os.Stderr, "[warn] permissions file %s: unknown scope %q ignored\n", path, scope)
			continue
		}
		policy.scopes[scope] = cfg
	}

	return policy, nil
}

// ScopeAllowed answers the scope question directly, which is what script-level
// checks need (a script is one action but may contain several capabilities).
func (p *PermissionPolicy) ScopeAllowed(scope PermissionScope) bool {
	if p == nil {
		return false
	}
	if cfg, ok := p.scopes[scope]; ok {
		return cfg.Allowed
	}
	return p.DefaultAllow
}

// ScopeRateLimit returns the scope's actions-per-minute budget, or 0 when the
// scope is unlimited.
func (p *PermissionPolicy) ScopeRateLimit(scope PermissionScope) int {
	if p == nil || scope == "" {
		return 0
	}
	if cfg, ok := p.scopes[scope]; ok {
		return cfg.Limits.MaxActionsPerMinute
	}
	return 0
}

// ScopeConfigs exposes the per-scope configuration for the dashboard.
func (p *PermissionPolicy) ScopeConfigs() map[PermissionScope]ScopeConfig {
	out := map[PermissionScope]ScopeConfig{}
	if p == nil {
		return out
	}
	for _, scope := range []PermissionScope{
		ScopeInput, ScopeGame, ScopeShell, ScopeFilesystem, ScopeProcess, ScopeNetwork, ScopeSystem, ScopeVision,
	} {
		out[scope] = p.scopes[scope]
	}
	return out
}

// scopeRequiresExplicitConsent marks the scopes that must be enabled by name.
// shell and system can wipe a machine or run arbitrary programs, so a policy
// that merely lists the action (or sets default_allow) does not enable them.
func scopeRequiresExplicitConsent(scope PermissionScope) bool {
	switch scope {
	case ScopeShell, ScopeSystem:
		return true
	}
	return false
}

func knownScope(scope PermissionScope) bool {
	switch scope {
	case ScopeInput, ScopeGame, ScopeShell, ScopeFilesystem, ScopeProcess, ScopeNetwork, ScopeSystem, ScopeVision:
		return true
	}
	return false
}

func (p *PermissionPolicy) IsAllowed(action string) bool {
	// The hard deny list outranks everything, including an explicit allow.
	if p.hardDenied(action) {
		return false
	}

	// Deny list always wins.
	if _, denied := p.denied[action]; denied {
		return false
	}

	// The dangerous scopes are checked before the explicit allow list: a bare
	// allow-list entry (or default_allow) must not hand out a shell or system
	// control. The operator has to turn those scopes on deliberately.
	if scope, ok := actionScope[action]; ok && scopeRequiresExplicitConsent(scope) {
		if cfg, ok := p.scopes[scope]; ok {
			return cfg.Allowed
		}
		return false
	}

	// Explicit allow list wins next.
	if _, allowed := p.allowed[action]; allowed {
		return true
	}

	// Scope gates: Vedal toggles capability categories in the management UI.
	if scope, ok := actionScope[action]; ok {
		if cfg, ok := p.scopes[scope]; ok {
			return cfg.Allowed
		}
	}

	return p.DefaultAllow
}
