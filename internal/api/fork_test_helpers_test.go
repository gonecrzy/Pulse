package api

import (
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
)

func enableRouterForkCapabilitiesForTest(t *testing.T, router *Router, ids ...forkfeatures.FeatureID) {
	t.Helper()
	if router == nil || router.forkProfileManager == nil {
		t.Fatal("router fork profile manager is unavailable")
	}
	profile := forkfeatures.DefaultProfile()
	for _, id := range ids {
		profile.EnabledFeatures = append(profile.EnabledFeatures, id)
	}
	if _, err := router.forkProfileManager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable local fork capabilities: %v", err)
	}
}
