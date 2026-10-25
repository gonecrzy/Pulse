// Package komodo contains the read-only Komodo Core v2 API client used by the
// Phase 2 monitoring provider.
package komodo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultTimeout   = 10 * time.Second
	maxTimeout       = 30 * time.Second
	defaultPageSize  = 100
	maxPageSize      = 500
	maxPages         = 1000
	maxResponseBytes = 8 << 20
	maxRequestBytes  = 1 << 20
)

// ClientConfig contains the only values needed to connect to a Core API.
type ClientConfig struct {
	Endpoint  string
	APIKey    string
	APISecret string
	Timeout   time.Duration
}

// ResourceKind is a Komodo resource type accepted by the inventory API.
type ResourceKind string

const (
	KindServer     ResourceKind = "Server"
	KindStack      ResourceKind = "Stack"
	KindDeployment ResourceKind = "Deployment"
	KindSwarm      ResourceKind = "Swarm"
)

var resourceKinds = []ResourceKind{KindServer, KindStack, KindDeployment, KindSwarm}

var readMethods = map[string]struct{}{
	"GetVersion":      {},
	"ListPermissions": {},
	"GetPermission":   {},
	"ListServers":     {},
	"ListStacks":      {},
	"ListDeployments": {},
	"ListSwarms":      {},
}

var coreVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?$`)

// ReadMethodAllowlist returns the method names that can be sent by this
// client. All requests use Komodo's POST /read/<method> API.
func ReadMethodAllowlist() []string {
	methods := make([]string, 0, len(readMethods))
	for method := range readMethods {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

// Client is intentionally read-only: it exposes no write, execute, terminal,
// or action API.
type Client struct {
	endpoint   *url.URL
	apiKey     string
	apiSecret  string
	httpClient *http.Client
	pageSize   int
}

// NewClient creates a bounded HTTPS client with normal certificate
// verification and redirects disabled so API credentials cannot be forwarded
// to a second host.
func NewClient(config ClientConfig) (*Client, error) {
	return newClientWithTransport(config, nil)
}

func newClientWithTransport(config ClientConfig, transport http.RoundTripper) (*Client, error) {
	endpoint, err := validateEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("Komodo API key is required")
	}
	if strings.TrimSpace(config.APISecret) == "" {
		return nil, errors.New("Komodo API secret is required")
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: timeout,
			ExpectContinueTimeout: time.Second,
			MaxIdleConns:          20,
			MaxIdleConnsPerHost:   4,
			IdleConnTimeout:       60 * time.Second,
		}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{endpoint: endpoint, apiKey: config.APIKey, apiSecret: config.APISecret, httpClient: client, pageSize: defaultPageSize}, nil
}

func validateEndpoint(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("Komodo endpoint must be a valid HTTPS URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, errors.New("Komodo endpoint must use HTTPS")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Komodo endpoint must be an HTTPS origin without credentials, path, query, or fragment")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	return parsed, nil
}

// PermissionScope distinguishes successful read-method access from evidence
// about the credential's additional Komodo-specific permissions. The Pulse
// client remains read-only regardless of those upstream permissions.
type PermissionScope struct {
	Verified                      []ResourceKind `json:"verified"` // Allowlisted inventory methods returned successfully.
	SpecificPermissionsVerified   []ResourceKind `json:"specificPermissionsVerified,omitempty"`
	SpecificPermissionsUnverified []ResourceKind `json:"specificPermissionsUnverified,omitempty"`
}

// Resource is the allowlisted, normalized subset of a Komodo list item.
// Fields omitted by this model (configuration, environment, secrets, errors,
// tags and raw API payloads) are never exposed to callers.
type Resource struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Kind      ResourceKind `json:"kind"`
	State     string       `json:"state,omitempty"`
	ServerID  string       `json:"serverId,omitempty"`
	SwarmID   string       `json:"swarmId,omitempty"`
	ServerIDs []string     `json:"serverIds,omitempty"`
}

// Inventory contains the Core version and only allowlisted resource facts.
type Inventory struct {
	Version     string          `json:"version"`
	Servers     []Resource      `json:"servers"`
	Stacks      []Resource      `json:"stacks"`
	Deployments []Resource      `json:"deployments"`
	Swarms      []Resource      `json:"swarms"`
	Scope       PermissionScope `json:"scope"`
}

// Count returns the number of inventoried resources.
func (i Inventory) Count() int {
	return len(i.Servers) + len(i.Stacks) + len(i.Deployments) + len(i.Swarms)
}

type versionResponse struct {
	Version string `json:"version"`
}
type permissionGrant struct {
	ResourceTarget struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"resource_target"`
	Level    string   `json:"level"`
	Specific []string `json:"specific"`
}
type permissionResponse struct {
	Level         string   `json:"level"`
	Specific      []string `json:"specific"`
	specificKnown bool
}

