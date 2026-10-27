// Package forkfeatures owns the personal-fork availability profile. It is
// deliberately independent of commercial licensing and action authority.
package forkfeatures

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const ProfileVersion = 2

type FeatureID string

const (
	FeatureCodexLBGateway      FeatureID = "fork.codex_lb_gateway"
	FeatureKomodoReadOnly      FeatureID = "fork.komodo_read_only"
	FeatureDeterministicTriage FeatureID = "fork.deterministic_triage"
	FeatureAIInvestigation     FeatureID = "fork.ai_investigation"
	FeatureGovernedActions     FeatureID = "fork.governed_actions"
	FeatureAgentProfiles       FeatureID = "fork.agent_profiles"
	FeatureRBAC                FeatureID = "fork.rbac"
	FeatureAuditLogging        FeatureID = "fork.audit_logging"
	FeatureAdvancedReporting   FeatureID = "fork.advanced_reporting"
	FeatureExternalProbe       FeatureID = "fork.external_probe"
	FeatureActionSafety        FeatureID = "fork.action_safety"
	FeatureOperationalCases    FeatureID = "fork.operational_cases"
	FeatureLongTermMetrics     FeatureID = "fork.long_term_metrics"
	FeatureDiscordOps          FeatureID = "fork.discord_ops"
)

type FeatureDefinition struct {
	ID             FeatureID `json:"id"`
	Implemented    bool      `json:"implemented"`
	Status         Status    `json:"status"`
	Implementation string    `json:"implementation,omitempty"`
}

// Status describes implementation maturity independently from whether an
// operator enabled the capability in the local profile.
type Status string

const (
	StatusSupported    Status = "supported"
	StatusExperimental Status = "experimental"
	StatusUnavailable  Status = "unavailable"
)

var registeredFeatures = []FeatureDefinition{
	{ID: FeatureCodexLBGateway, Status: StatusExperimental, Implementation: "OpenAI-compatible Codex-LB provider enforcement"},
	{ID: FeatureKomodoReadOnly, Implemented: true, Status: StatusSupported, Implementation: "read-only Komodo monitoring adapter"},
	{ID: FeatureDeterministicTriage, Implemented: true, Status: StatusSupported, Implementation: "deterministic alert triage extensions"},
	{ID: FeatureAIInvestigation, Implemented: true, Status: StatusSupported, Implementation: "Pulse chat investigation runner and findings store"},
	{ID: FeatureGovernedActions, Implemented: true, Status: StatusSupported, Implementation: "Pulse typed action planner and canonical action lifecycle"},
	{ID: FeatureAgentProfiles, Implemented: true, Status: StatusSupported, Implementation: "public agent profile manager and versioned configuration"},
	{ID: FeatureRBAC, Implemented: true, Status: StatusSupported, Implementation: "organization-scoped roles and permission checks"},
	{ID: FeatureAuditLogging, Implemented: true, Status: StatusSupported, Implementation: "local append-only audit records and read APIs"},
	{ID: FeatureAdvancedReporting, Implemented: true, Status: StatusSupported, Implementation: "local report generation and scheduling"},
	{ID: FeatureExternalProbe, Implemented: true, Status: StatusSupported, Implementation: "operator-configured external availability probes"},
	{ID: FeatureActionSafety, Status: StatusExperimental, Implementation: "strict remediation policy and safety controls"},
	{ID: FeatureOperationalCases, Status: StatusExperimental, Implementation: "multi-alert operational cases and repair plans"},
	{ID: FeatureLongTermMetrics, Status: StatusUnavailable, Implementation: "extended Pulse metric retention tiers"},
	{ID: FeatureDiscordOps, Status: StatusExperimental, Implementation: "Discord conversational operations"},
}

type ForkProfile struct {
	Version         int         `json:"version"`
	Revision        uint64      `json:"revision"`
	EnabledFeatures []FeatureID `json:"enabled_features"`
}

func DefaultProfile() ForkProfile {
	return ForkProfile{Version: ProfileVersion, Revision: 1, EnabledFeatures: []FeatureID{
		FeatureAIInvestigation,
		FeatureDeterministicTriage,
		FeatureGovernedActions,
		FeatureKomodoReadOnly,
	}}
}

type Capability struct {
	ID          FeatureID `json:"id"`
	Available   bool      `json:"available"`
	Implemented bool      `json:"implemented"`
	Enabled     bool      `json:"enabled"`
	Status      Status    `json:"status"`
	Source      string    `json:"source"`
}

