package komodo

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	qualificationAttestationMaxAge     = 24 * time.Hour
	qualificationAttestationFutureSkew = 5 * time.Minute
	qualificationAttestationProtocol   = "pulse-komodo-qualification-permission-attestation-v1"
)

// qualificationPermissionAttestation is available only to the opt-in live
// test binary. Its JSON fields are operator assertions, not API evidence.
type qualificationPermissionAttestation struct {
	Version                        int    `json:"version"`
	Endpoint                       string `json:"endpoint"`
	CredentialFingerprint          string `json:"credentialFingerprint"`
	VerifiedAt                     string `json:"verifiedAt"`
	PrincipalNonAdmin              bool   `json:"principalNonAdmin"`
	AllExposedResourcesRead        bool   `json:"allExposedResourcesRead"`
	NoEffectiveSpecificPermissions bool   `json:"noEffectiveSpecificPermissions"`
	NoElevatedDirectGrants         bool   `json:"noElevatedDirectGrants"`
	NoElevatedUserGroupGrants      bool   `json:"noElevatedUserGroupGrants"`
	NoElevatedEveryoneGroupGrants  bool   `json:"noElevatedEveryoneGroupGrants"`
	NoElevatedResourceDefaults     bool   `json:"noElevatedResourceDefaults"`
	NoElevatedInheritedPermissions bool   `json:"noElevatedInheritedPermissions"`
	OperatorVerified               bool   `json:"-"`
}

// qualificationCredentialFingerprint creates a non-secret, endpoint-bound
// identity token. The API secret is used only as the HMAC key and never leaves
// memory or appears in the result.
func qualificationCredentialFingerprint(config ClientConfig) string {
	endpoint, err := validateQualificationEndpoint(config.Endpoint)
	if err != nil || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.APISecret) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(config.APISecret))
	_, _ = io.WriteString(mac, qualificationAttestationProtocol+"\n"+endpoint.String()+"\n"+config.APIKey)
	return hex.EncodeToString(mac.Sum(nil))
}

// loadQualificationPermissionAttestation reads a small owner-only file only
// after the live qualification test was explicitly opted into. No production
// credential validation calls this function.
func loadQualificationPermissionAttestation(path string, config ClientConfig, now time.Time) (qualificationPermissionAttestation, error) {
	if os.Getenv("PULSE_KOMODO_LIVE_QUALIFICATION") != "1" {
		return qualificationPermissionAttestation{}, errors.New("permission attestation requires an explicitly opted in test")
	}
	if strings.TrimSpace(path) == "" {
		return qualificationPermissionAttestation{}, errors.New("permission attestation is missing")
	}
	if _, err := validateQualificationEndpoint(config.Endpoint); err != nil || qualificationCredentialFingerprint(config) == "" {
		return qualificationPermissionAttestation{}, errors.New("qualification connection identity is invalid")
	}
	contents, err := readOwnerOnlyQualificationFile(path)
	if err != nil {
		return qualificationPermissionAttestation{}, err
	}
	attestation, err := decodeQualificationPermissionAttestation(contents)
	zeroQualificationBytes(contents)
	if err != nil {
		return qualificationPermissionAttestation{}, errors.New("permission attestation has an invalid format or incomplete claims")
	}

	endpoint, _ := validateQualificationEndpoint(config.Endpoint)
	if attestation.Endpoint != endpoint.String() {
		return qualificationPermissionAttestation{}, errors.New("permission attestation endpoint does not match the selected Core endpoint")
	}
	expectedFingerprint, err := hex.DecodeString(qualificationCredentialFingerprint(config))
	providedFingerprint, fingerprintErr := hex.DecodeString(attestation.CredentialFingerprint)
	if err != nil || fingerprintErr != nil || len(providedFingerprint) != sha256.Size || !hmac.Equal(expectedFingerprint, providedFingerprint) {
		return qualificationPermissionAttestation{}, errors.New("permission attestation credential identity does not match the selected credentials")
	}
	verifiedAt, err := time.Parse(time.RFC3339, attestation.VerifiedAt)
	if err != nil || !strings.HasSuffix(attestation.VerifiedAt, "Z") || verifiedAt.After(now.Add(qualificationAttestationFutureSkew)) || now.Sub(verifiedAt) > qualificationAttestationMaxAge {
		return qualificationPermissionAttestation{}, errors.New("permission attestation verification time is invalid or stale")
	}
	attestation.OperatorVerified = true
	return attestation, nil
}

