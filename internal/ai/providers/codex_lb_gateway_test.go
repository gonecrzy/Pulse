package providers

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
)

func codexLBGatewayTestConfig(t *testing.T, endpoint string, allowPrivate bool) (*config.ConfigPersistence, *config.AIConfig) {
	t.Helper()
	persistence := config.NewConfigPersistence(t.TempDir())
	manager := forkfeatures.NewManager(persistence)
	profile := forkfeatures.DefaultProfile()
	profile.EnabledFeatures = append(profile.EnabledFeatures, forkfeatures.FeatureCodexLBGateway)
	if _, err := manager.Replace(profile, profile.Revision); err != nil {
		t.Fatalf("enable gateway capability: %v", err)
	}
	persistence.SetForkProfileManager(manager)
	if err := persistence.SaveAIConfig(config.AIConfig{
		Enabled:                       true,
		Model:                         "codex_lb_gateway:owner/model-v1",
		CodexLBGatewayEndpoint:        endpoint,
		CodexLBGatewayAPIKey:          "gateway-test-secret",
		CodexLBGatewayAllowPrivateURL: allowPrivate,
		OpenAIAPIKey:                  "legacy-openai-secret",
		AnthropicAPIKey:               "legacy-claude-secret",
		CodexSubscriptionEnabled:      true,
	}); err != nil {
		t.Fatalf("save gateway config: %v", err)
	}
	cfg, err := persistence.LoadAIConfig()
	if err != nil {
		t.Fatalf("load gateway config: %v", err)
	}
	return persistence, cfg
}

func TestCodexLBGatewayUsesExistingOpenAICompatibleModelsAndChatTransport(t *testing.T) {
	var seenAuth string
	var seenModel string
	var seenTools int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"owner/model-v1","name":"Model V1"}]}`))
		case "/v1/chat/completions":
			var payload struct {
				Model string `json:"model"`
				Tools []any  `json:"tools"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode Chat Completions request: %v", err)
			}
			seenModel, seenTools = payload.Model, len(payload.Tools)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"chat-1","object":"chat.completion","model":"owner/model-v1","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"pulse_query","arguments":"{\"resource_id\":\"vm:42\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":21,"completion_tokens":4,"total_tokens":25}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, cfg := codexLBGatewayTestConfig(t, server.URL+"/v1", true)
	provider, err := NewForModel(cfg, cfg.GetModel())
	if err != nil {
		t.Fatalf("NewForModel(): %v", err)
	}
	models, err := provider.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "owner/model-v1" {
		t.Fatalf("ListModels() = %#v, %v", models, err)
	}
	response, err := provider.Chat(context.Background(), ChatRequest{
		Model:    cfg.GetModel(),
		Messages: []Message{{Role: "user", Content: "inspect the resource"}},
		Tools:    []Tool{{Name: "pulse_query", Type: "function", InputSchema: map[string]interface{}{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("Chat(): %v", err)
	}
	if seenAuth != "Bearer gateway-test-secret" {
		t.Fatalf("Authorization = %q, want gateway credential", seenAuth)
	}
	if seenModel != "owner/model-v1" || seenTools != 1 {
		t.Fatalf("request model/tools = %q/%d, want selected model and one tool", seenModel, seenTools)
	}
	if response.InputTokens != 21 || response.OutputTokens != 4 {
		t.Fatalf("usage = input %d output %d, want provider usage buckets", response.InputTokens, response.OutputTokens)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].ID != "call-1" || response.ToolCalls[0].Name != "pulse_query" {
		t.Fatalf("tool calls = %+v, want parsed gateway tool call", response.ToolCalls)
	}
	if strings.Contains(fmt.Sprint(response), "gateway-test-secret") {
		t.Fatal("provider response leaked the gateway credential")
	}
}

func TestCodexLBGatewayParsesIncrementalStreamingToolArgumentsAndRejectsModelOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-stream-1\",\"type\":\"function\",\"function\":{\"name\":\"pulse_query\",\"arguments\":\"{\\\"resource_id\\\":\"}}]},\"finish_reason\":null}]}\n\n"+
				"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"vm:42\\\"}\"}}]},\"finish_reason\":null}]}\n\n"+
				"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"+
				"data: [DONE]\n\n")
	}))
	defer server.Close()

	_, cfg := codexLBGatewayTestConfig(t, server.URL+"/v1", true)
	provider, err := NewForModel(cfg, cfg.GetModel())
	if err != nil {
		t.Fatalf("NewForModel(): %v", err)
	}
	streaming, ok := provider.(StreamingProvider)
	if !ok {
		t.Fatal("Codex-LB provider does not implement streaming")
	}
	var done *DoneEvent
	err = streaming.ChatStream(context.Background(), ChatRequest{
		Model:    cfg.GetModel(),
		Messages: []Message{{Role: "user", Content: "inspect vm 42"}},
		Tools:    []Tool{{Name: "pulse_query", Type: "function", InputSchema: map[string]interface{}{"type": "object"}}},
	}, func(event StreamEvent) {
		if event.Type == "done" {
			if result, ok := event.Data.(DoneEvent); ok {
				done = &result
			}
		}
	})
	if err != nil {
		t.Fatalf("ChatStream(): %v", err)
	}
	if done == nil || len(done.ToolCalls) != 1 || done.ToolCalls[0].ID != "call-stream-1" {
		t.Fatalf("streamed done event = %+v, want assembled tool call", done)
	}
	if got := done.ToolCalls[0].Input["resource_id"]; got != "vm:42" {
		t.Fatalf("assembled tool arguments = %#v, want resource_id vm:42", done.ToolCalls[0].Input)
	}
	if err := streaming.ChatStream(context.Background(), ChatRequest{
		Model:    "codex_lb_gateway:owner/other-model",
		Messages: []Message{{Role: "user", Content: "must reject before request"}},
	}, func(StreamEvent) {}); err == nil {
		t.Fatal("ChatStream accepted a model override under the gateway-only client")
	}
}

