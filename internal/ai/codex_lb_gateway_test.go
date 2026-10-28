package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
	"github.com/rcourtman/pulse-go-rewrite/internal/models"
)

func newEnabledCodexLBGatewayServiceForTest(t *testing.T, endpoint string) *Service {
	t.Helper()
	persistence := config.NewConfigPersistence(t.TempDir())
	manager := forkfeatures.NewManager(persistence)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable Codex-LB capability: %v", err)
	}
	persistence.SetForkProfileManager(manager)
	if err := persistence.SaveAIConfig(config.AIConfig{
		Enabled:                       true,
		Model:                         "codex_lb_gateway:owner/model-v1",
		PatrolModel:                   "codex_lb_gateway:owner/model-v1",
		CodexLBGatewayEndpoint:        endpoint,
		CodexLBGatewayAPIKey:          "gateway-test-secret",
		CodexLBGatewayAllowPrivateURL: true,
		OpenAIAPIKey:                  "legacy-openai-secret",
	}); err != nil {
		t.Fatalf("SaveAIConfig(): %v", err)
	}
	service := NewService(persistence, nil)
	if err := service.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig(): %v", err)
	}
	return service
}

func TestCodexLBGatewayAuthenticationFailureDoesNotFallbackAndPatrolTriageContinues(t *testing.T) {
	var gatewayRequests atomic.Int32
	var alternateRequests atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayRequests.Add(1)
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, `{"error":{"message":"gateway authentication failed"}}`, http.StatusUnauthorized)
	}))
	defer gateway.Close()
	alternate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alternateRequests.Add(1)
		http.NotFound(w, r)
	}))
	defer alternate.Close()

	persistence := config.NewConfigPersistence(t.TempDir())
	manager := forkfeatures.NewManager(persistence)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable Codex-LB capability: %v", err)
	}
	persistence.SetForkProfileManager(manager)
	if err := persistence.SaveAIConfig(config.AIConfig{
		Enabled:                       true,
		Model:                         "codex_lb_gateway:owner/model-v1",
		CodexLBGatewayEndpoint:        gateway.URL + "/v1",
		CodexLBGatewayAPIKey:          "gateway-test-secret",
		CodexLBGatewayAllowPrivateURL: true,
		OpenAIBaseURL:                 alternate.URL + "/v1",
		OpenAIAPIKey:                  "legacy-openai-secret",
		CodexSubscriptionEnabled:      true,
	}); err != nil {
		t.Fatalf("SaveAIConfig(): %v", err)
	}

	service := NewService(persistence, nil)
	if err := service.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig(): %v", err)
	}
	if err := service.TestConnection(context.Background()); err == nil {
		t.Fatal("TestConnection() accepted gateway authentication failure")
	}
	if gatewayRequests.Load() == 0 {
		t.Fatal("gateway was not contacted")
	}
	if got := alternateRequests.Load(); got != 0 {
		t.Fatalf("alternate OpenAI endpoint received %d requests after gateway failure", got)
	}

	patrol := NewPatrolService(service, nil)
	state := models.StateSnapshot{DockerHosts: []models.DockerHost{{
		ID:       "host",
		Hostname: "host",
		Status:   "online",
		Containers: []models.DockerContainer{{
			ID: "container-1", Name: "database", State: "running", Health: "unhealthy",
		}},
	}}}
	triage := patrol.runDeterministicTriageState(context.Background(), patrolRuntimeStateForTest(patrol, state), nil, nil)
	if len(triage.Flags) == 0 {
		t.Fatal("deterministic Patrol triage stopped producing findings after the gateway failure")
	}
}

