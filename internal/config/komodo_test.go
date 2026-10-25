package config

import (
	"os"
	"strings"
	"testing"
)

func TestKomodoInstanceValidatesCoreEndpointAndCredentials(t *testing.T) {
	valid := NewKomodoInstance()
	valid.Name = "production"
	valid.Endpoint = "https://komodo.example.test"
	valid.APIKey = "api-key"
	valid.APISecret = "api-secret"
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Komodo instance: %v", err)
	}

	tests := []struct {
		name   string
		edit   func(*KomodoInstance)
		wanted string
	}{
		{name: "missing endpoint", edit: func(v *KomodoInstance) { v.Endpoint = "" }, wanted: "endpoint"},
		{name: "plain HTTP", edit: func(v *KomodoInstance) { v.Endpoint = "http://komodo.example.test" }, wanted: "https"},
		{name: "endpoint userinfo", edit: func(v *KomodoInstance) { v.Endpoint = "https://user:pass@komodo.example.test" }, wanted: "credentials"},
		{name: "path prefix", edit: func(v *KomodoInstance) { v.Endpoint = "https://komodo.example.test/core" }, wanted: "path"},
		{name: "missing API key", edit: func(v *KomodoInstance) { v.APIKey = "" }, wanted: "key"},
		{name: "missing API secret", edit: func(v *KomodoInstance) { v.APISecret = "" }, wanted: "secret"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := valid
			test.edit(&instance)
			if err := instance.Validate(); err == nil || !strings.Contains(strings.ToLower(err.Error()), test.wanted) {
				t.Fatalf("Validate() error = %v, want safe %q validation error", err, test.wanted)
			}
		})
	}
}

func TestKomodoInstanceMasksAndPreservesCredentialRotation(t *testing.T) {
	stored := KomodoInstance{APIKey: "old-key", APISecret: "old-secret"}
	redacted := stored.Redacted()
	if redacted.APIKey != komodoSensitiveMask || redacted.APISecret != komodoSensitiveMask {
		t.Fatalf("redacted credentials = (%q, %q)", redacted.APIKey, redacted.APISecret)
	}
	if strings.Contains(strings.Join([]string{redacted.APIKey, redacted.APISecret}, " "), "old-") {
		t.Fatal("redacted connection exposes an old credential")
	}

	unchanged := redacted
	unchanged.PreserveMaskedSecrets(stored)
	if unchanged.APIKey != stored.APIKey || unchanged.APISecret != stored.APISecret {
		t.Fatalf("masked update did not preserve existing credentials: %#v", unchanged)
	}

	rotated := redacted
	rotated.APIKey = "new-key"
	rotated.APISecret = "new-secret"
	rotated.PreserveMaskedSecrets(stored)
	if rotated.APIKey != "new-key" || rotated.APISecret != "new-secret" {
		t.Fatalf("explicitly rotated credentials were overwritten: %#v", rotated)
	}
}

func TestKomodoConfigPersistenceEncryptsCredentials(t *testing.T) {
	cp, err := newConfigPersistence(t.TempDir())
	if err != nil {
		t.Fatalf("newConfigPersistence: %v", err)
	}
	instance := NewKomodoInstance()
	instance.Name = "home Core"
	instance.Endpoint = "https://komodo.example.test"
	instance.APIKey = "persisted-komodo-key"
	instance.APISecret = "persisted-komodo-secret"
	if err := cp.SaveKomodoConfig([]KomodoInstance{instance}); err != nil {
		t.Fatalf("SaveKomodoConfig: %v", err)
	}
	data, err := os.ReadFile(cp.komodoFile)
	if err != nil {
		t.Fatalf("read encrypted Komodo config: %v", err)
	}
	if strings.Contains(string(data), instance.APIKey) || strings.Contains(string(data), instance.APISecret) {
		t.Fatal("Komodo credentials were written to disk without encryption")
	}
	loaded, err := cp.LoadKomodoConfig()
	if err != nil {
		t.Fatalf("LoadKomodoConfig: %v", err)
	}
	if len(loaded) != 1 || loaded[0].APIKey != instance.APIKey || loaded[0].APISecret != instance.APISecret {
		t.Fatalf("loaded config = %#v, want encrypted credential round trip", loaded)
	}
}
