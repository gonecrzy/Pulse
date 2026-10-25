package komodo

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/rcourtman/pulse-go-rewrite/internal/config"
)

const maxQualificationCredentialFileBytes = 4096

type liveQualificationConfig struct {
	Client     ClientConfig
	IdentityID string
}

type qualificationCredentials struct {
	APIKey    string
	APISecret string
}

// loadLiveQualificationConfig is test-only and refuses to read protected
// material unless the live harness was explicitly opted into. An explicitly
// selected encrypted Pulse connection takes precedence over the local file.
func loadLiveQualificationConfig() (liveQualificationConfig, error) {
	if os.Getenv("PULSE_KOMODO_LIVE_QUALIFICATION") != "1" {
		return liveQualificationConfig{}, errors.New("live Komodo credentials require an explicitly opted in test")
	}

	dataDir := strings.TrimSpace(os.Getenv("PULSE_DATA_DIR"))
	connectionID := strings.TrimSpace(os.Getenv("PULSE_KOMODO_QUALIFICATION_CONNECTION_ID"))
	if dataDir != "" {
		if connectionID == "" {
			return liveQualificationConfig{}, errors.New("protected Komodo connection selection requires a data directory and connection UUID")
		}
		persistence := config.NewConfigPersistence(dataDir)
		instances, err := persistence.LoadKomodoConfig()
		if err != nil {
			return liveQualificationConfig{}, errors.New("could not load protected Komodo connection configuration")
		}
		var selected *config.KomodoInstance
		for index := range instances {
			if instances[index].ID == connectionID {
				selected = &instances[index]
				break
			}
		}
		if selected == nil {
			return liveQualificationConfig{}, errors.New("selected Komodo connection was not found in the protected configuration")
		}
		if !selected.Enabled {
			return liveQualificationConfig{}, errors.New("selected Komodo connection is disabled")
		}
		if err := selected.Validate(); err != nil {
			return liveQualificationConfig{}, errors.New("selected Komodo connection configuration is invalid")
		}
		return liveQualificationConfig{
			Client: ClientConfig{
				Endpoint:  selected.Endpoint,
				APIKey:    selected.APIKey,
				APISecret: selected.APISecret,
			},
			IdentityID: selected.ID,
		}, nil
	}
	if connectionID != "" {
		return liveQualificationConfig{}, errors.New("connection UUID selection requires an explicit Pulse data directory")
	}

	credentialPath := strings.TrimSpace(os.Getenv("PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE"))
	endpoint := strings.TrimSpace(os.Getenv("PULSE_KOMODO_QUALIFICATION_ENDPOINT"))
	if credentialPath == "" || endpoint == "" {
		return liveQualificationConfig{}, errors.New("file qualification requires an explicit credential file and HTTPS endpoint")
	}
	validatedEndpoint, err := validateEndpoint(endpoint)
	if err != nil {
		return liveQualificationConfig{}, errors.New("qualification endpoint is invalid")
	}
	credentials, err := readQualificationCredentialFile(credentialPath)
	if err != nil {
		return liveQualificationConfig{}, err
	}

	// File-backed qualification has no persisted Pulse connection UUID. Scope
	// the temporary identity to the normalized endpoint without using secrets.
	identityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("pulse-komodo-live-qualification:"+validatedEndpoint.String())).String()
	return liveQualificationConfig{
		Client: ClientConfig{
			Endpoint:  validatedEndpoint.String(),
			APIKey:    credentials.APIKey,
			APISecret: credentials.APISecret,
		},
		IdentityID: identityID,
	}, nil
}

// readQualificationCredentialFile accepts only a small, regular, non-symlink
// file owned by the current test operator and readable by that owner alone.
// It never includes file contents or the path in returned errors.
func readQualificationCredentialFile(path string) (qualificationCredentials, error) {
	contents, err := readOwnerOnlyQualificationFile(path)
	if err != nil {
		return qualificationCredentials{}, err
	}
	credentials, err := decodeQualificationCredentials(contents)
	zeroQualificationBytes(contents)
	if err != nil {
		return qualificationCredentials{}, errors.New("qualification credential file has an invalid format")
	}
	return credentials, nil
}