func TestCodexLBGatewayModelDiscoveryUsesCanonicalProviderIdentifier(t *testing.T) {
	seenAuthorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		seenAuthorization <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"owner/model-v1","name":"Model V1"}]}`)
	}))
	defer server.Close()

	service := newEnabledCodexLBGatewayServiceForTest(t, server.URL+"/v1")
	models, cached, err := service.ListModelsWithCache(context.Background())
	if err != nil {
		t.Fatalf("ListModelsWithCache(): %v", err)
	}
	if cached || len(models) != 1 {
		t.Fatalf("model discovery returned %+v cached=%v; want one fresh gateway model", models, cached)
	}
	if models[0].Provider != config.AIProviderCodexLBGateway || models[0].ID != "codex_lb_gateway:owner/model-v1" {
		t.Fatalf("discovered model lost canonical provider identity: %+v", models[0])
	}
	if got := <-seenAuthorization; got != "Bearer gateway-test-secret" {
		t.Fatalf("gateway Authorization = %q, want configured credential", got)
	}
}

func TestCodexLBGatewayPatrolPreflightUsesCanonicalProviderIdentifier(t *testing.T) {
	type seenRequest struct {
		authorization string
		model         string
	}
	seen := make(chan seenRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		authorization := r.Header.Get("Authorization")
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode preflight request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		seen <- seenRequest{authorization: authorization, model: request.Model}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"preflight-1","model":"owner/model-v1","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"verify_pulse_patrol","arguments":"{\"ok\":true}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()

	service := newEnabledCodexLBGatewayServiceForTest(t, server.URL+"/v1")
	result := service.RunPatrolToolPreflight(context.Background(), config.AIProviderCodexLBGateway, "owner/model-v1")
	if !result.Success || !result.ToolCallObserved || result.Provider != config.AIProviderCodexLBGateway {
		t.Fatalf("gateway Patrol preflight = success %v tool_call %v provider %q summary %q", result.Success, result.ToolCallObserved, result.Provider, result.Summary)
	}
	got := <-seen
	if got.model != "owner/model-v1" || got.authorization != "Bearer gateway-test-secret" {
		t.Fatalf("preflight request model/auth = %q/%q; want selected model and gateway credential", got.model, got.authorization)
	}
}

func TestCodexLBGatewayPatrolReadinessUsesStreamingToolsAndMultiTurnContinuation(t *testing.T) {
	var streamRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gateway-test-secret" {
			t.Errorf("gateway Authorization = %q, want configured credential", got)
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"id":"owner/model-v1"}]}`)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		streamRequests.Add(1)
		var request struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode readiness request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if !request.Stream {
			t.Errorf("readiness request was not streamed")
		}
		var toolName string
		var arguments map[string]interface{}
		var toolResult struct {
			FindingID string `json:"finding_id"`
			Nonce     string `json:"nonce"`
		}
		for _, message := range request.Messages {
			if message.Role == "tool" && strings.Contains(message.Content, "finding_id") {
				_ = json.Unmarshal([]byte(message.Content), &toolResult)
			}
			if message.Role != "user" || message.Content == "" {
				continue
			}
			prompt := message.Content
			var nonce string
			switch {
			case strings.Contains(prompt, "resource_id=node-protocol"):
				toolName = patrolReadinessObservationTool
				nonce = strings.SplitN(strings.SplitN(prompt, "nonce=", 2)[1], ".", 2)[0]
				arguments = map[string]interface{}{
					"resource_id": "node-protocol", "category": "reliability", "severity": "warning",
					"evidence_token": "protocol-" + nonce, "nonce": nonce,
				}
			case strings.Contains(prompt, "resource_id=pbs-lab-a"):
				toolName = patrolReadinessObservationTool
				nonce = strings.SplitN(strings.SplitN(prompt, "Evaluation nonce: ", 2)[1], "\n", 2)[0]
				arguments = map[string]interface{}{
					"resource_id": "pbs-lab-a", "category": "backup", "severity": "critical",
					"evidence_token": "backup-failed-201", "nonce": nonce,
				}
			case strings.Contains(prompt, "resource_id=storage-lab-b"):
				toolName = patrolReadinessObservationTool
				nonce = strings.SplitN(strings.SplitN(prompt, "Evaluation nonce: ", 2)[1], "\n", 2)[0]
				arguments = map[string]interface{}{
					"resource_id": "storage-lab-b", "category": "capacity", "severity": "warning",
					"evidence_token": "storage-91-percent", "nonce": nonce,
				}
			}
		}

		if toolResult.FindingID != "" {
			toolName = patrolReadinessReceiptTool
			arguments = map[string]interface{}{
				"accepted":   true,
				"finding_id": toolResult.FindingID,
				"nonce":      toolResult.Nonce,
			}
		} else if toolName == "" || arguments == nil {
			t.Errorf("could not identify readiness fixture in request")
			arguments = map[string]interface{}{}
		}
		encodedArguments, _ := json.Marshal(arguments)
		chunk, _ := json.Marshal(map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{map[string]interface{}{
						"index": 0,
						"id":    "readiness-call-1",
						"type":  "function",
						"function": map[string]interface{}{
							"name":      toolName,
							"arguments": string(encodedArguments),
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
		})
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()

	persistence := config.NewConfigPersistence(t.TempDir())
	manager := forkfeatures.NewManager(persistence)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable Codex-LB capability: %v", err)
	}
	persistence.SetForkProfileManager(manager)
	if err := persistence.SaveAIConfig(config.AIConfig{
		Enabled:                       true,
		Model:                         "codex_lb_gateway:owner/model-v1",
		PatrolModel:                   "codex_lb_gateway:owner/model-v1",
		CodexLBGatewayEndpoint:        server.URL + "/v1",
		CodexLBGatewayAPIKey:          "gateway-test-secret",
		CodexLBGatewayAllowPrivateURL: true,
	}); err != nil {
		t.Fatalf("SaveAIConfig(): %v", err)
	}
	service := NewService(persistence, nil)
	if err := service.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig(): %v", err)
	}
	result := service.RunPatrolModelReadiness(context.Background(), "", "")
	if !result.Success || !result.PatrolCapable || result.Provider != config.AIProviderCodexLBGateway {
		t.Fatalf("gateway readiness = success %v capable %v provider %q summary %q details %v", result.Success, result.PatrolCapable, result.Provider, result.Summary, result.Details)
	}
	if result.Dimensions.ToolProtocol.Passed != 3 || !result.Dimensions.ToolProtocol.ContinuationObserved || result.Dimensions.ContextQuality.Passed != 2 {
		t.Fatalf("gateway tool/context qualification = %+v", result.Dimensions)
	}
	if got := streamRequests.Load(); got != 4 {
		t.Fatalf("gateway streaming request count = %d, want 3 readiness probes plus one continuation", got)
	}
}
