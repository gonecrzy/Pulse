package komodo

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const qualificationHTTPReadEndpoint = "http://10.1.10.101:9120"

// validateQualificationEndpoint accepts production HTTPS origins and the one
// explicit LAN HTTP endpoint supplied for this opt-in qualification run. It
// is test-only and does not change production endpoint validation.
func validateQualificationEndpoint(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("qualification endpoint is invalid")
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return validateEndpoint(trimmed)
	}
	if !strings.EqualFold(parsed.Scheme, "http") || parsed.Host != "10.1.10.101:9120" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("qualification HTTP is limited to the operator-provided Core endpoint")
	}
	return &url.URL{Scheme: "http", Host: "10.1.10.101:9120"}, nil
}

// newLiveQualificationClient keeps the production client HTTPS-only. The
// explicitly opted-in live test can use the exact operator-provided LAN HTTP
// endpoint with a direct, bounded transport that does not route credentials
// through an environment-configured proxy.
func newLiveQualificationClient(config ClientConfig) (*Client, error) {
	endpoint, err := validateQualificationEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if endpoint.Scheme == "https" {
		return NewClient(config)
	}

	secureConfig := config
	secureConfig.Endpoint = "https://qualification-http-endpoint.invalid"
	client, err := NewClient(secureConfig)
	if err != nil {
		return nil, err
	}
	client.endpoint = endpoint
	client.httpClient.Transport = &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: client.httpClient.Timeout,
		ExpectContinueTimeout: time.Second,
		MaxIdleConns:          2,
		MaxIdleConnsPerHost:   1,
		IdleConnTimeout:       15 * time.Second,
	}
	return client, nil
}

func TestQualificationHTTPEndpointIsNarrowlyAllowlisted(t *testing.T) {
	valid := []string{
		"https://komodo.example.test",
		qualificationHTTPReadEndpoint,
		qualificationHTTPReadEndpoint + "/",
	}
	for _, endpoint := range valid {
		if _, err := validateQualificationEndpoint(endpoint); err != nil {
			t.Errorf("validateQualificationEndpoint(%q): %v", endpoint, err)
		}
	}

	invalid := []string{
		"http://10.1.10.102:9120",
		"http://10.1.10.101:9121",
		"http://komodo.home.goneczy.com:9120",
		qualificationHTTPReadEndpoint + "/read/GetVersion",
		qualificationHTTPReadEndpoint + "?redirect=https://example.test",
		"http://user:pass@10.1.10.101:9120",
	}
	for _, endpoint := range invalid {
		if _, err := validateQualificationEndpoint(endpoint); err == nil {
			t.Errorf("validateQualificationEndpoint(%q) unexpectedly succeeded", endpoint)
		}
	}
	if _, err := NewClient(ClientConfig{Endpoint: qualificationHTTPReadEndpoint, APIKey: "key", APISecret: "secret"}); err == nil {
		t.Fatal("production client accepted the HTTP qualification endpoint")
	}
}

func TestLiveQualificationHTTPClientIsDirectAndBounded(t *testing.T) {
	client, err := newLiveQualificationClient(ClientConfig{
		Endpoint:  qualificationHTTPReadEndpoint,
		APIKey:    "fixture-key",
		APISecret: "fixture-secret",
	})
	if err != nil {
		t.Fatalf("construct qualification client: %v", err)
	}
	if client.endpoint.String() != qualificationHTTPReadEndpoint {
		t.Fatalf("client endpoint = %q, want exact approved endpoint", client.endpoint.String())
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("qualification transport = %T, want bounded HTTP transport", client.httpClient.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("HTTP qualification transport must not forward credentials through an environment proxy")
	}
	if transport.ResponseHeaderTimeout <= 0 || client.httpClient.Timeout <= 0 || client.httpClient.Timeout > maxTimeout {
		t.Fatalf("qualification transport is not bounded: header=%s total=%s", transport.ResponseHeaderTimeout, client.httpClient.Timeout)
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS verification settings must remain secure even though this endpoint uses plain HTTP")
	}
}
