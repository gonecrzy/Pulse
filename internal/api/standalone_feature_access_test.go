package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
)

type staticLocalFeatureBase struct {
	features map[string]bool
}

func (s staticLocalFeatureBase) HasFeature(feature string) bool { return s.features[feature] }
func (s staticLocalFeatureBase) RequireFeature(feature string) error {
	if s.HasFeature(feature) {
		return nil
	}
	return errors.New("feature not included")
}

func TestStandaloneFeatureCheckerUsesExplicitForkPolicy(t *testing.T) {
	manager := forkfeatures.NewManager(nil)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = []forkfeatures.FeatureID{
		forkfeatures.FeatureAIInvestigation,
		forkfeatures.FeatureAgentProfiles,
	}
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	checker := standaloneFeatureChecker{
		base:       staticLocalFeatureBase{features: map[string]bool{}},
		access:     forkfeatures.AccessResolver{Fork: manager},
		ctx:        context.Background(),
		standalone: true,
	}
	if !checker.HasFeature("agent_profiles") || checker.RequireFeature("agent_profiles") != nil {
		t.Fatal("enabled local agent_profiles capability was not admitted")
	}
	if checker.HasFeature("rbac") || checker.RequireFeature("rbac") == nil {
		t.Fatal("disabled local RBAC capability was admitted")
	}
	if checker.HasFeature("ai_autofix") || checker.RequireFeature("ai_autofix") == nil {
		t.Fatal("Sindri investigation availability fabricated ai_autofix entitlement")
	}
	if checker.HasFeature("ai_alerts") || checker.RequireFeature("ai_alerts") == nil {
		t.Fatal("Sindri investigation availability fabricated ai_alerts entitlement")
	}

	// Hosted and explicit legacy-compatibility modes keep ordinary commercial
	// feature decisions, even when the corresponding local capability is on.
	commercial := standaloneFeatureChecker{base: staticLocalFeatureBase{features: map[string]bool{}}, access: checker.access, standalone: false}
	if commercial.HasFeature("agent_profiles") || commercial.RequireFeature("agent_profiles") == nil {
		t.Fatal("commercial mode incorrectly used local capability admission")
	}
}

func TestStandaloneLicenseMiddlewareUsesLocalCapabilityAndFailsClosed(t *testing.T) {
	baseDir := t.TempDir()
	mtp := config.NewMultiTenantPersistence(baseDir)
	manager := forkfeatures.NewManager(nil)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = []forkfeatures.FeatureID{forkfeatures.FeatureRBAC}
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	handlers := NewLicenseHandlers(mtp, false, &config.Config{ConfigPath: baseDir, DataPath: baseDir})
	handlers.SetForkAccessResolver(forkfeatures.AccessResolver{Fork: manager})

	called := 0
	protected := RequireLicenseFeature(handlers, "rbac", func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	})
	ctx := context.WithValue(context.Background(), OrgIDContextKey, "default")
	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/rbac/roles", nil).WithContext(ctx))
	if recorder.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("enabled local capability status=%d calls=%d body=%s", recorder.Code, called, recorder.Body.String())
	}

	disabled := profile
	disabled.EnabledFeatures = []forkfeatures.FeatureID{}
	if _, err := manager.Replace(disabled, profile.Revision+1); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/rbac/roles", nil).WithContext(ctx))
	if recorder.Code != http.StatusForbidden || called != 1 {
		t.Fatalf("disabled local capability status=%d calls=%d body=%s", recorder.Code, called, recorder.Body.String())
	}

	corrupt := forkfeatures.NewManager(staticForkProfileStore{profile: forkfeatures.ForkProfile{Version: 77, Revision: 1}})
	handlers.SetForkAccessResolver(forkfeatures.AccessResolver{Fork: corrupt})
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/rbac/roles", nil).WithContext(ctx))
	if recorder.Code != http.StatusForbidden || called != 1 {
		t.Fatalf("corrupt local policy status=%d calls=%d body=%s", recorder.Code, called, recorder.Body.String())
	}
}