func TestCodexLBGatewayRetriesOneTransientStreamingRateLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"error":{"message":"temporarily rate limited"}}`, http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"ready\"},\"finish_reason\":null}]}\n\n"+
				"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"+
				"data: [DONE]\n\n")
	}))
	defer server.Close()

	client := newCodexLBGatewayClient("gateway-test-secret", "owner/model-v1", server.URL+"/v1", true, 0)
	var completed bool
	err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "owner/model-v1",
		Messages: []Message{{Role: "user", Content: "qualify the gateway"}},
	}, func(event StreamEvent) {
		if event.Type == "done" {
			completed = true
		}
	})
	if err != nil {
		t.Fatalf("ChatStream(): %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("stream startup requests = %d, want one retry after the 429", got)
	}
	if !completed {
		t.Fatal("stream did not complete after the gateway recovered from rate limiting")
	}
}

func TestCodexLBGatewayRejectsDirectProvidersAndUntrustedLocalEndpoint(t *testing.T) {
	_, cfg := codexLBGatewayTestConfig(t, "https://gateway.example/v1", false)
	if _, err := NewForProvider(cfg, config.AIProviderOpenAI, "gpt-4o"); err == nil {
		t.Fatal("direct OpenAI route was accepted while Codex-LB capability was active")
	}
	if _, err := NewForProvider(cfg, config.AIProviderCodexSubscription, "gpt-5.6-luna"); err == nil {
		t.Fatal("subscription-agent route was accepted while Codex-LB capability was active")
	}
	if _, err := newCodexLBGatewayClient("key", "model", "http://127.0.0.1:9000/v1", false, 0).ListModels(context.Background()); err == nil {
		t.Fatal("gateway client reached a local endpoint without explicit private-network opt-in")
	}
}

func TestCodexLBGatewayRejectsGenericConstructorAndReportsUnreachableGateway(t *testing.T) {
	var genericRequests int
	genericServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		genericRequests++
		w.WriteHeader(http.StatusOK)
	}))
	defer genericServer.Close()

	generic := NewOpenAICompatibleClient(config.AIProviderCodexLBGateway, "key", "owner/model", genericServer.URL+"/v1", 0)
	if _, err := generic.ListModels(context.Background()); err == nil {
		t.Fatal("generic OpenAI-compatible constructor bypassed Codex-LB provider admission")
	}
	if genericRequests != 0 {
		t.Fatalf("generic constructor made %d outbound requests before gateway admission", genericRequests)
	}

	unreachable := httptest.NewServer(http.NotFoundHandler())
	endpoint := unreachable.URL + "/v1"
	unreachable.Close()
	client := newCodexLBGatewayClient("key", "owner/model", endpoint, true, 0)
	if _, err := client.ListModels(context.Background()); err == nil {
		t.Fatal("unreachable gateway unexpectedly served a model catalog")
	}
}

func TestCodexLBGatewayRedactsAuthenticationErrorsAndRejectsMalformedStream(t *testing.T) {
	const secret = "gateway-test-secret"
	var streamRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Stream {
			streamRequests++
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {not-json}\n\n")
			return
		}
		http.Error(w, `{"error":{"message":"unauthorized token `+secret+`"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, cfg := codexLBGatewayTestConfig(t, server.URL+"/v1", true)
	provider, err := NewForModel(cfg, cfg.GetModel())
	if err != nil {
		t.Fatalf("NewForModel(): %v", err)
	}
	if _, err := provider.Chat(context.Background(), ChatRequest{Model: cfg.GetModel()}); err == nil {
		t.Fatal("Chat() accepted gateway authentication failure")
	} else if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("gateway error did not redact credential: %v", err)
	}
	streaming, ok := provider.(StreamingProvider)
	if !ok {
		t.Fatal("Codex-LB provider does not implement streaming")
	}
	if err := streaming.ChatStream(context.Background(), ChatRequest{Model: cfg.GetModel()}, func(StreamEvent) {}); err == nil {
		t.Fatal("ChatStream() accepted malformed SSE JSON")
	}
	if streamRequests != 1 {
		t.Fatalf("malformed stream requests = %d, want 1", streamRequests)
	}
}
