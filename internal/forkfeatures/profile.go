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

const ProfileVersion = 1

type FeatureID string

const (
	FeatureCodexLBGateway      FeatureID = "fork.codex_lb_gateway"
	FeatureKomodoReadOnly      FeatureID = "fork.komodo_read_only"
	FeatureDeterministicTriage FeatureID = "fork.deterministic_triage"
	FeatureAIInvestigation     FeatureID = "fork.ai_investigation"
	FeatureGovernedActions     FeatureID = "fork.governed_actions"
	FeatureActionSafety        FeatureID = "fork.action_safety"
	FeatureOperationalCases    FeatureID = "fork.operational_cases"
	FeatureLongTermMetrics     FeatureID = "fork.long_term_metrics"
	FeatureDiscordOps          FeatureID = "fork.discord_ops"
)

type FeatureDefinition struct {
	ID             FeatureID `json:"id"`
	Implemented    bool      `json:"implemented"`
	Implementation string    `json:"implementation,omitempty"`
}

var registeredFeatures = []FeatureDefinition{
	{ID: FeatureCodexLBGateway, Implementation: "OpenAI-compatible Codex-LB provider enforcement"},
	{ID: FeatureKomodoReadOnly, Implementation: "read-only Komodo monitoring adapter"},
	{ID: FeatureDeterministicTriage, Implementation: "deterministic alert triage extensions"},
	{ID: FeatureAIInvestigation, Implemented: true, Implementation: "Pulse chat investigation runner and findings store"},
	{ID: FeatureGovernedActions, Implemented: true, Implementation: "Pulse typed action planner and canonical action lifecycle"},
	{ID: FeatureActionSafety, Implementation: "strict remediation policy and safety controls"},
	{ID: FeatureOperationalCases, Implementation: "multi-alert operational cases and repair plans"},
	{ID: FeatureLongTermMetrics, Implementation: "extended Pulse metric retention tiers"},
	{ID: FeatureDiscordOps, Implementation: "Discord conversational operations"},
}

type ForkProfile struct {
	Version         int         `json:"version"`
	Revision        uint64      `json:"revision"`
	EnabledFeatures []FeatureID `json:"enabled_features"`
}

func DefaultProfile() ForkProfile {
	return ForkProfile{Version: ProfileVersion, Revision: 1, EnabledFeatures: []FeatureID{}}
}

type Capability struct {
	ID          FeatureID `json:"id"`
	Available   bool      `json:"available"`
	Implemented bool      `json:"implemented"`
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
	if err := validateProfile(*profile); err != nil {
		manager.loadErr = fmt.Errorf("validate persisted fork profile: %w", err)
		return manager
	}
	manager.profile = cloneProfile(*profile)
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
		available := feature.Implemented && enabled[feature.ID]
		source := "unavailable"
		if available {
			source = "fork_public"
		}
		snapshot.Capabilities = append(snapshot.Capabilities, Capability{
			ID: feature.ID, Available: available, Implemented: feature.Implemented, Source: source,
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
		available := feature.Implemented && enabled[feature.ID]
		source := "unavailable"
		if available {
			source = "fork_public"
		}
		snapshot.Capabilities = append(snapshot.Capabilities, Capability{ID: feature.ID, Available: available, Implemented: feature.Implemented, Source: source})
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
		if !feature.Implemented {
			return fmt.Errorf("fork capability %q has no complete implementation", id)
		}
	}
	return nil
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

func cloneProfile(profile ForkProfile) ForkProfile {
	profile.EnabledFeatures = append([]FeatureID(nil), profile.EnabledFeatures...)
	return profile
}

type Source string

const (
	SourceForkPublic  Source = "fork_public"
	SourceCommercial  Source = "commercial_license"
	SourceUnavailable Source = "unavailable"
)

type AccessDecision struct {
	Capability FeatureID `json:"capability"`
	Available  bool      `json:"available"`
	Source     Source    `json:"source"`
	ForkError  error     `json:"-"`
}

type CommercialChecker func(context.Context, FeatureID) bool

// AccessResolver combines the central fork availability profile with an
// existing commercial check where a public route already has a paid path.
// It never mutates commercial entitlements or treats fork availability as one.
type AccessResolver struct {
	Fork       *Manager
	Commercial CommercialChecker
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
	if r.Commercial != nil && r.Commercial(ctx, id) {
		decision.Available, decision.Source = true, SourceCommercial
	}
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
