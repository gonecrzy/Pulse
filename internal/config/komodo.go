package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const komodoSensitiveMask = "********"

// KomodoInstance stores an organization-scoped connection to a Komodo Core
// API. Credentials are stored in the encrypted komodo.enc file.
type KomodoInstance struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	APIKey    string `json:"apiKey,omitempty"`
	APISecret string `json:"apiSecret,omitempty"`
	Enabled   bool   `json:"enabled"`
}

// NewKomodoInstance creates an enabled connection with a stable UUID.
func NewKomodoInstance() KomodoInstance {
	return KomodoInstance{ID: uuid.NewString(), Enabled: true}
}

// Validate accepts an HTTPS origin only. This keeps requests on the
// documented Core API root and prevents credentials being sent to an
// arbitrary path or URL userinfo target.
func (k *KomodoInstance) Validate() error {
	if k == nil {
		return fmt.Errorf("Komodo connection is required")
	}
	if strings.TrimSpace(k.Name) == "" {
		return fmt.Errorf("Komodo connection name is required")
	}
	if _, err := uuid.Parse(strings.TrimSpace(k.ID)); err != nil {
		return fmt.Errorf("Komodo connection ID must be a UUID")
	}
	parsed, err := url.Parse(strings.TrimSpace(k.Endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("Komodo endpoint must be a valid HTTPS URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("Komodo endpoint must use HTTPS")
	}
	if parsed.User != nil {
		return fmt.Errorf("Komodo endpoint must not include credentials")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("Komodo endpoint must not include an API path")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("Komodo endpoint must not include a query or fragment")
	}
	if strings.TrimSpace(k.APIKey) == "" {
		return fmt.Errorf("Komodo API key is required")
	}
	if strings.TrimSpace(k.APISecret) == "" {
		return fmt.Errorf("Komodo API secret is required")
	}
	return nil
}

// Redacted returns a copy with credentials replaced by the API mask.
func (k *KomodoInstance) Redacted() KomodoInstance {
	if k == nil {
		return KomodoInstance{}
	}
	out := *k
	if strings.TrimSpace(out.APIKey) != "" {
		out.APIKey = komodoSensitiveMask
	}
	if strings.TrimSpace(out.APISecret) != "" {
		out.APISecret = komodoSensitiveMask
	}
	return out
}

// PreserveMaskedSecrets keeps stored credentials when an editor submits the
// redacted placeholders unchanged, while allowing explicit key rotation.
func (k *KomodoInstance) PreserveMaskedSecrets(existing KomodoInstance) {
	if k == nil {
		return
	}
	if strings.TrimSpace(k.APIKey) == komodoSensitiveMask {
		k.APIKey = existing.APIKey
	}
	if strings.TrimSpace(k.APISecret) == komodoSensitiveMask {
		k.APISecret = existing.APISecret
	}
}
