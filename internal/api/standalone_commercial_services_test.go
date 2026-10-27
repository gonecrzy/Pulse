package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/pkg/edition"
	pkglicensing "github.com/rcourtman/pulse-go-rewrite/pkg/licensing"
	licensetestsupport "github.com/rcourtman/pulse-go-rewrite/pkg/licensing/testsupport"
)

func TestStandaloneRuntimeDoesNotExchangeOrPollPersistedLicenses(t *testing.T) {
	previousEdition := edition.Current()
	// A standalone fork remains independent even if built with Pro edition
	// files; only hosted mode or an explicit operator opt-in restores Pulse
	// commercial service traffic.
	edition.SetEdition(edition.Pro)
	t.Cleanup(func() { edition.SetEdition(previousEdition) })
	t.Setenv("PULSE_LICENSE_DEV_MODE", "false")

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("PULSE_LICENSE_SERVER_URL", server.URL)

	grantJWT, publicKey, err := licensetestsupport.GenerateGrantJWTForTesting(pkglicensing.GrantClaims{
		LicenseID: "lic_local_restore",
		Tier:      string(pkglicensing.TierPro),
		State:     "active",
		Features:  []string{pkglicensing.FeatureAIAutoFix},
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("generate grant: %v", err)
	}
	pkglicensing.SetPublicKey(publicKey)
	t.Cleanup(func() { pkglicensing.SetPublicKey(nil) })

	baseDir := t.TempDir()
	mtp := config.NewMultiTenantPersistence(baseDir)
	configPersistence, err := mtp.GetPersistence("default")
	if err != nil {
		t.Fatalf("get config persistence: %v", err)
	}
	persistence, err := pkglicensing.NewPersistence(configPersistence.GetConfigDir())
	if err != nil {
		t.Fatalf("new licensing persistence: %v", err)
	}
	if err := persistence.SaveActivationState(&pkglicensing.ActivationState{
		InstallationID:      "inst_local_restore",
		InstallationToken:   "pit_live_local_restore",
		LicenseID:           "lic_local_restore",
		GrantJWT:            grantJWT,
		GrantJTI:            "grant_local_restore",
		GrantExpiresAt:      time.Now().Add(time.Hour).Unix(),
		InstanceFingerprint: "fp_local_restore",
		LicenseServerURL:    server.URL,
		ActivatedAt:         time.Now().Unix(),
		LastRefreshedAt:     time.Now().Unix(),
	}); err != nil {
		t.Fatalf("save activation state: %v", err)
	}
	legacyJWT, err := licensetestsupport.GenerateLicenseForTesting("operator@example.test", pkglicensing.TierPro, 24*time.Hour)
	if err != nil {
		t.Fatalf("generate legacy license: %v", err)
	}
	if err := persistence.Save(legacyJWT); err != nil {
		t.Fatalf("save legacy license: %v", err)
	}
	billingStore := config.NewFileBillingStore(baseDir)
	if err := billingStore.SaveBillingState("default", &pkglicensing.BillingState{
		EntitlementRefreshToken: "etr_legacy_refresh",
	}); err != nil {
		t.Fatalf("save legacy hosted entitlement state: %v", err)
	}

	licenseHandlers := NewLicenseHandlers(mtp, false, &config.Config{
		ConfigPath:        baseDir,
		DataPath:          baseDir,
		ProTrialSignupURL: server.URL,
	})
	t.Cleanup(licenseHandlers.StopAllBackgroundLoops)
	ctx := context.WithValue(context.Background(), OrgIDContextKey, "default")
	service := licenseHandlers.Service(ctx)
	if service == nil || service.IsActivated() {
		t.Fatal("standalone runtime loaded a persisted upstream activation as current entitlement")
	}
	if status := service.Status().Synchronization; status != nil && status.Running {
		t.Fatalf("standalone runtime started installation status polling: %+v", status)
	}
	if loop := licenseHandlers.hostedEntitlementRefreshLoop("default"); loop != nil && loop.isRunning() {
		t.Fatal("standalone runtime started hosted entitlement refresh")
	}
	if stored, err := persistence.LoadActivationState(); err != nil || stored == nil {
		t.Fatalf("standalone startup did not preserve activation data for rollback: state=%v err=%v", stored, err)
	}
	if storedLegacy, err := persistence.Load(); err != nil || storedLegacy == "" {
		t.Fatalf("standalone startup did not preserve legacy license data: present=%v err=%v", storedLegacy != "", err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := requests.Load(); got != 0 {
		t.Fatalf("standalone startup made %d implicit Pulse licensing requests", got)
	}
	if _, running := licenseHandlers.legacyExchangeRetries.Load("default"); running {
		t.Fatal("standalone runtime started the legacy license exchange retry loop")
	}
	licenseHandlers.StopAllBackgroundLoops()
}

func TestStandaloneRuntimeKeepsEntitlementsRouteWithoutInventingSubscription(t *testing.T) {
	previousEdition := edition.Current()
	edition.SetEdition(edition.Pro)
	t.Cleanup(func() { edition.SetEdition(previousEdition) })
	baseDir := t.TempDir()
	mtp := config.NewMultiTenantPersistence(baseDir)
	licenseHandlers := NewLicenseHandlers(mtp, false, &config.Config{ConfigPath: baseDir, DataPath: baseDir})
	req := httptest.NewRequest(http.MethodGet, "/api/license/entitlements", nil)
	req = req.WithContext(context.WithValue(req.Context(), OrgIDContextKey, "default"))
	recorder := httptest.NewRecorder()
	licenseHandlers.HandleEntitlements(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("entitlements status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		CommercialServicesEnabled bool             `json:"commercial_services_enabled"`
		HostedMode                bool             `json:"hosted_mode"`
		SubscriptionState         string           `json:"subscription_state"`
		Tier                      string           `json:"tier"`
		TrialEligible             bool             `json:"trial_eligible"`
		UpgradeReasons            []map[string]any `json:"upgrade_reasons"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode entitlements response: %v", err)
	}
	if payload.CommercialServicesEnabled || payload.HostedMode || payload.SubscriptionState != "" || payload.TrialEligible || len(payload.UpgradeReasons) != 0 {
		t.Fatalf("standalone entitlement payload invented commercial state: %+v", payload)
	}
	if payload.Tier != string(pkglicensing.TierFree) {
		t.Fatalf("tier = %q, want locally resolved Free tier", payload.Tier)
	}

	runtimeReq := httptest.NewRequest(http.MethodGet, "/api/license/runtime-capabilities", nil)
	runtimeReq = runtimeReq.WithContext(context.WithValue(runtimeReq.Context(), OrgIDContextKey, "default"))
	runtimeRecorder := httptest.NewRecorder()
	licenseHandlers.HandleRuntimeCapabilities(runtimeRecorder, runtimeReq)
	if runtimeRecorder.Code != http.StatusOK {
		t.Fatalf("runtime capabilities status = %d: %s", runtimeRecorder.Code, runtimeRecorder.Body.String())
	}
	var runtimePayload RuntimeCapabilitiesPayload
	if err := json.Unmarshal(runtimeRecorder.Body.Bytes(), &runtimePayload); err != nil {
		t.Fatalf("decode runtime capabilities response: %v", err)
	}
	if runtimePayload.CommercialServicesEnabled || runtimePayload.HostedMode {
		t.Fatalf("standalone runtime capabilities enabled commercial workflows: %+v", runtimePayload)
	}
}

func TestStandaloneActivationAndCheckoutRoutesFailWithoutOutboundRequest(t *testing.T) {
	previousEdition := edition.Current()
	edition.SetEdition(edition.Community)
	t.Cleanup(func() { edition.SetEdition(previousEdition) })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("PULSE_LICENSE_SERVER_URL", server.URL)
	baseDir := t.TempDir()
	mtp := config.NewMultiTenantPersistence(baseDir)
	licenseHandlers := NewLicenseHandlers(mtp, false, &config.Config{ConfigPath: baseDir, DataPath: baseDir})

	activate := httptest.NewRecorder()
	licenseHandlers.HandleActivateLicense(activate, httptest.NewRequest(http.MethodPost, "/api/license/activate", nil))
	if activate.Code != http.StatusGone {
		t.Fatalf("activation status = %d, want 410: %s", activate.Code, activate.Body.String())
	}
	checkout := httptest.NewRecorder()
	licenseHandlers.HandleCheckoutStart(checkout, httptest.NewRequest(http.MethodGet, "/auth/license-purchase-start", nil))
	if checkout.Code != http.StatusGone {
		t.Fatalf("checkout status = %d, want 410: %s", checkout.Code, checkout.Body.String())
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("disabled commercial routes made %d outbound requests", got)
	}
}
