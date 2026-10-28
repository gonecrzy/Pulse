package config

import (
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
)

func TestCodexLBGatewayConfigurationPersistsAcrossRestartWithoutDeletingLegacyProviders(t *testing.T) {
	dir := t.TempDir()
	persistence := NewConfigPersistence(dir)
	manager := forkfeatures.NewManager(persistence)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable Codex-LB capability: %v", err)
	}
	persistence.SetForkProfileManager(manager)

	want := AIConfig{
		Enabled:                       true,
		Model:                         "codex_lb_gateway:owner/model-v1",
		OpenAIAPIKey:                  "legacy-openai-secret",
		OpenAIBaseURL:                 "https://legacy.example/v1",
		CodexLBGatewayEndpoint:        "https://gateway.example/v1",
		CodexLBGatewayAPIKey:          "gateway-secret",
		CodexLBGatewayAllowPrivateURL: false,
	}
	if err := persistence.SaveAIConfig(want); err != nil {
		t.Fatalf("SaveAIConfig(): %v", err)
	}

	restarted := NewConfigPersistence(dir)
	restarted.SetForkProfileManager(forkfeatures.NewManager(restarted))
	got, err := restarted.LoadAIConfig()
	if err != nil {
		t.Fatalf("LoadAIConfig() after restart: %v", err)
	}
	if got.CodexLBGatewayEndpoint != want.CodexLBGatewayEndpoint || got.CodexLBGatewayAPIKey != want.CodexLBGatewayAPIKey {
		t.Fatalf("gateway settings after restart = endpoint %q, credential %q; want configured values", got.CodexLBGatewayEndpoint, got.CodexLBGatewayAPIKey)
	}
	if got.OpenAIAPIKey != want.OpenAIAPIKey || got.OpenAIBaseURL != want.OpenAIBaseURL {
		t.Fatalf("legacy provider settings were changed: openai key set=%v, endpoint=%q", got.OpenAIAPIKey != "", got.OpenAIBaseURL)
	}
	if !got.CodexLBGatewayPolicyEnabled() {
		t.Fatal("gateway capability was not applied after restart")
	}
}

func TestCodexLBGatewayPolicyFollowsInstallationCapability(t *testing.T) {
	persistence := NewConfigPersistence(t.TempDir())
	manager := forkfeatures.NewManager(persistence)
	persistence.SetForkProfileManager(manager)
	if err := persistence.SaveAIConfig(AIConfig{
		Enabled:                true,
		Model:                  "codex_lb_gateway:owner/model-v1",
		CodexLBGatewayEndpoint: "https://gateway.example/v1",
		CodexLBGatewayAPIKey:   "gateway-secret",
		OpenAIAPIKey:           "legacy-openai-secret",
	}); err != nil {
		t.Fatalf("SaveAIConfig(): %v", err)
	}
	cfg, err := persistence.LoadAIConfig()
	if err != nil {
		t.Fatalf("LoadAIConfig(): %v", err)
	}
	if cfg.CodexLBGatewayPolicyEnabled() {
		t.Fatal("gateway-only policy was active while the fork capability was disabled")
	}
	if !cfg.HasProvider(AIProviderOpenAI) {
		t.Fatal("legacy OpenAI provider config should remain available while gateway policy is disabled")
	}
	if err := cfg.ValidateProviderRoute(AIProviderCodexLBGateway, "owner/model-v1"); err == nil {
		t.Fatal("Codex-LB route was admitted while fork.codex_lb_gateway was disabled")
	}
	if err := cfg.ValidateProviderRoute(AIProviderOpenAI, "gpt-4o"); err != nil {
		t.Fatalf("legacy OpenAI route changed while the gateway capability was disabled: %v", err)
	}
}

