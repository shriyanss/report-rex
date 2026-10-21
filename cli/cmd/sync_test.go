package cmd

import "testing"

func TestResolveSyncPasswordPrefersFlagOverEnv(t *testing.T) {
	t.Setenv("REX_PASSWORD", "from-env")
	pw, err := resolveSyncPassword(syncFlags{Password: "from-flag"})
	if err != nil {
		t.Fatalf("resolveSyncPassword: %v", err)
	}
	if pw != "from-flag" {
		t.Errorf("password = %q, want %q", pw, "from-flag")
	}
}

func TestResolveSyncPasswordFallsBackToEnv(t *testing.T) {
	t.Setenv("REX_PASSWORD", "from-env")
	pw, err := resolveSyncPassword(syncFlags{})
	if err != nil {
		t.Fatalf("resolveSyncPassword: %v", err)
	}
	if pw != "from-env" {
		t.Errorf("password = %q, want %q", pw, "from-env")
	}
}