func readOwnerOnlyQualificationFile(path string) ([]byte, error) {
	if os.Getenv("PULSE_KOMODO_LIVE_QUALIFICATION") != "1" {
		return nil, errors.New("protected qualification file access requires an explicitly opted in test")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("could not read protected qualification file")
	}
	initialInfo, err := os.Lstat(path)
	if err != nil || !initialInfo.Mode().IsRegular() {
		return nil, errors.New("could not read protected qualification file")
	}

	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("could not read protected qualification file")
	}
	file := os.NewFile(uintptr(fd), "komodo-qualification-protected-file")
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not read protected qualification file")
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(initialInfo, openedInfo) {
		return nil, errors.New("could not read protected qualification file")
	}
	currentInfo, err := os.Lstat(path)
	if err != nil || !currentInfo.Mode().IsRegular() || !os.SameFile(openedInfo, currentInfo) {
		return nil, errors.New("could not read protected qualification file")
	}
	permissions := openedInfo.Mode().Perm()
	specialPermissions := openedInfo.Mode() & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if permissions&0077 != 0 || permissions&0400 == 0 || permissions&0111 != 0 || specialPermissions != 0 || !qualificationFileOwnedByCurrentUser(openedInfo) {
		return nil, errors.New("qualification file must be owner-only and owned by the test operator")
	}
	if openedInfo.Size() > maxQualificationCredentialFileBytes {
		return nil, errors.New("qualification file exceeds the safe size limit")
	}

	contents, err := io.ReadAll(io.LimitReader(file, maxQualificationCredentialFileBytes+1))
	if err != nil || len(contents) > maxQualificationCredentialFileBytes {
		zeroQualificationBytes(contents)
		return nil, errors.New("could not read protected qualification file")
	}
	return contents, nil
}

func zeroQualificationBytes(contents []byte) {
	for index := range contents {
		contents[index] = 0
	}
}

func qualificationFileOwnedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func decodeQualificationCredentials(contents []byte) (qualificationCredentials, error) {
	var credentials qualificationCredentials
	decoder := json.NewDecoder(bytes.NewReader(contents))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return qualificationCredentials{}, errors.New("invalid credential object")
	}
	seen := make(map[string]struct{}, 2)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return qualificationCredentials{}, errors.New("invalid credential field")
		}
		name, ok := token.(string)
		if !ok {
			return qualificationCredentials{}, errors.New("invalid credential field")
		}
		if _, duplicate := seen[name]; duplicate {
			return qualificationCredentials{}, errors.New("duplicate credential field")
		}
		seen[name] = struct{}{}
		var value string
		if err := decoder.Decode(&value); err != nil {
			return qualificationCredentials{}, errors.New("invalid credential value")
		}
		switch name {
		case "apiKey":
			credentials.APIKey = value
		case "apiSecret":
			credentials.APISecret = value
		default:
			return qualificationCredentials{}, errors.New("unsupported credential field")
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return qualificationCredentials{}, errors.New("invalid credential object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return qualificationCredentials{}, errors.New("trailing credential data")
	}
	if strings.TrimSpace(credentials.APIKey) == "" || strings.TrimSpace(credentials.APISecret) == "" || strings.ContainsAny(credentials.APIKey+credentials.APISecret, "\r\n\x00") {
		return qualificationCredentials{}, errors.New("missing or invalid credentials")
	}
	return credentials, nil
}

func TestLiveQualificationCredentialFileIsExplicitAndOwnerOnly(t *testing.T) {
	const key = "fixture-api-key"
	const secret = "fixture-api-secret"
	path := filepath.Join(t.TempDir(), "komodo-qualification.json")
	if err := os.WriteFile(path, []byte(`{"apiKey":"`+key+`","apiSecret":"`+secret+`"}`), 0600); err != nil {
		t.Fatal("write protected credential fixture")
	}

	t.Setenv("PULSE_KOMODO_LIVE_QUALIFICATION", "1")
	t.Setenv("PULSE_DATA_DIR", "")
	t.Setenv("PULSE_KOMODO_QUALIFICATION_CONNECTION_ID", "")
	t.Setenv("PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE", path)
	t.Setenv("PULSE_KOMODO_QUALIFICATION_ENDPOINT", "https://komodo.example.test")

	got, err := loadLiveQualificationConfig()
	if err != nil {
		t.Fatalf("load opted-in owner-only credential file: %v", err)
	}
	if got.Client.APIKey != key || got.Client.APISecret != secret || got.Client.Endpoint != "https://komodo.example.test" {
		t.Fatal("loaded qualification client config does not match the protected fixture")
	}
	if got.IdentityID == "" {
		t.Fatal("file-backed qualification did not derive a stable identity scope")
	}
	again, err := loadLiveQualificationConfig()
	if err != nil || again.IdentityID != got.IdentityID {
		t.Fatal("file-backed qualification identity scope was not stable")
	}
}

