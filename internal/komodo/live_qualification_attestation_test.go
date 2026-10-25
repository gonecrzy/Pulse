package komodo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQualificationAttestationRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("PULSE_KOMODO_LIVE_QUALIFICATION", "0")
	_, err := loadQualificationPermissionAttestation("", ClientConfig{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "explicitly opted in") {
		t.Fatalf("attestation loader without opt-in error = %v", err)
	}
}

func TestQualificationAttestationRejectsMissingFile(t *testing.T) {
	qualificationTestOptIn(t)
	_, err := loadQualificationPermissionAttestation("", qualificationTestClientConfig(), qualificationTestNow())
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing attestation error = %v", err)
	}
}

func TestQualificationAttestationRejectsEndpointAndCredentialMismatch(t *testing.T) {
	qualificationTestOptIn(t)
	now := qualificationTestNow()
	config := qualificationTestClientConfig()
	t.Run("endpoint", func(t *testing.T) {
		document := qualificationTestAttestation(config, now)
		document.Endpoint = "https://other.example.test"
		path := writeQualificationAttestationFixture(t, document, 0600)
		_, err := loadQualificationPermissionAttestation(path, config, now)
		if err == nil || !strings.Contains(err.Error(), "endpoint") {
			t.Fatalf("endpoint mismatch error = %v", err)
		}
	})
	t.Run("credential fingerprint", func(t *testing.T) {
		document := qualificationTestAttestation(config, now)
		document.CredentialFingerprint = strings.Repeat("0", len(document.CredentialFingerprint))
		path := writeQualificationAttestationFixture(t, document, 0600)
		_, err := loadQualificationPermissionAttestation(path, config, now)
		if err == nil || !strings.Contains(err.Error(), "credential") {
			t.Fatalf("credential mismatch error = %v", err)
		}
	})
}

func TestQualificationAttestationRejectsStaleAndFutureClaims(t *testing.T) {
	qualificationTestOptIn(t)
	now := qualificationTestNow()
	config := qualificationTestClientConfig()
	for _, test := range []struct {
		name     string
		verified time.Time
	}{
		{name: "stale", verified: now.Add(-qualificationAttestationMaxAge - time.Second)},
		{name: "future", verified: now.Add(qualificationAttestationFutureSkew + time.Second)},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := qualificationTestAttestation(config, test.verified)
			path := writeQualificationAttestationFixture(t, document, 0600)
			_, err := loadQualificationPermissionAttestation(path, config, now)
			if err == nil || !strings.Contains(err.Error(), "time") {
				t.Fatalf("%s attestation error = %v", test.name, err)
			}
		})
	}
}

func TestQualificationAttestationRequiresCanonicalUTCAndRejectsUnknownFields(t *testing.T) {
	qualificationTestOptIn(t)
	now := qualificationTestNow()
	config := qualificationTestClientConfig()
	t.Run("non-UTC timestamp spelling", func(t *testing.T) {
		document := qualificationTestAttestation(config, now)
		document.VerifiedAt = "2026-10-25T12:00:00+00:00"
		path := writeQualificationAttestationFixture(t, document, 0600)
		if _, err := loadQualificationPermissionAttestation(path, config, now); err == nil {
			t.Fatal("non-canonical UTC timestamp was accepted")
		}
	})
	t.Run("unknown field", func(t *testing.T) {
		document := qualificationTestAttestation(config, now)
		contents, err := json.Marshal(document)
		if err != nil {
			t.Fatal("encode attestation fixture")
		}
		contents = append(contents[:len(contents)-1], []byte(`,"unexpected":"value"}`)...)
		path := filepath.Join(t.TempDir(), "permission-attestation.json")
		if err := os.WriteFile(path, contents, 0600); err != nil {
			t.Fatal("write attestation fixture")
		}
		if _, err := loadQualificationPermissionAttestation(path, config, now); err == nil {
			t.Fatal("attestation with an unknown field was accepted")
		}
	})
}

func TestQualificationAttestationAcceptsCurrentBoundOperatorClaims(t *testing.T) {
	qualificationTestOptIn(t)
	now := qualificationTestNow()
	config := qualificationTestClientConfig()
	document := qualificationTestAttestation(config, now)
	path := writeQualificationAttestationFixture(t, document, 0600)

	got, err := loadQualificationPermissionAttestation(path, config, now)
	if err != nil {
		t.Fatalf("valid operator attestation rejected: %v", err)
	}
	if got.Endpoint != "https://komodo.example.test" || got.CredentialFingerprint != qualificationCredentialFingerprint(config) || !got.OperatorVerified {
		t.Fatal("accepted attestation does not preserve its bound operator-verification metadata")
	}
}