// UnmarshalJSON accepts the structured response declared by the Core v2
// client model and the level-only enum observed from the live v2.3.4 Core.
// A level-only response verifies the base level but cannot attest that
// effective specific permissions are empty. Inventory collection records
// that uncertainty separately; the live qualification harness requires its
// existing operator attestation before reporting a pass.
func (p *permissionResponse) UnmarshalJSON(data []byte) error {
	*p = permissionResponse{}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return errors.New("empty permission response")
	}
	if trimmed[0] == '"' {
		if err := json.Unmarshal(trimmed, &p.Level); err != nil {
			return errors.New("invalid level-only permission response")
		}
		return nil
	}
	if trimmed[0] != '{' {
		return errors.New("permission response must be a string or object")
	}

	fields, err := decodePermissionObject(trimmed)
	if err != nil {
		return errors.New("invalid structured permission response")
	}
	if len(fields) != 2 {
		return errors.New("structured permission response must contain level and specific")
	}
	level, hasLevel := fields["level"]
	specific, hasSpecific := fields["specific"]
	if !hasLevel || !hasSpecific || bytes.Equal(bytes.TrimSpace(level), []byte("null")) || bytes.Equal(bytes.TrimSpace(specific), []byte("null")) {
		return errors.New("structured permission response must contain non-null level and specific")
	}
	if err := json.Unmarshal(level, &p.Level); err != nil {
		return errors.New("structured permission level must be a string")
	}
	if err := json.Unmarshal(specific, &p.Specific); err != nil || p.Specific == nil {
		return errors.New("structured permission specifics must be an array of strings")
	}
	p.specificKnown = true
	return nil
}

func decodePermissionObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("invalid permission object")
	}
	fields := make(map[string]json.RawMessage, 2)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid permission object key")
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("invalid permission object key")
		}
		if _, exists := fields[key]; exists {
			return nil, errors.New("duplicate permission object key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("invalid permission object value")
		}
		fields[key] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return nil, errors.New("unterminated permission object")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing permission response data")
	}
	return fields, nil
}

type apiListItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Info struct {
		State     string   `json:"state"`
		ServerID  string   `json:"server_id"`
		SwarmID   string   `json:"swarm_id"`
		ServerIDs []string `json:"server_ids"`
	} `json:"info"`
}

// CollectInventory verifies the running Core major version, the caller's
// access scope, and all four supported resource lists.
func (c *Client) CollectInventory(ctx context.Context) (Inventory, error) {
	var inventory Inventory
	if c == nil {
		return inventory, errors.New("Komodo client is required")
	}
	var version versionResponse
	if err := c.call(ctx, "GetVersion", struct{}{}, &version); err != nil {
		return inventory, err
	}
	if !supportedCoreVersion(version.Version) {
		return inventory, errors.New("unsupported Komodo Core API version")
	}
	inventory.Version = version.Version
	var grants []permissionGrant
	if err := c.call(ctx, "ListPermissions", struct{}{}, &grants); err != nil {
		return Inventory{}, err
	}
	for _, grant := range grants {
		if !permissionSatisfiesRead(grant.Level) || len(grant.Specific) > 0 {
			return Inventory{}, errors.New("Komodo credential has permissions beyond supported read-only inventory")
		}
	}
	for _, kind := range resourceKinds {
		items, err := c.listKind(ctx, kind)
		if err != nil {
			return Inventory{}, err
		}
		specificPermissionsKnown := len(items) > 0
		if len(items) == 0 {
			// A successful List* response verifies that this principal can call
			// the inventory method. With no resource IDs, Core provides no
			// per-resource effective permission target to inspect. Direct grants
			// cannot prove that groups or inherited permissions add no specifics.
		} else {
			for _, item := range items {
				specificKnown, err := c.verifyEffectiveReadPermission(ctx, kind, item.ID)
				if err != nil {
					return Inventory{}, err
				}
				if !specificKnown {
					specificPermissionsKnown = false
				}
			}
		}
		inventory.Scope.Verified = append(inventory.Scope.Verified, kind)
		if specificPermissionsKnown {
			inventory.Scope.SpecificPermissionsVerified = append(inventory.Scope.SpecificPermissionsVerified, kind)
		} else {
			inventory.Scope.SpecificPermissionsUnverified = append(inventory.Scope.SpecificPermissionsUnverified, kind)
		}
		switch kind {
		case KindServer:
			inventory.Servers = items
		case KindStack:
			inventory.Stacks = items
		case KindDeployment:
			inventory.Deployments = items
		case KindSwarm:
			inventory.Swarms = items
		}
	}
	return inventory, nil
}

