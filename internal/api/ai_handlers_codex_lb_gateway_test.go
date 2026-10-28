package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
	"github.com/stretchr/testify/require"
)

func TestAISettingsCodexLBGatewayStatusMasksSavedCredential(t *testing.T) {
	const secret = "codex-lb-test-secret-must-stay-server-side"
	dir := t.TempDir()
	persistence := config.NewConfigPersistence(dir)
	manager := forkfeatures.NewManager(persistence)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	_, err := manager.Replace(profile, profile.Revision)
	require.NoError(t, err)
	persistence.SetForkProfileManager(manager)
	require.NoError(t, persistence.SaveAIConfig(config.AIConfig{
		Enabled:                       true,
		Model:                         "codex_lb_gateway:owner/model-v1",
		CodexLBGatewayEndpoint:        "https://gateway.example/v1",
		CodexLBGatewayAPIKey:          secret,
		CodexLBGatewayAllowPrivateURL: false,
	}))

	handler := newTestAISettingsHandler(&config.Config{DataPath: dir}, persistence, nil)
	handler.SetForkAccessResolver(forkfeatures.AccessResolver{Fork: manager})
	recorder := httptest.NewRecorder()
	handler.HandleGetAISettings(recorder, newLoopbackRequest(http.MethodGet, "/api/settings/ai", nil))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	responseBody := recorder.Body.String()
	require.NotContains(t, responseBody, secret)
	require.True(t, strings.Contains(responseBody, `"credential_configured":true`), responseBody)
	require.True(t, strings.Contains(responseBody, `"endpoint":"https://gateway.example/v1"`), responseBody)
	require.True(t, strings.Contains(responseBody, `"capability_available":true`), responseBody)
}
