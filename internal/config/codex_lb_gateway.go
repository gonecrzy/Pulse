package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
)

// SetForkProfileManager attaches the installation-wide capability source to a
// loaded AI config. The manager is not serialized; provider construction reads
// its current snapshot so a profile change takes effect without rebuilding a
// tenant config object.
func (c *AIConfig) SetForkProfileManager(manager *forkfeatures.Manager) {
	if c == nil {
		return
	}
	c.forkProfileManager = manager
}

// CodexLBGatewayPolicyState reports whether the local exclusive-gateway
// capability is available. A nil manager means this AI config was constructed
// outside the application runtime and has no fork policy attached.
func (c *AIConfig) CodexLBGatewayPolicyState() (bool, error) {
	if c == nil || c.forkProfileManager == nil {
		return false, nil
	}
	return c.forkProfileManager.Available(forkfeatures.FeatureCodexLBGateway)
}

func (c *AIConfig) CodexLBGatewayPolicyEnabled() bool {
	enabled, err := c.CodexLBGatewayPolicyState()
	return err == nil && enabled
}

// ValidateProviderRoute is the backend provider-construction boundary. When
// the fork capability is active, every inference route must use the dedicated
// Codex-LB provider and the one explicitly selected model. An empty model is
// allowed only for model discovery, which makes no inference request.
func (c *AIConfig) ValidateProviderRoute(provider, model string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	if c == nil {
		return fmt.Errorf("Pulse Intelligence config is unavailable")
	}
	if c.forkProfileManager == nil {
		if provider == AIProviderCodexLBGateway {
			return fmt.Errorf("Codex-LB gateway capability policy is unavailable")
		}
		return nil
	}
	available, err := c.forkProfileManager.Available(forkfeatures.FeatureCodexLBGateway)
	if err != nil {
		return fmt.Errorf("fork gateway capability policy is unavailable")
	}
	if !available {
		if provider == AIProviderCodexLBGateway {
			return fmt.Errorf("fork.codex_lb_gateway capability is unavailable")
		}
		return nil
	}
	if provider != AIProviderCodexLBGateway {
		return fmt.Errorf("provider %q is disabled while fork.codex_lb_gateway is enabled", provider)
	}
	if strings.TrimSpace(c.CodexLBGatewayEndpoint) == "" || strings.TrimSpace(c.CodexLBGatewayAPIKey) == "" {
		return fmt.Errorf("Codex-LB gateway endpoint and credential must be configured")
	}
	if err := ValidateCodexLBGatewayEndpoint(c.CodexLBGatewayEndpoint, c.CodexLBGatewayAllowPrivateURL); err != nil {
		return fmt.Errorf("invalid Codex-LB gateway endpoint: %w", err)
	}
	if model == "" {
		return nil
	}
	requestedModel := model
	if prefixEnd := strings.IndexByte(model, ':'); prefixEnd > 0 {
		modelProvider := strings.ToLower(strings.TrimSpace(model[:prefixEnd]))
		if _, knownProvider := LookupAIProviderDefinition(modelProvider); knownProvider {
			if modelProvider != AIProviderCodexLBGateway {
				return fmt.Errorf("model route %q is not a Codex-LB gateway model", model)
			}
			requestedModel = strings.TrimSpace(model[prefixEnd+1:])
		}
	}
	selectedModels := make(map[string]struct{})
	for _, selected := range []string{c.GetModel(), c.GetChatModel(), c.GetPatrolModel(), c.GetAutoFixModel()} {
		selectedProvider, selectedModel := ParseModelString(selected)
		if selectedProvider == AIProviderCodexLBGateway && strings.TrimSpace(selectedModel) != "" {
			selectedModels[selectedModel] = struct{}{}
		}
	}
	if len(selectedModels) == 0 {
		return fmt.Errorf("select a Codex-LB gateway model before running AI")
	}
	if _, selected := selectedModels[requestedModel]; !selected {
		return fmt.Errorf("requested model %q is not an explicitly selected Codex-LB gateway model", requestedModel)
	}
	return nil
}

// ValidateCodexLBGatewayEndpoint performs non-network validation before an
// endpoint is persisted. The HTTP client repeats resolved-IP checks at each
// request to prevent DNS rebinding and blocks metadata/link-local destinations.
// Private or loopback routes require the separate explicit opt-in.
func ValidateCodexLBGatewayEndpoint(raw string, allowPrivate bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("endpoint is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("endpoint must be an absolute URL without user information, query, or fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "metadata.google.internal" || host == "metadata.goog" || host == "metadata.azure.internal" || host == "instance-data.ec2.internal" {
		return fmt.Errorf("metadata service host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.Equal(net.ParseIP("169.254.169.254")) ||
			ip.Equal(net.ParseIP("fd00:ec2::254")) ||
			ip.Equal(net.ParseIP("100.100.100.200")) ||
			ip.Equal(net.ParseIP("168.63.129.16")) ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("metadata, link-local, multicast, and unspecified destinations are not allowed")
		}
		if (ip.IsLoopback() || ip.IsPrivate()) && !allowPrivate {
			return fmt.Errorf("private and loopback destinations require explicit opt-in")
		}
	} else if !allowPrivate && (host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local")) {
		return fmt.Errorf("local destinations require explicit opt-in")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
	case "http":
		if !allowPrivate {
			return fmt.Errorf("HTTPS is required unless private gateway access is explicitly allowed")
		}
		ip := net.ParseIP(host)
		if ip != nil {
			if !ip.IsLoopback() && !ip.IsPrivate() {
				return fmt.Errorf("HTTP is only allowed for loopback or private IP literals")
			}
		} else if host != "localhost" && !strings.HasSuffix(host, ".localhost") {
			return fmt.Errorf("HTTP is only allowed for localhost or loopback/private IP literals")
		}
	default:
		return fmt.Errorf("endpoint scheme must be http or https")
	}
	return nil
}
