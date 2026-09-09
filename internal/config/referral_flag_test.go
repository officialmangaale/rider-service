package config

import "testing"

// The rider referral callback must be off unless a deployment opts in, so
// applying this work changes no delivery behaviour by itself.
func TestRiderReferralIsDisabledByDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RiderReferralEnabled {
		t.Error("RiderReferralEnabled must default to false")
	}
}

func TestRiderReferralIsEnabledOnlyByAnExplicitTruthyValue(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")

	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
		// A typo must not silently enable a programme that pays money.
		{"yes", false},
		{"", false},
	} {
		t.Setenv("RIDER_REFERRAL_ENABLED", tc.value)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load with %q: %v", tc.value, err)
		}
		if cfg.RiderReferralEnabled != tc.want {
			t.Errorf("RIDER_REFERRAL_ENABLED=%q gave %v; want %v", tc.value, cfg.RiderReferralEnabled, tc.want)
		}
	}
}
