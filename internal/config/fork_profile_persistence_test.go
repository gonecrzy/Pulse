package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
)

func TestForkProfilePersistenceDefaultsAndUsesPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	persistence, err := newConfigPersistence(root)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := persistence.LoadForkProfile()
	if err != nil || profile != nil {
		t.Fatalf("missing profile = %#v, err = %v; want nil default", profile, err)
	}
	want := forkfeatures.DefaultProfile()
	want.EnabledFeatures = []forkfeatures.FeatureID{forkfeatures.FeatureAIInvestigation}
	if err := persistence.SaveForkProfile(want); err != nil {
		t.Fatal(err)
	}
	got, err := persistence.LoadForkProfile()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("loaded profile = %#v, want %#v", got, want)
	}
	info, err := os.Stat(filepath.Join(root, "fork_profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("fork profile permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestForkProfilePersistenceRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	persistence, err := newConfigPersistence(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{
		`{"version":1,"revision":1,"enabled_features":[],"commercial_entitlements":["ai_autofix"]}`,
		`{"version":1,"revision":1,"enabled_features":[]} {"version":1}`,
	} {
		if err := os.WriteFile(persistence.forkProfileFile, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := persistence.LoadForkProfile(); err == nil {
			t.Fatalf("LoadForkProfile accepted %s", content)
		}
	}
}
