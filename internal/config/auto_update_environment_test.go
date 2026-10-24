package config

import "testing"

func TestEffectiveAutoUpdateEnabledHonorsPinnedDeploymentLock(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{name: "unset permits normal stable-channel setting", env: "", want: true},
		{name: "true disables persisted stable-channel setting", env: "true", want: false},
		{name: "one disables persisted stable-channel setting", env: "1", want: false},
		{name: "yes disables persisted stable-channel setting", env: "yes", want: false},
		{name: "false leaves persisted stable-channel setting intact", env: "false", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PULSE_DISABLE_AUTO_UPDATE", tt.env)
			if got := EffectiveAutoUpdateEnabled("stable", true); got != tt.want {
				t.Fatalf("EffectiveAutoUpdateEnabled(stable, true) = %v, want %v", got, tt.want)
			}
		})
	}
}