func TestQualificationAttestationRejectsIncompleteClaims(t *testing.T) {
	qualificationTestOptIn(t)
	now := qualificationTestNow()
	config := qualificationTestClientConfig()
	for _, test := range []struct {
		name  string
		clear func(*qualificationPermissionAttestation)
	}{
		{name: "principal non-admin", clear: func(a *qualificationPermissionAttestation) { a.PrincipalNonAdmin = false }},
		{name: "all exposed resources read", clear: func(a *qualificationPermissionAttestation) { a.AllExposedResourcesRead = false }},
		{name: "no effective specific permissions", clear: func(a *qualificationPermissionAttestation) { a.NoEffectiveSpecificPermissions = false }},
		{name: "no direct elevated grants", clear: func(a *qualificationPermissionAttestation) { a.NoElevatedDirectGrants = false }},
		{name: "no UserGroup elevated grants", clear: func(a *qualificationPermissionAttestation) { a.NoElevatedUserGroupGrants = false }},
		{name: "no Everyone elevated grants", clear: func(a *qualificationPermissionAttestation) { a.NoElevatedEveryoneGroupGrants = false }},
		{name: "no resource-default elevated grants", clear: func(a *qualificationPermissionAttestation) { a.NoElevatedResourceDefaults = false }},
		{name: "no inherited elevated grants", clear: func(a *qualificationPermissionAttestation) { a.NoElevatedInheritedPermissions = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := qualificationTestAttestation(config, now)
			test.clear(&document)
			path := writeQualificationAttestationFixture(t, document, 0600)
			if _, err := loadQualificationPermissionAttestation(path, config, now); err == nil {
				t.Fatal("attestation with an unverified permission claim was accepted")
			}
		})
	}
}

func TestQualificationCredentialFingerprintBindsEndpointAndCredentials(t *testing.T) {
	config := qualificationTestClientConfig()
	fingerprint := qualificationCredentialFingerprint(config)
	if fingerprint == "" || strings.Contains(fingerprint, config.APIKey) || strings.Contains(fingerprint, config.APISecret) {
		t.Fatal("credential fingerprint is empty or contains credential material")
	}
	changed := config
	changed.Endpoint = "https://other.example.test"
	if qualificationCredentialFingerprint(changed) == fingerprint {
		t.Fatal("credential fingerprint did not bind the endpoint")
	}
	changed = config
	changed.APIKey = "different-key"
	if qualificationCredentialFingerprint(changed) == fingerprint {
		t.Fatal("credential fingerprint did not bind the API key")
	}
	changed = config
	changed.APISecret = "different-secret"
	if qualificationCredentialFingerprint(changed) == fingerprint {
		t.Fatal("credential fingerprint did not bind the API secret")
	}
}

func TestQualificationAttestationRequiresOwnerOnlyFile(t *testing.T) {
	qualificationTestOptIn(t)
	now := qualificationTestNow()
	config := qualificationTestClientConfig()
	document := qualificationTestAttestation(config, now)
	path := writeQualificationAttestationFixture(t, document, 0644)
	if _, err := loadQualificationPermissionAttestation(path, config, now); err == nil {
		t.Fatal("group-readable attestation file was accepted")
	}
}

