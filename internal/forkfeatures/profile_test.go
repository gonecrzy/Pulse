package forkfeatures

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultProfileEnablesSupportedLocalCapabilitiesOnly(t *testing.T) {
	manager := NewManager(nil)
	snapshot, err := manager.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Capabilities) != len(registeredFeatures) {
		t.Fatalf("got %d capabilities, want %d", len(snapshot.Capabilities), len(registeredFeatures))
	}
	wantAvailable := map[FeatureID]bool{
		FeatureAIInvestigation:     true,
		FeatureGovernedActions:     true,
		FeatureKomodoReadOnly:      true,
		FeatureDeterministicTriage: true,
	}
	for _, capability := range snapshot.Capabilities {
		if got, want := capability.Available, wantAvailable[capability.ID]; got != want {
			t.Errorf("%s available = %v, want %v", capability.ID, got, want)
		}
		if capability.ID == FeatureActionSafety && capability.Available {
			t.Error("action safety is available before its implementation is complete")
		}
		if capability.Available != (capability.Implemented && capability.Enabled) {
			t.Errorf("%s available=%v does not match implemented=%v and enabled=%v", capability.ID, capability.Available, capability.Implemented, capability.Enabled)
		}
	}
}

func TestLocalLicenseMappingsAreExplicitAndExcludeCompositeEntitlements(t *testing.T) {
	for licenseFeature, want := range map[string]FeatureID{
		"agent_profiles":     FeatureAgentProfiles,
		"rbac":               FeatureRBAC,
		"audit_logging":      FeatureAuditLogging,
		"advanced_reporting": FeatureAdvancedReporting,
		"external_probe":     FeatureExternalProbe,
	} {
		if got, ok := LocalCapabilityForLicenseFeature(licenseFeature); !ok || got != want {
			t.Errorf("LocalCapabilityForLicenseFeature(%q) = %q, %v; want %q, true", licenseFeature, got, ok, want)
		}
	}
	for _, feature := range []string{"ai_autofix", "ai_alerts", "relay", "multi_tenant", "white_label", "unknown"} {
		if id, ok := LocalCapabilityForLicenseFeature(feature); ok {
			t.Errorf("LocalCapabilityForLicenseFeature(%q) = %q, true; want unmapped", feature, id)
		}
	}
}

func TestManagerMigratesVersionOneProfileWithoutChangingExplicitChoices(t *testing.T) {
	store := &memoryStore{profile: ForkProfile{Version: 1, Revision: 7, EnabledFeatures: []FeatureID{FeatureAIInvestigation}}}
	manager := NewManager(store)
	snapshot, err := manager.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProfileVersion != ProfileVersion || snapshot.Revision != 7 {
		t.Fatalf("migrated snapshot version/revision = %d/%d, want %d/7", snapshot.ProfileVersion, snapshot.Revision, ProfileVersion)
	}
	if !snapshot.Has(FeatureAIInvestigation) || snapshot.Has(FeatureGovernedActions) {
		t.Fatalf("migration changed the explicit v1 feature set: %+v", snapshot.Capabilities)
	}
	if store.profile.Version != 1 {
		t.Fatalf("read-only migration wrote profile version %d; want persisted source unchanged at v1", store.profile.Version)
	}
}

func TestManagerEnablesOnlyImplementedRegisteredCapabilities(t *testing.T) {
	manager := NewManager(nil)
	profile := DefaultProfile()
	profile.EnabledFeatures = []FeatureID{FeatureAIInvestigation, FeatureGovernedActions}
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	for _, id := range []FeatureID{FeatureAIInvestigation, FeatureGovernedActions} {
		available, err := manager.Available(id)
		if err != nil || !available {
			t.Errorf("%s available = %v, err = %v", id, available, err)
		}
	}
}

func TestManagerRejectsInvalidAndUnavailableProfileEntries(t *testing.T) {
	for name, mutate := range map[string]func(*ForkProfile){
		"version":      func(p *ForkProfile) { p.Version++ },
		"unknown":      func(p *ForkProfile) { p.EnabledFeatures = []FeatureID{"fork.unknown"} },
		"experimental": func(p *ForkProfile) { p.EnabledFeatures = []FeatureID{FeatureOperationalCases} },
		"unavailable":  func(p *ForkProfile) { p.EnabledFeatures = []FeatureID{FeatureDiscordOps} },
		"duplicate":    func(p *ForkProfile) { p.EnabledFeatures = []FeatureID{FeatureAIInvestigation, FeatureAIInvestigation} },
	} {
		t.Run(name, func(t *testing.T) {
			manager := NewManager(nil)
			profile := DefaultProfile()
			mutate(&profile)
			if _, err := manager.Replace(profile, profile.Revision); err == nil {
				t.Fatal("Replace accepted an invalid profile")
			}
		})
	}
}

func TestManagerRejectsStaleProfileRevision(t *testing.T) {
	manager := NewManager(nil)
	profile := DefaultProfile()
	profile.EnabledFeatures = []FeatureID{FeatureAIInvestigation}
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Replace(profile, profile.Revision); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("got %v, want ErrStaleRevision", err)
	}
}

func TestManagerFailsClosedOnInvalidLoadedProfile(t *testing.T) {
	store := &memoryStore{profile: ForkProfile{Version: 77, Revision: 1}}
	manager := NewManager(store)
	if _, err := manager.Snapshot(); err == nil {
		t.Fatal("Snapshot accepted invalid persisted profile")
	}
	if available, err := manager.Available(FeatureAIInvestigation); err == nil || available {
		t.Fatalf("invalid profile availability = %v, err = %v", available, err)
	}
}

func TestAccessResolverNeverFallsBackToCommercialEntitlements(t *testing.T) {
	manager := NewManager(nil)
	profile := DefaultProfile()
	profile.EnabledFeatures = []FeatureID{FeatureGovernedActions}
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	resolver := AccessResolver{Fork: manager}
	decision := resolver.Resolve(context.Background(), FeatureAIInvestigation)
	if decision.Available || decision.Source != SourceUnavailable {
		t.Fatalf("disabled local decision = %+v", decision)
	}

	corrupt := NewManager(&memoryStore{profile: ForkProfile{Version: 77, Revision: 1}})
	decision = (AccessResolver{Fork: corrupt}).Resolve(context.Background(), FeatureAIInvestigation)
	if decision.Available || decision.ForkError == nil {
		t.Fatalf("corrupt local policy decision = %+v, want closed", decision)
	}
}

type memoryStore struct{ profile ForkProfile }

func (s *memoryStore) LoadForkProfile() (*ForkProfile, error) { p := s.profile; return &p, nil }
func (s *memoryStore) SaveForkProfile(p ForkProfile) error    { s.profile = p; return nil }
