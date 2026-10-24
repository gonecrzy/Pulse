package forkfeatures

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultProfileDisablesEveryForkCapability(t *testing.T) {
	manager := NewManager(nil)
	snapshot, err := manager.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Capabilities) != len(registeredFeatures) {
		t.Fatalf("got %d capabilities, want %d", len(snapshot.Capabilities), len(registeredFeatures))
	}
	for _, capability := range snapshot.Capabilities {
		if capability.Available {
			t.Errorf("%s is available in the default profile", capability.ID)
		}
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
		"version":       func(p *ForkProfile) { p.Version++ },
		"unknown":       func(p *ForkProfile) { p.EnabledFeatures = []FeatureID{"fork.unknown"} },
		"unimplemented": func(p *ForkProfile) { p.EnabledFeatures = []FeatureID{FeatureKomodoReadOnly} },
		"duplicate":     func(p *ForkProfile) { p.EnabledFeatures = []FeatureID{FeatureAIInvestigation, FeatureAIInvestigation} },
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

func TestAccessResolverSeparatesForkAndCommercialSources(t *testing.T) {
	manager := NewManager(nil)
	resolver := AccessResolver{Fork: manager, Commercial: func(_ context.Context, id FeatureID) bool { return id == FeatureAIInvestigation }}
	decision := resolver.Resolve(context.Background(), FeatureAIInvestigation)
	if !decision.Available || decision.Source != SourceCommercial {
		t.Fatalf("commercial decision = %+v", decision)
	}
	profile := DefaultProfile()
	profile.EnabledFeatures = []FeatureID{FeatureAIInvestigation}
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	decision = resolver.Resolve(context.Background(), FeatureAIInvestigation)
	if !decision.Available || decision.Source != SourceForkPublic {
		t.Fatalf("fork decision = %+v", decision)
	}
}

type memoryStore struct{ profile ForkProfile }

func (s *memoryStore) LoadForkProfile() (*ForkProfile, error) { p := s.profile; return &p, nil }
func (s *memoryStore) SaveForkProfile(p ForkProfile) error    { s.profile = p; return nil }