type Snapshot struct {
	ProfileVersion int          `json:"profile_version"`
	Revision       uint64       `json:"revision"`
	Capabilities   []Capability `json:"capabilities"`
}

func (s Snapshot) Has(id FeatureID) bool {
	for _, capability := range s.Capabilities {
		if capability.ID == id {
			return capability.Available
		}
	}
	return false
}

type ProfileStore interface {
	LoadForkProfile() (*ForkProfile, error)
	SaveForkProfile(ForkProfile) error
}

var (
	ErrStaleRevision  = errors.New("fork profile revision is stale")
	ErrUnavailable    = errors.New("fork capability is unavailable")
	ErrInvalidProfile = errors.New("invalid fork profile")
)

type Manager struct {
	mu      sync.RWMutex
	store   ProfileStore
	profile ForkProfile
	loadErr error
}

func NewManager(store ProfileStore) *Manager {
	manager := &Manager{store: store, profile: DefaultProfile()}
	if store == nil {
		return manager
	}
	profile, err := store.LoadForkProfile()
	if err != nil {
		manager.loadErr = fmt.Errorf("load fork profile: %w", err)
		return manager
	}
	if profile == nil {
		return manager
	}
	migratedProfile := migrateProfile(*profile)
	if err := validateProfile(migratedProfile); err != nil {
		manager.loadErr = fmt.Errorf("validate persisted fork profile: %w", err)
		return manager
	}
	manager.profile = cloneProfile(migratedProfile)
	return manager
}

func RegisteredFeatures() []FeatureDefinition {
	features := append([]FeatureDefinition(nil), registeredFeatures...)
	sort.Slice(features, func(i, j int) bool { return features[i].ID < features[j].ID })
	return features
}

func (m *Manager) Snapshot() (Snapshot, error) {
	if m == nil {
		return Snapshot{}, errors.New("fork profile manager is unavailable")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.loadErr != nil {
		return Snapshot{}, m.loadErr
	}
	enabled := make(map[FeatureID]bool, len(m.profile.EnabledFeatures))
	for _, id := range m.profile.EnabledFeatures {
		enabled[id] = true
	}
	snapshot := Snapshot{ProfileVersion: m.profile.Version, Revision: m.profile.Revision}
	for _, feature := range RegisteredFeatures() {
		isEnabled := enabled[feature.ID]
		available := feature.Implemented && isEnabled
		source := "unavailable"
		if available {
			source = "fork_public"
		}
		snapshot.Capabilities = append(snapshot.Capabilities, Capability{
			ID: feature.ID, Available: available, Implemented: feature.Implemented, Enabled: isEnabled, Status: feature.Status, Source: source,
		})
	}
	return snapshot, nil
}

func (m *Manager) Available(id FeatureID) (bool, error) {
	snapshot, err := m.Snapshot()
	if err != nil {
		return false, err
	}
	if !registered(id) {
		return false, fmt.Errorf("unknown fork capability %q", id)
	}
	return snapshot.Has(id), nil
}

func (m *Manager) Require(id FeatureID) error {
	available, err := m.Available(id)
	if err != nil {
		return err
	}
	if !available {
		return fmt.Errorf("%w: %s", ErrUnavailable, id)
	}
	return nil
}

// Replace validates the whole profile and writes it before publishing the new
// in-memory snapshot. expectedRevision is the revision the caller observed.
func (m *Manager) Replace(profile ForkProfile, expectedRevision uint64) (Snapshot, error) {
	if m == nil {
		return Snapshot{}, errors.New("fork profile manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadErr == nil && expectedRevision != m.profile.Revision {
		return Snapshot{}, ErrStaleRevision
	}
	if m.loadErr != nil && expectedRevision != 0 {
		return Snapshot{}, ErrStaleRevision
	}
	profile.Revision = expectedRevision + 1
	profile = migrateProfile(profile)
	if err := validateProfile(profile); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalidProfile, err)
	}
	if m.store != nil {
		if err := m.store.SaveForkProfile(profile); err != nil {
			return Snapshot{}, fmt.Errorf("save fork profile: %w", err)
		}
	}
	m.profile = cloneProfile(profile)
	m.loadErr = nil
	return m.snapshotLocked(), nil
}