func TestCodexLBGatewayRouteAdmissionRequiresAnExplicitlySelectedGatewayModel(t *testing.T) {
	persistence := NewConfigPersistence(t.TempDir())
	manager := forkfeatures.NewManager(persistence)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable Codex-LB capability: %v", err)
	}
	persistence.SetForkProfileManager(manager)
	if err := persistence.SaveAIConfig(AIConfig{
		Model:                         "codex_lb_gateway:owner/model-v1",
		ChatModel:                     "codex_lb_gateway:owner/chat-model",
		CodexLBGatewayEndpoint:        "https://gateway.example/v1",
		CodexLBGatewayAPIKey:          "gateway-secret",
		CodexLBGatewayAllowPrivateURL: false,
		OpenAIAPIKey:                  "legacy-openai-secret",
	}); err != nil {
		t.Fatalf("SaveAIConfig(): %v", err)
	}
	cfg, err := persistence.LoadAIConfig()
	if err != nil {
		t.Fatalf("LoadAIConfig(): %v", err)
	}
	for _, selected := range []string{"owner/model-v1", "codex_lb_gateway:owner/model-v1", "owner/chat-model"} {
		if err := cfg.ValidateProviderRoute(AIProviderCodexLBGateway, selected); err != nil {
			t.Errorf("selected gateway model %q rejected: %v", selected, err)
		}
	}
	if err := cfg.ValidateProviderRoute(AIProviderOpenAI, "gpt-4o"); err == nil {
		t.Fatal("direct OpenAI route accepted while gateway capability is enabled")
	}
	if err := cfg.ValidateProviderRoute(AIProviderCodexLBGateway, "owner/unselected-model"); err == nil {
		t.Fatal("unselected gateway model was admitted")
	}
	if err := cfg.ValidateProviderRoute(AIProviderCodexLBGateway, "openai:gpt-4o"); err == nil {
		t.Fatal("explicit alternate provider prefix was admitted on the gateway route")
	}
	missingCredential := *cfg
	missingCredential.CodexLBGatewayAPIKey = ""
	if err := missingCredential.ValidateProviderRoute(AIProviderCodexLBGateway, "owner/model-v1"); err == nil {
		t.Fatal("gateway route was admitted without its dedicated credential")
	}
}

func TestValidateCodexLBGatewayEndpointRequiresTrustedSchemesAndBlocksMetadata(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		allowPrivate bool
		wantErr      bool
	}{
		{name: "hosted https", endpoint: "https://gateway.example/v1"},
		{name: "private https requires opt-in", endpoint: "https://10.20.30.40/v1", wantErr: true},
		{name: "private https opt-in", endpoint: "https://10.20.30.40/v1", allowPrivate: true},
		{name: "loopback http opt-in", endpoint: "http://127.0.0.1:9000/v1", allowPrivate: true},
		{name: "public http denied even with private opt-in", endpoint: "http://gateway.example/v1", allowPrivate: true, wantErr: true},
		{name: "metadata literal denied", endpoint: "https://169.254.169.254/latest/meta-data", allowPrivate: true, wantErr: true},
		{name: "ipv6 metadata literal denied", endpoint: "https://[fd00:ec2::254]/latest/meta-data", allowPrivate: true, wantErr: true},
		{name: "alibaba metadata literal denied", endpoint: "https://100.100.100.200/latest/meta-data", allowPrivate: true, wantErr: true},
		{name: "azure platform address denied", endpoint: "https://168.63.129.16/v1", allowPrivate: true, wantErr: true},
		{name: "metadata hostname denied", endpoint: "https://metadata.google.internal/v1", allowPrivate: true, wantErr: true},
		{name: "embedded credential denied", endpoint: "https://user:pass@gateway.example/v1", wantErr: true},
		{name: "query denied", endpoint: "https://gateway.example/v1?key=secret", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCodexLBGatewayEndpoint(tt.endpoint, tt.allowPrivate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCodexLBGatewayEndpoint(%q, %v) error = %v, wantErr=%v", tt.endpoint, tt.allowPrivate, err, tt.wantErr)
			}
		})
	}
}

func TestCodexLBGatewaySettingsAreTenantScoped(t *testing.T) {
	base := t.TempDir()
	mtp := NewMultiTenantPersistence(base)
	manager := forkfeatures.NewManager(NewConfigPersistence(base))
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable Codex-LB capability: %v", err)
	}
	mtp.SetForkProfileManager(manager)

	for orgID, values := range map[string][2]string{
		"org-a": {"https://a.example/v1", "a-secret"},
		"org-b": {"https://b.example/v1", "b-secret"},
	} {
		persistence, err := mtp.GetPersistence(orgID)
		if err != nil {
			t.Fatalf("GetPersistence(%q): %v", orgID, err)
		}
		if err := persistence.SaveAIConfig(AIConfig{CodexLBGatewayEndpoint: values[0], CodexLBGatewayAPIKey: values[1]}); err != nil {
			t.Fatalf("SaveAIConfig(%q): %v", orgID, err)
		}
	}

	for orgID, want := range map[string][2]string{
		"org-a": {"https://a.example/v1", "a-secret"},
		"org-b": {"https://b.example/v1", "b-secret"},
	} {
		persistence, err := mtp.GetPersistence(orgID)
		if err != nil {
			t.Fatalf("GetPersistence(%q): %v", orgID, err)
		}
		got, err := persistence.LoadAIConfig()
		if err != nil {
			t.Fatalf("LoadAIConfig(%q): %v", orgID, err)
		}
		if got.CodexLBGatewayEndpoint != want[0] || got.CodexLBGatewayAPIKey != want[1] {
			t.Errorf("tenant %q gateway = (%q, credential-set=%v), want (%q, credential-set=true)", orgID, got.CodexLBGatewayEndpoint, got.CodexLBGatewayAPIKey != "", want[0])
		}
	}
}