func TestQualificationPermissionTransportOnlyAdaptsAttestedReadString(t *testing.T) {
	for _, test := range []struct {
		name        string
		path        string
		status      int
		body        string
		attested    bool
		wantAdapted bool
		wantBody    string
		method      string
	}{
		{name: "attested level-only read", path: "/read/GetPermission", status: http.StatusOK, body: `"Read"`, attested: true, wantAdapted: true, wantBody: `{"level":"Read","specific":[]}`},
		{name: "unattested level-only read", path: "/read/GetPermission", status: http.StatusOK, body: `"Read"`, wantBody: `"Read"`},
		{name: "structured response", path: "/read/GetPermission", status: http.StatusOK, body: `{"level":"Read","specific":[]}`, attested: true, wantBody: `{"level":"Read","specific":[]}`},
		{name: "elevated level string", path: "/read/GetPermission", status: http.StatusOK, body: `"Write"`, attested: true, wantBody: `"Write"`},
		{name: "non-POST request", path: "/read/GetPermission", status: http.StatusOK, body: `"Read"`, attested: true, wantBody: `"Read"`, method: http.MethodGet},
		{name: "other method", path: "/read/ListServers", status: http.StatusOK, body: `"Read"`, attested: true, wantBody: `"Read"`},
		{name: "error response", path: "/read/GetPermission", status: http.StatusForbidden, body: `"Read"`, attested: true, wantBody: `"Read"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := qualificationRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body)), ContentLength: int64(len(test.body)), Request: request}, nil
			})
			transport := &qualificationPermissionTransport{base: base, attested: test.attested}
			method := test.method
			if method == "" {
				method = http.MethodPost
			}
			request, err := http.NewRequest(method, "https://komodo.example.test"+test.path, nil)
			if err != nil {
				t.Fatal("create fixture request")
			}
			response, err := transport.RoundTrip(request)
			if err != nil {
				t.Fatalf("qualification transport returned error: %v", err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal("read fixture response")
			}
			if string(body) != test.wantBody {
				t.Fatalf("body = %q, want %q", body, test.wantBody)
			}
			wantSeen := 0
			if test.wantAdapted {
				wantSeen = 1
			}
			if transport.seen != wantSeen {
				t.Fatalf("adapted count = %d, want %d", transport.seen, wantSeen)
			}
		})
	}
}

func TestQualificationAttestedReadAdapterStillRejectsElevatedPermissions(t *testing.T) {
	for _, response := range []string{`{"level":"Read","specific":["Terminal"]}`, `"Write"`} {
		t.Run(response, func(t *testing.T) {
			client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/read/ListServers" {
					writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
					return true
				}
				if r.URL.Path == "/read/GetPermission" {
					writeFixtureJSON(w, http.StatusOK, response)
					return true
				}
				return false
			}))
			client.httpClient.Transport = newQualificationPermissionTransport(client.httpClient.Transport, qualificationPermissionAttestation{OperatorVerified: true})
			_, err := client.CollectInventory(context.Background())
			if err == nil {
				t.Fatal("attested test adapter accepted an elevated structured permission")
			}
			if response == `{"level":"Read","specific":["Terminal"]}` && !strings.Contains(err.Error(), "specific permissions") {
				t.Fatalf("structured elevated specific permission was not rejected by normal validation: %v", err)
			}
			if response == `"Write"` && !strings.Contains(err.Error(), "scope") {
				t.Fatalf("elevated level-only response was not rejected by normal validation: %v", err)
			}
		})
	}
}

func TestQualificationAttestationEnablesLevelOnlyReadInventoryInOptInFixture(t *testing.T) {
	qualificationTestOptIn(t)
	handler := inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/read/ListServers":
			writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
			return true
		case "/read/GetPermission":
			writeFixtureJSON(w, http.StatusOK, `"Read"`)
			return true
		default:
			return false
		}
	})
	client, server := newFixtureClient(t, handler)
	config := ClientConfig{Endpoint: server.URL, APIKey: fixtureAPIKey, APISecret: fixtureAPISecret}
	file := qualificationTestAttestation(config, time.Now().UTC())
	path := writeQualificationAttestationFixture(t, file, 0600)
	attestation, err := loadQualificationPermissionAttestation(path, config, time.Now().UTC())
	if err != nil {
		t.Fatalf("load fixture attestation: %v", err)
	}
	transport := newQualificationPermissionTransport(client.httpClient.Transport, attestation)
	client.httpClient.Transport = transport
	inventory, err := client.CollectInventory(context.Background())
	if err != nil {
		t.Fatalf("opt-in attested fixture inventory failed: %v", err)
	}
	if inventory.Count() != 1 || len(inventory.Servers) != 1 || transport.seen != 1 {
		t.Fatalf("attested fixture result count=%d servers=%d adapted=%d; want one server and one adapted response", inventory.Count(), len(inventory.Servers), transport.seen)
	}
}

type qualificationRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip qualificationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func qualificationTestOptIn(t *testing.T) {
	t.Helper()
	t.Setenv("PULSE_KOMODO_LIVE_QUALIFICATION", "1")
}

func qualificationTestNow() time.Time {
	return time.Date(2026, time.October, 25, 12, 0, 0, 0, time.UTC)
}

func qualificationTestClientConfig() ClientConfig {
	return ClientConfig{Endpoint: "https://komodo.example.test", APIKey: "qualification-key", APISecret: "qualification-secret"}
}

func qualificationTestAttestation(config ClientConfig, verifiedAt time.Time) qualificationPermissionAttestation {
	return qualificationPermissionAttestation{
		Version:                        1,
		Endpoint:                       config.Endpoint,
		CredentialFingerprint:          qualificationCredentialFingerprint(config),
		VerifiedAt:                     verifiedAt.UTC().Format(time.RFC3339),
		PrincipalNonAdmin:              true,
		AllExposedResourcesRead:        true,
		NoEffectiveSpecificPermissions: true,
		NoElevatedDirectGrants:         true,
		NoElevatedUserGroupGrants:      true,
		NoElevatedEveryoneGroupGrants:  true,
		NoElevatedResourceDefaults:     true,
		NoElevatedInheritedPermissions: true,
	}
}

func writeQualificationAttestationFixture(t *testing.T, document qualificationPermissionAttestation, mode os.FileMode) string {
	t.Helper()
	contents, err := json.Marshal(document)
	if err != nil {
		t.Fatal("encode attestation fixture")
	}
	path := filepath.Join(t.TempDir(), "permission-attestation.json")
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal("write attestation fixture")
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal("set attestation fixture permissions")
	}
	return path
}
