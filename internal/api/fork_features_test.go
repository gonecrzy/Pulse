package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
	"github.com/rcourtman/pulse-go-rewrite/pkg/auth"
)

func TestForkFeatureHandlersRejectForgedAndStaleProfiles(t *testing.T) {
	manager := forkfeatures.NewManager(nil)
	handlers := NewForkFeatureHandlers(manager)

	for _, body := range []string{
		`{"version":1,"expected_revision":1,"enabled_features":["fork.long_term_metrics"]}`,
		`{"version":2,"expected_revision":1,"enabled_features":["fork.ai_investigation","fork.ai_investigation"]}`,
		`{"version":1,"expected_revision":1,"enabled_features":[],"commercial_entitlements":["ai_autofix"]}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/fork/profile", strings.NewReader(body))
		recorder := httptest.NewRecorder()
		handlers.HandleProfile(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("PUT %s status = %d, want 400: %s", body, recorder.Code, recorder.Body.String())
		}
	}

	valid := `{"version":2,"expected_revision":1,"enabled_features":["fork.ai_investigation"]}`
	first := httptest.NewRecorder()
	handlers.HandleProfile(first, httptest.NewRequest(http.MethodPut, "/api/fork/profile", strings.NewReader(valid)))
	if first.Code != http.StatusOK {
		t.Fatalf("valid profile status = %d: %s", first.Code, first.Body.String())
	}
	stale := httptest.NewRecorder()
	handlers.HandleProfile(stale, httptest.NewRequest(http.MethodPut, "/api/fork/profile", strings.NewReader(valid)))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale profile status = %d, want 409: %s", stale.Code, stale.Body.String())
	}

	var snapshot forkfeatures.Snapshot
	if err := json.Unmarshal(first.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 2 || !snapshot.Has(forkfeatures.FeatureAIInvestigation) || snapshot.Has(forkfeatures.FeatureGovernedActions) {
		t.Fatalf("resolved profile = %+v", snapshot)
	}
}

func TestForkCapabilityRouteGateFailsClosedAndUsesCentralResolver(t *testing.T) {
	manager := forkfeatures.NewManager(nil)
	disabled := forkfeatures.DefaultProfile()
	disabled.EnabledFeatures = []forkfeatures.FeatureID{}
	disabledSnapshot, err := manager.Replace(disabled, disabled.Revision)
	if err != nil {
		t.Fatal(err)
	}
	router := &Router{forkAccess: forkfeatures.AccessResolver{Fork: manager}}
	calls := 0
	gate := router.withForkCapability(forkfeatures.FeatureGovernedActions, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	gate.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/actions/plan", nil))
	if recorder.Code != http.StatusForbidden || calls != 0 {
		t.Fatalf("disabled capability status=%d calls=%d", recorder.Code, calls)
	}

	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = []forkfeatures.FeatureID{forkfeatures.FeatureGovernedActions}
	if _, err := manager.Replace(profile, disabledSnapshot.Revision); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	gate.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/actions/plan", nil))
	if recorder.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("enabled capability status=%d calls=%d", recorder.Code, calls)
	}

	invalidManager := forkfeatures.NewManager(staticForkProfileStore{profile: forkfeatures.ForkProfile{Version: 99, Revision: 1}})
	invalidRouter := &Router{forkAccess: forkfeatures.AccessResolver{Fork: invalidManager}}
	invalidCalls := 0
	invalidGate := invalidRouter.withForkCapability(forkfeatures.FeatureGovernedActions, func(http.ResponseWriter, *http.Request) { invalidCalls++ })
	invalidRecorder := httptest.NewRecorder()
	invalidGate.ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodPost, "/api/actions/plan", nil))
	if invalidRecorder.Code != http.StatusForbidden || invalidCalls != 0 {
		t.Fatalf("invalid resolver status=%d calls=%d, want closed", invalidRecorder.Code, invalidCalls)
	}
}

type staticForkProfileStore struct {
	profile forkfeatures.ForkProfile
}

func (s staticForkProfileStore) LoadForkProfile() (*forkfeatures.ForkProfile, error) {
	return &s.profile, nil
}

func (staticForkProfileStore) SaveForkProfile(forkfeatures.ForkProfile) error { return nil }

type denyingActionAuthorizer struct{}

func (denyingActionAuthorizer) Authorize(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestForkGovernedActionsDoesNotGrantActionPermission(t *testing.T) {
	manager := forkfeatures.NewManager(nil)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = []forkfeatures.FeatureID{forkfeatures.FeatureGovernedActions}
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	router := &Router{forkAccess: forkfeatures.AccessResolver{Fork: manager}}
	calls := 0
	endpoint := requireActionCapability(nil, denyingActionAuthorizer{}, auth.ActionPlan,
		router.withForkCapability(forkfeatures.FeatureGovernedActions, func(http.ResponseWriter, *http.Request) { calls++ }))
	req := httptest.NewRequest(http.MethodPost, "/api/actions/plan", nil)
	req = req.WithContext(auth.WithUser(req.Context(), "ordinary-operator"))
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden || calls != 0 {
		t.Fatalf("feature availability granted action permission: status=%d calls=%d", recorder.Code, calls)
	}
}

type allowingActionAuthorizer struct{}

func (allowingActionAuthorizer) Authorize(context.Context, string, string) (bool, error) {
	return true, nil
}

func TestAuthenticatedActionPlanRouteRequiresForkAvailability(t *testing.T) {
	rawToken := "fork-actions-route.12345678"
	cfg := newTestConfigWithTokens(t, newTokenRecord(t, rawToken, []string{config.ScopeActionsPlan}, nil))
	router := NewRouter(cfg, nil, nil, nil, nil, "1.0.0")
	cleanupTestRouter(t, router)
	router.authorizer = allowingActionAuthorizer{}
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = []forkfeatures.FeatureID{}
	if _, err := router.forkProfileManager.Replace(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/actions/plan", strings.NewReader(`{}`))
	req.Header.Set("X-API-Token", rawToken)
	recorder := httptest.NewRecorder()
	router.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "fork_capability_unavailable") {
		t.Fatalf("authenticated action plan status=%d, want fork capability refusal: %s", recorder.Code, recorder.Body.String())
	}
}

func TestForkCapabilitiesEndpointReportsOnlyForkAvailability(t *testing.T) {
	manager := forkfeatures.NewManager(nil)
	handlers := NewForkFeatureHandlers(manager)
	recorder := httptest.NewRecorder()
	handlers.HandleCapabilities(recorder, httptest.NewRequest(http.MethodGet, "/api/fork/capabilities", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var snapshot forkfeatures.Snapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, id := range []forkfeatures.FeatureID{
		forkfeatures.FeatureAIInvestigation,
		forkfeatures.FeatureGovernedActions,
		forkfeatures.FeatureKomodoReadOnly,
		forkfeatures.FeatureDeterministicTriage,
	} {
		if !snapshot.Has(id) {
			t.Fatalf("default profile did not enable %s: %+v", id, snapshot.Capabilities)
		}
	}
	for _, id := range []forkfeatures.FeatureID{
		forkfeatures.FeatureAgentProfiles,
		forkfeatures.FeatureRBAC,
		forkfeatures.FeatureAuditLogging,
		forkfeatures.FeatureAdvancedReporting,
		forkfeatures.FeatureExternalProbe,
	} {
		if snapshot.Has(id) {
			t.Fatalf("default profile enabled opt-in public capability %s: %+v", id, snapshot.Capabilities)
		}
	}
	if snapshot.Has(forkfeatures.FeatureActionSafety) {
		t.Fatalf("default profile enabled unfinished action safety: %+v", snapshot.Capabilities)
	}
	if strings.Contains(recorder.Body.String(), "ai_autofix") {
		t.Fatalf("fork capability response exposed a commercial entitlement: %s", recorder.Body.String())
	}
}

func TestForkCapabilityAndProfileHTTPRoutesRequireAuthentication(t *testing.T) {
	cfg := newTestConfigWithTokens(t, newTokenRecord(t, "fork-capabilities-reader.12345678", []string{config.ScopeMonitoringRead}, nil))
	router := NewRouter(cfg, nil, nil, nil, nil, "1.0.0")
	cleanupTestRouter(t, router)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/fork/capabilities"},
		{method: http.MethodGet, path: "/api/fork/profile"},
		{method: http.MethodPut, path: "/api/fork/profile"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		recorder := httptest.NewRecorder()
		router.Handler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s status=%d, want 401", tc.method, tc.path, recorder.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/fork/capabilities", nil)
	req.Header.Set("X-API-Token", "fork-capabilities-reader.12345678")
	recorder := httptest.NewRecorder()
	router.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated capability read status=%d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		req := httptest.NewRequest(method, "/api/fork/profile", strings.NewReader(`{"version":1,"expected_revision":1,"enabled_features":[]}`))
		req.Header.Set("X-API-Token", "fork-capabilities-reader.12345678")
		recorder := httptest.NewRecorder()
		router.Handler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("non-admin profile %s status=%d, want 403: %s", method, recorder.Code, recorder.Body.String())
		}
	}
}