func decodeQualificationPermissionAttestation(contents []byte) (qualificationPermissionAttestation, error) {
	var result qualificationPermissionAttestation
	decoder := json.NewDecoder(bytes.NewReader(contents))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return result, errors.New("invalid permission attestation object")
	}
	seen := make(map[string]struct{}, 12)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return result, errors.New("invalid permission attestation field")
		}
		name, ok := token.(string)
		if !ok {
			return result, errors.New("invalid permission attestation field")
		}
		if _, duplicate := seen[name]; duplicate {
			return result, errors.New("duplicate permission attestation field")
		}
		seen[name] = struct{}{}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return result, errors.New("invalid permission attestation value")
		}
		switch name {
		case "version":
			err = json.Unmarshal(raw, &result.Version)
		case "endpoint":
			err = json.Unmarshal(raw, &result.Endpoint)
		case "credentialFingerprint":
			err = json.Unmarshal(raw, &result.CredentialFingerprint)
		case "verifiedAt":
			err = json.Unmarshal(raw, &result.VerifiedAt)
		case "principalNonAdmin":
			err = json.Unmarshal(raw, &result.PrincipalNonAdmin)
		case "allExposedResourcesRead":
			err = json.Unmarshal(raw, &result.AllExposedResourcesRead)
		case "noEffectiveSpecificPermissions":
			err = json.Unmarshal(raw, &result.NoEffectiveSpecificPermissions)
		case "noElevatedDirectGrants":
			err = json.Unmarshal(raw, &result.NoElevatedDirectGrants)
		case "noElevatedUserGroupGrants":
			err = json.Unmarshal(raw, &result.NoElevatedUserGroupGrants)
		case "noElevatedEveryoneGroupGrants":
			err = json.Unmarshal(raw, &result.NoElevatedEveryoneGroupGrants)
		case "noElevatedResourceDefaults":
			err = json.Unmarshal(raw, &result.NoElevatedResourceDefaults)
		case "noElevatedInheritedPermissions":
			err = json.Unmarshal(raw, &result.NoElevatedInheritedPermissions)
		default:
			return qualificationPermissionAttestation{}, errors.New("unsupported permission attestation field")
		}
		if err != nil {
			return qualificationPermissionAttestation{}, errors.New("invalid permission attestation value")
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return qualificationPermissionAttestation{}, errors.New("invalid permission attestation object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return qualificationPermissionAttestation{}, errors.New("trailing permission attestation data")
	}
	if len(seen) != 12 || result.Version != 1 || strings.TrimSpace(result.Endpoint) == "" || strings.TrimSpace(result.CredentialFingerprint) == "" || strings.TrimSpace(result.VerifiedAt) == "" ||
		!result.PrincipalNonAdmin || !result.AllExposedResourcesRead || !result.NoEffectiveSpecificPermissions ||
		!result.NoElevatedDirectGrants || !result.NoElevatedUserGroupGrants || !result.NoElevatedEveryoneGroupGrants ||
		!result.NoElevatedResourceDefaults || !result.NoElevatedInheritedPermissions {
		return qualificationPermissionAttestation{}, errors.New("incomplete permission attestation")
	}
	return result, nil
}

// qualificationPermissionTransport observes level-only "Read" responses for
// qualification reporting. It never adapts them into structured responses,
// because doing so would claim that effective specific permissions were
// API-verified when they were not. Production clients never install this
// test-only observer.
type qualificationPermissionTransport struct {
	base     http.RoundTripper
	attested bool
	seen     int
}

func newQualificationPermissionTransport(base http.RoundTripper, attestation qualificationPermissionAttestation) *qualificationPermissionTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &qualificationPermissionTransport{base: base, attested: attestation.OperatorVerified}
}

func (transport *qualificationPermissionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil || response == nil || request.URL.Path != "/read/GetPermission" || response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, err
	}
	if response.ContentLength > 64 {
		return response, nil
	}
	contents, readErr := io.ReadAll(io.LimitReader(response.Body, 65))
	if readErr != nil {
		response.Body = &qualificationReplayBody{Reader: io.MultiReader(bytes.NewReader(contents), response.Body), closer: response.Body}
		return response, nil
	}
	if request.Method == http.MethodPost && len(contents) <= 64 && strings.TrimSpace(string(contents)) == `"Read"` {
		transport.seen++
	}
	response.Body = &qualificationReplayBody{Reader: io.MultiReader(bytes.NewReader(contents), response.Body), closer: response.Body}
	return response, nil
}

type qualificationReplayBody struct {
	io.Reader
	closer io.Closer
}

func (body *qualificationReplayBody) Close() error { return body.closer.Close() }