func (c *Client) verifyEffectiveReadPermission(ctx context.Context, kind ResourceKind, targetID string) (bool, error) {
	var permission permissionResponse
	err := c.call(ctx, "GetPermission", struct {
		Target struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"target"`
	}{Target: struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{Type: string(kind), ID: targetID}}, &permission)
	if err != nil {
		return false, err
	}
	return validateEffectiveReadPermission(kind, permission)
}

func validateEffectiveReadPermission(kind ResourceKind, permission permissionResponse) (bool, error) {
	if !permissionSatisfiesRead(permission.Level) {
		return false, fmt.Errorf("Komodo read scope for %s could not be verified", kind)
	}
	if !permission.specificKnown {
		return false, nil
	}
	if len(permission.Specific) > 0 {
		return false, fmt.Errorf("Komodo effective specific permissions for %s exceed the supported read-only scope", kind)
	}
	return true, nil
}

func (c *Client) listKind(ctx context.Context, kind ResourceKind) ([]Resource, error) {
	method := "List" + string(kind) + "s"
	if kind == KindDeployment {
		method = "ListDeployments"
	}
	seen := make(map[string]struct{})
	resources := make([]Resource, 0)
	for page := 0; page < maxPages; page++ {
		var items []apiListItem
		request := struct {
			Query map[string]any `json:"query"`
			Page  int            `json:"page"`
			Limit int            `json:"limit"`
		}{Query: map[string]any{}, Page: page, Limit: c.pageSize}
		if err := c.call(ctx, method, request, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				return nil, fmt.Errorf("Komodo %s list returned a resource without an ID", kind)
			}
			if _, ok := seen[id]; ok {
				return nil, fmt.Errorf("Komodo %s list returned a duplicate resource ID", kind)
			}
			seen[id] = struct{}{}
			resources = append(resources, Resource{
				ID:        id,
				Name:      safeDisplayName(item.Name, id),
				Kind:      kind,
				State:     safeState(item.Info.State),
				ServerID:  safeNativeID(item.Info.ServerID),
				SwarmID:   safeNativeID(item.Info.SwarmID),
				ServerIDs: safeNativeIDs(item.Info.ServerIDs),
			})
		}
		if len(items) < c.pageSize {
			return resources, nil
		}
	}
	return nil, errors.New("Komodo inventory exceeded the bounded pagination limit")
}

func (c *Client) call(ctx context.Context, method string, requestBody any, responseBody any) error {
	if _, allowed := readMethods[method]; !allowed {
		return errors.New("Komodo API method is not allowlisted")
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil || len(encoded) > maxRequestBytes {
		return errors.New("Komodo API request could not be encoded")
	}
	requestURL := *c.endpoint
	requestURL.Path = "/read/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(encoded))
	if err != nil {
		return errors.New("Komodo API request could not be created")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("X-Api-Secret", c.apiSecret)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("Komodo %s request timeout or cancellation", method)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return fmt.Errorf("Komodo %s request timeout", method)
		}
		return fmt.Errorf("Komodo %s API request failed", method)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return errors.New("Komodo API response could not be read")
	}
	if len(data) > maxResponseBytes {
		return errors.New("Komodo API response exceeded the size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("Komodo %s authentication failed", method)
		case http.StatusForbidden:
			return fmt.Errorf("Komodo %s permission scope is insufficient", method)
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return fmt.Errorf("Komodo %s request timeout", method)
		default:
			return fmt.Errorf("Komodo %s upstream API request failed (HTTP %d)", method, resp.StatusCode)
		}
	}
	if responseBody == nil {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		// The method is from the client-side allowlist. Reporting that bounded
		// identifier makes compatibility failures actionable without exposing
		// any part of the response, which can contain sensitive configuration.
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			field := typeErr.Field
			if field == "" {
				field = "top-level"
			}
			return fmt.Errorf("Komodo %s response had an incompatible JSON type at field %s (received %s)", method, field, typeErr.Value)
		}
		return fmt.Errorf("Komodo %s response did not match the supported contract", method)
	}
	return nil
}

func supportedCoreVersion(version string) bool {
	parts := coreVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	return len(parts) == 4 && parts[1] == "2"
}

func permissionSatisfiesRead(level string) bool {
	return strings.EqualFold(strings.TrimSpace(level), "read")
}

func safeDisplayName(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
		return fallback
	}
	return value
}

func safeState(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 64 || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

func safeNativeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

func safeNativeIDs(values []string) []string {
	if len(values) > 256 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if id := safeNativeID(value); id != "" {
			out = append(out, id)
		}
	}
	return out
}