func (m *Manager) snapshotLocked() Snapshot {
	enabled := make(map[FeatureID]bool, len(m.profile.EnabledFeatures))
	for _, id := range m.profile.EnabledFeatures {
		enabled[id] = true
	}
	snapshot := Snapshot{ProfileVersion: m.profile.Version, Revision: m.profile.Revision}
	for _, feature := range RegisteredFeatures() {
		isEnabled := enabled[feature.ID]
		available := feature.Implemented && isEnabled
		source := "unavailable"
		if available {
			source = "fork_public"
		}
		snapshot.Capabilities = append(snapshot.Capabilities, Capability{ID: feature.ID, Available: available, Implemented: feature.Implemented, Enabled: isEnabled, Status: feature.Status, Source: source})
	}
	return snapshot
}

func validateProfile(profile ForkProfile) error {
	if profile.Version != ProfileVersion {
		return fmt.Errorf("unsupported fork profile version %d", profile.Version)
	}
	if profile.Revision == 0 {
		return errors.New("fork profile revision must be positive")
	}
	seen := make(map[FeatureID]struct{}, len(profile.EnabledFeatures))
	for _, rawID := range profile.EnabledFeatures {
		id := FeatureID(strings.TrimSpace(string(rawID)))
		if id == "" {
			return errors.New("fork profile contains an empty feature identifier")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("fork profile contains duplicate feature %q", id)
		}
		seen[id] = struct{}{}
		feature, ok := featureDefinition(id)
		if !ok {
			return fmt.Errorf("unknown fork capability %q", id)
		}
		if !feature.Implemented || feature.Status != StatusSupported {
			return fmt.Errorf("fork capability %q has no complete implementation", id)
		}
	}
	return nil
}

// migrateProfile upgrades the persisted pre-policy-version profile while
// preserving its explicit feature choices. A missing profile uses new defaults;
// an explicitly empty v1 profile remains disabled after migration.
func migrateProfile(profile ForkProfile) ForkProfile {
	if profile.Version == 1 {
		profile.Version = ProfileVersion
	}
	return profile
}

func featureDefinition(id FeatureID) (FeatureDefinition, bool) {
	for _, feature := range registeredFeatures {
		if feature.ID == id {
			return feature, true
		}
	}
	return FeatureDefinition{}, false
}

func registered(id FeatureID) bool {
	_, ok := featureDefinition(id)
	return ok
}

// LocalCapabilityForLicenseFeature maps implemented public standalone surfaces
// to their explicit fork-policy capability. Composite or upstream-dependent
// entitlements are deliberately omitted: ai_autofix includes automation,
// relay depends on Pulse services, and hosted/multi-tenant authority is not a
// local feature toggle.
func LocalCapabilityForLicenseFeature(feature string) (FeatureID, bool) {
	switch strings.TrimSpace(feature) {
	case "agent_profiles":
		return FeatureAgentProfiles, true
	case "rbac":
		return FeatureRBAC, true
	case "audit_logging":
		return FeatureAuditLogging, true
	case "advanced_reporting":
		return FeatureAdvancedReporting, true
	case "external_probe":
		return FeatureExternalProbe, true
	default:
		return "", false
	}
}

func cloneProfile(profile ForkProfile) ForkProfile {
	profile.EnabledFeatures = append([]FeatureID(nil), profile.EnabledFeatures...)
	return profile
}

type Source string

const (
	SourceForkPublic  Source = "fork_public"
	SourceUnavailable Source = "unavailable"
)

type AccessDecision struct {
	Capability FeatureID `json:"capability"`
	Available  bool      `json:"available"`
	Source     Source    `json:"source"`
	ForkError  error     `json:"-"`
}

// AccessResolver is the single local fork capability admission boundary.
type AccessResolver struct {
	Fork *Manager
}

func (r AccessResolver) Resolve(ctx context.Context, id FeatureID) AccessDecision {
	decision := AccessDecision{Capability: id, Source: SourceUnavailable}
	if r.Fork != nil {
		if available, err := r.Fork.Available(id); err == nil && available {
			decision.Available, decision.Source = true, SourceForkPublic
			return decision
		} else if err != nil {
			decision.ForkError = err
		}
	}
	_ = ctx
	return decision
}

func (r AccessResolver) Require(ctx context.Context, id FeatureID) (AccessDecision, error) {
	decision := r.Resolve(ctx, id)
	if !decision.Available {
		if decision.ForkError != nil {
			return decision, decision.ForkError
		}
		return decision, fmt.Errorf("%w: %s", ErrUnavailable, id)
	}
	return decision, nil
}