func TestLiveQualificationPrefersProtectedPulseConnection(t *testing.T) {
	dataDir := t.TempDir()
	persistence := config.NewConfigPersistence(dataDir)
	instance := config.NewKomodoInstance()
	instance.Name = "qualification fixture"
	instance.Endpoint = "https://stored.example.test"
	instance.APIKey = "stored-api-key"
	instance.APISecret = "stored-api-secret"
	if err := persistence.SaveKomodoConfig([]config.KomodoInstance{instance}); err != nil {
		t.Fatal("save encrypted connection fixture")
	}

	t.Setenv("PULSE_KOMODO_LIVE_QUALIFICATION", "1")
	t.Setenv("PULSE_DATA_DIR", dataDir)
	t.Setenv("PULSE_KOMODO_QUALIFICATION_CONNECTION_ID", instance.ID)
	t.Setenv("PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE", filepath.Join(t.TempDir(), "must-not-be-read.json"))
	t.Setenv("PULSE_KOMODO_QUALIFICATION_ENDPOINT", "https://ignored.example.test")

	got, err := loadLiveQualificationConfig()
	if err != nil {
		t.Fatalf("load protected Pulse connection: %v", err)
	}
	if got.Client.Endpoint != instance.Endpoint || got.Client.APIKey != instance.APIKey || got.Client.APISecret != instance.APISecret || got.IdentityID != instance.ID {
		t.Fatal("protected Pulse connection was not preferred")
	}
}

func TestLiveQualificationCredentialFileRejectsUnsafeFilesWithoutEchoingContents(t *testing.T) {
	const secret = "do-not-echo-this-secret"
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) string
	}{
		{
			name: "group-readable",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(path, []byte(`{"apiKey":"key","apiSecret":"`+secret+`"}`), 0644); err != nil {
					t.Fatal("write credential fixture")
				}
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal("set unsafe credential fixture permissions")
				}
				return path
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "target.json")
				if err := os.WriteFile(target, []byte(`{"apiKey":"key","apiSecret":"`+secret+`"}`), 0600); err != nil {
					t.Fatal("write credential fixture")
				}
				path := filepath.Join(dir, "credentials.json")
				if err := os.Symlink(target, path); err != nil {
					t.Skip("symlinks are unavailable")
				}
				return path
			},
		},
		{
			name: "malformed-json",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
					t.Fatal("write credential fixture")
				}
				return path
			},
		},
		{
			name: "duplicate-field",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(path, []byte(`{"apiKey":"key","apiKey":"other","apiSecret":"`+secret+`"}`), 0600); err != nil {
					t.Fatal("write credential fixture")
				}
				return path
			},
		},
		{
			name: "unknown-field",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(path, []byte(`{"apiKey":"key","apiSecret":"`+secret+`","extra":"value"}`), 0600); err != nil {
					t.Fatal("write credential fixture")
				}
				return path
			},
		},
		{
			name: "trailing-data",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(path, []byte(`{"apiKey":"key","apiSecret":"`+secret+`"} {}`), 0600); err != nil {
					t.Fatal("write credential fixture")
				}
				return path
			},
		},
		{
			name: "oversized",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(path, []byte(strings.Repeat("x", maxQualificationCredentialFileBytes+1)), 0600); err != nil {
					t.Fatal("write oversized credential fixture")
				}
				return path
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PULSE_KOMODO_LIVE_QUALIFICATION", "1")
			path := test.setup(t, t.TempDir())
			_, err := readQualificationCredentialFile(path)
			if err == nil {
				t.Fatal("unsafe credential file was accepted")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("credential file error exposed secret contents")
			}
		})
	}
}

func TestLiveQualificationCredentialFileRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("PULSE_KOMODO_LIVE_QUALIFICATION", "0")
	t.Setenv("PULSE_DATA_DIR", "")
	t.Setenv("PULSE_KOMODO_QUALIFICATION_CONNECTION_ID", "")
	t.Setenv("PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("PULSE_KOMODO_QUALIFICATION_ENDPOINT", "https://komodo.example.test")

	_, err := loadLiveQualificationConfig()
	if err == nil || !strings.Contains(err.Error(), "explicitly opted in") {
		t.Fatal("live credential configuration was accessible without explicit opt-in")
	}
	_, err = readQualificationCredentialFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "explicitly opted in") {
		t.Fatal("credential file reader was accessible without explicit opt-in")
	}
}
